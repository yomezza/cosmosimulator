package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

//go:embed schema.sql web/*
var assets embed.FS

type App struct {
	db       *pgxpool.Pool
	secure   bool
	origin   string
	mu       sync.Mutex
	attempts map[string]attempt
}
type attempt struct {
	count int
	until time.Time
}
type User struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Bot      bool   `json:"bot"`
	Credits  int64  `json:"credits"`
	Ore      int64  `json:"ore"`
}

func Run(ctx context.Context) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("set DATABASE_URL to a PostgreSQL connection string")
	}
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	check, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err = db.Ping(check); err != nil {
		return fmt.Errorf("PostgreSQL: %w", err)
	}
	tx, err := db.Begin(check)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(check, "SELECT pg_advisory_xact_lock(784216)"); err != nil {
		return err
	}
	schema, _ := assets.ReadFile("schema.sql")
	if _, err = tx.Exec(check, string(schema)); err != nil {
		return err
	}
	if err = tx.Commit(check); err != nil {
		return err
	}
	a := &App{db: db, secure: os.Getenv("COOKIE_SECURE") == "true", origin: strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/"), attempts: make(map[string]attempt)}
	if err = a.bootstrap(ctx); err != nil {
		return err
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	srv := &http.Server{Addr: addr, Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go a.simulate(ctx)
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	log.Printf("Cosmosimulator listening on %s", addr)
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func (a *App) bootstrap(ctx context.Context) error {
	name, pass := os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD")
	if name == "" && pass == "" {
		return nil
	}
	if !validName(name) || len(pass) < 12 || len(pass) > 72 {
		return errors.New("admin username must be 3–24 ASCII letters/digits/_/-; password 12–72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	var role string
	err = a.db.QueryRow(ctx, `INSERT INTO users(username,password_hash,role) VALUES($1,$2,'admin') ON CONFLICT(username) DO UPDATE SET username=EXCLUDED.username RETURNING role`, name, string(hash)).Scan(&role)
	if err == nil && role != "admin" {
		return errors.New("ADMIN_USERNAME belongs to a player; choose another name")
	}
	return err
}
func (a *App) routes() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		c, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if a.db.Ping(c) != nil {
			fail(w, 503, "База недоступна")
			return
		}
		reply(w, 200, map[string]string{"status": "ok"})
	})
	m.HandleFunc("POST /api/register", a.register)
	m.HandleFunc("POST /api/login", a.login)
	m.HandleFunc("POST /api/logout", a.logout)
	m.HandleFunc("GET /api/state", a.state)
	m.HandleFunc("POST /api/sell", a.sell)
	m.HandleFunc("GET /api/admin/users", a.adminUsers)
	web, _ := fs.Sub(assets, "web")
	m.Handle("GET /", http.FileServer(http.FS(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		if r.Method == "POST" {
			origin := r.Header.Get("Origin")
			expected := a.origin
			if expected == "" {
				scheme := "http"
				if r.TLS != nil {
					scheme = "https"
				}
				expected = scheme + "://" + r.Host
			}
			if origin != "" && origin != expected {
				fail(w, 403, "Недопустимый источник запроса")
				return
			}
			if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
				fail(w, 403, "Недопустимый источник запроса")
				return
			}
			if r.Header.Get("Content-Type") != "application/json" {
				fail(w, 415, "Ожидается application/json")
				return
			}
		}
		m.ServeHTTP(w, r)
	})
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, s string) {
	reply(w, status, map[string]string{"error": s})
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil || d.Decode(new(any)) != io.EOF {
		fail(w, 400, "Некорректный JSON")
		return false
	}
	return true
}
func validName(s string) bool {
	if len(s) < 3 || len(s) > 24 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-') {
			return false
		}
	}
	return true
}
func (a *App) allow(r *http.Request) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	for k, v := range a.attempts {
		if now.After(v.until) {
			delete(a.attempts, k)
		}
	}
	// Use the direct peer, never trust a client-supplied forwarded address.
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	v := a.attempts[host]
	if v.count >= 20 {
		return false
	}
	if v.count == 0 {
		v.until = now.Add(5 * time.Minute)
	}
	v.count++
	a.attempts[host] = v
	return true
}
func (a *App) register(w http.ResponseWriter, r *http.Request) {
	if !a.allow(r) {
		fail(w, 429, "Слишком много попыток. Повторите через 5 минут.")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !validName(in.Username) || !utf8.ValidString(in.Password) || len(in.Password) < 12 || len(in.Password) > 72 {
		fail(w, 400, "Имя: 3–24 латинских символа, цифры, _ или -. Пароль: 12–72 байта.")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		a.internal(w, err)
		return
	}
	var id int64
	err = a.db.QueryRow(r.Context(), `INSERT INTO users(username,password_hash) VALUES($1,$2) ON CONFLICT DO NOTHING RETURNING id`, in.Username, string(hash)).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "Имя уже занято")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	a.newSession(w, r, id)
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if !a.allow(r) {
		fail(w, 429, "Слишком много попыток. Повторите через 5 минут.")
		return
	}
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	var id int64
	var hash string
	err := a.db.QueryRow(r.Context(), `SELECT id,password_hash FROM users WHERE username=$1 AND NOT is_bot`, in.Username).Scan(&id, &hash)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		hash = "$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"
	}
	bad := bcrypt.CompareHashAndPassword([]byte(hash), []byte(in.Password))
	if err != nil || bad != nil {
		fail(w, 401, "Неверное имя или пароль")
		return
	}
	a.newSession(w, r, id)
}
func tokenHash(t string) string { h := sha256.Sum256([]byte(t)); return hex.EncodeToString(h[:]) }
func (a *App) newSession(w http.ResponseWriter, r *http.Request, id int64) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		a.internal(w, err)
		return
	}
	token := hex.EncodeToString(b)
	_, err := a.db.Exec(r.Context(), `INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '7 days')`, tokenHash(token), id)
	if err != nil {
		a.internal(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Value: token, Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: 7 * 24 * 3600})
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *App) user(w http.ResponseWriter, r *http.Request) (User, bool) {
	var u User
	c, err := r.Cookie("session")
	if err != nil || len(c.Value) != 64 {
		fail(w, 401, "Войдите в аккаунт")
		return u, false
	}
	err = a.db.QueryRow(r.Context(), `SELECT u.id,u.username,u.role,u.is_bot,u.credits,u.ore FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.token_hash=$1 AND s.expires_at>now()`, tokenHash(c.Value)).Scan(&u.ID, &u.Username, &u.Role, &u.Bot, &u.Credits, &u.Ore)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 401, "Сессия истекла")
		return u, false
	}
	if err != nil {
		a.internal(w, err)
		return u, false
	}
	return u, true
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("session"); err == nil {
		if _, err = a.db.Exec(r.Context(), `DELETE FROM sessions WHERE token_hash=$1`, tokenHash(c.Value)); err != nil {
			a.internal(w, err)
			return
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "session", Path: "/", HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	reply(w, 200, map[string]bool{"ok": true})
}
func (a *App) state(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	var tick int64
	var last time.Time
	if err := a.db.QueryRow(r.Context(), `SELECT tick,last_tick FROM world WHERE id=1`).Scan(&tick, &last); err != nil {
		a.internal(w, err)
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT id,username,role,is_bot,credits,ore FROM users ORDER BY credits DESC,id LIMIT 20`)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	list := []User{}
	for rows.Next() {
		var p User
		if err = rows.Scan(&p.ID, &p.Username, &p.Role, &p.Bot, &p.Credits, &p.Ore); err != nil {
			a.internal(w, err)
			return
		}
		list = append(list, p)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	reply(w, 200, map[string]any{"user": u, "tick": tick, "last_tick": last, "companies": list, "tick_seconds": 10, "ore_price": 5})
}
func (a *App) sell(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	var amount int64
	err := a.db.QueryRow(r.Context(), `UPDATE users SET credits=credits+ore*5,ore=0 WHERE id=$1 AND ore>0 RETURNING credits`, u.ID).Scan(&amount)
	if errors.Is(err, pgx.ErrNoRows) {
		fail(w, 409, "Нет руды для продажи")
		return
	}
	if err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 200, map[string]int64{"credits": amount})
}
func (a *App) adminUsers(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		fail(w, 403, "Требуется роль администратора")
		return
	}
	rows, err := a.db.Query(r.Context(), `SELECT id,username,role,is_bot,credits,ore FROM users ORDER BY id LIMIT 200`)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer rows.Close()
	list := []User{}
	for rows.Next() {
		var p User
		if err = rows.Scan(&p.ID, &p.Username, &p.Role, &p.Bot, &p.Credits, &p.Ore); err != nil {
			a.internal(w, err)
			return
		}
		list = append(list, p)
	}
	if rows.Err() != nil {
		a.internal(w, rows.Err())
		return
	}
	reply(w, 200, list)
}
func (a *App) internal(w http.ResponseWriter, err error) {
	log.Printf("request failed: %v", err)
	fail(w, 500, "Внутренняя ошибка сервера")
}
func (a *App) simulate(ctx context.Context) {
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := a.tick(ctx); err != nil && ctx.Err() == nil {
				log.Printf("simulation: %v", err)
			}
		}
	}
}
func (a *App) tick(ctx context.Context) error {
	tx, err := a.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var tick int64
	err = tx.QueryRow(ctx, `UPDATE world SET tick=tick+1,last_tick=now() WHERE id=1 AND last_tick<=now()-interval '10 seconds' RETURNING tick`).Scan(&tick)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE users SET ore=ore+CASE WHEN is_bot THEN 2 ELSE 1 END`); err != nil {
		return err
	}
	if tick%3 == 0 {
		if _, err = tx.Exec(ctx, `UPDATE users SET credits=credits+ore*5,ore=0 WHERE is_bot`); err != nil {
			return err
		}
	}
	if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE expires_at<=now()`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
