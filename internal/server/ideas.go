package server

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type playerIdea struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Request     string `json:"request"`
}

func ideaText(s string, multiline bool) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return false
		}
	}
	return true
}
func (a *App) ideas(w http.ResponseWriter, r *http.Request) {
	u, ok := a.user(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if r.Method == "GET" {
		scope := r.URL.Query().Get("scope")
		if scope != "" && scope != "mine" && scope != "all" {
			fail(w, 400, "Некорректный раздел предложений")
			return
		}
		owner := u.ID
		if scope == "all" {
			if u.Role != "admin" {
				fail(w, 403, "Требуется роль администратора")
				return
			}
			owner = 0
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		page = max(1, min(100000, page))
		var total int
		if err := a.db.QueryRow(ctx, `SELECT count(*) FROM player_ideas WHERE ($1::bigint=0 OR user_id=$1)`, owner).Scan(&total); err != nil {
			a.internal(w, err)
			return
		}
		pages := max(1, (total+9)/10)
		page = min(page, pages)
		items, err := queryMaps(ctx, a.db, `SELECT i.id,i.title,i.description,i.category,i.created_at,COALESCE(u.username,i.author_name) AS author FROM player_ideas i LEFT JOIN users u ON u.id=i.user_id WHERE ($1::bigint=0 OR i.user_id=$1) ORDER BY i.id DESC LIMIT 10 OFFSET $2`, owner, (page-1)*10)
		if err != nil {
			a.internal(w, err)
			return
		}
		if items == nil {
			items = []map[string]any{}
		}
		reply(w, 200, map[string]any{"items": items, "total": total, "page": page, "pages": pages})
		return
	}
	var in playerIdea
	if !decode(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(in.Description, "\r\n", "\n"), "\r", "\n"))
	key, err := hex.DecodeString(in.Request)
	if err != nil || len(key) != 16 || strings.ToLower(in.Request) != in.Request {
		fail(w, 400, "Некорректное предложение")
		return
	}
	if !ideaText(in.Title, false) || utf8.RuneCountInString(in.Title) < 3 || utf8.RuneCountInString(in.Title) > 80 {
		fail(w, 400, "Название идеи: от 3 до 80 символов")
		return
	}
	if !ideaText(in.Description, true) || utf8.RuneCountInString(in.Description) < 10 || utf8.RuneCountInString(in.Description) > 3000 {
		fail(w, 400, "Описание идеи: от 10 до 3000 символов")
		return
	}
	if in.Category != "gameplay" && in.Category != "interface" && in.Category != "economy" && in.Category != "other" {
		fail(w, 400, "Выберите категорию идеи")
		return
	}
	tx, err := a.db.Begin(ctx)
	if err != nil {
		a.internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var author string
	if err = tx.QueryRow(ctx, `SELECT username FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&author); err != nil {
		a.internal(w, err)
		return
	}
	var saved struct {
		ID                           int64
		Title, Description, Category string
	}
	err = tx.QueryRow(ctx, `SELECT id,title,description,category FROM player_ideas WHERE user_id=$1 AND request_id=$2`, u.ID, in.Request).Scan(&saved.ID, &saved.Title, &saved.Description, &saved.Category)
	if err == nil {
		if saved.Title != in.Title || saved.Description != in.Description || saved.Category != in.Category {
			fail(w, 409, "Этот запрос уже использован для другой идеи")
			return
		}
		reply(w, 200, map[string]any{"id": saved.ID, "saved": true})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		a.internal(w, err)
		return
	}
	var recent int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM player_ideas WHERE user_id=$1 AND created_at>now()-interval '1 hour'`, u.ID).Scan(&recent); err != nil {
		a.internal(w, err)
		return
	}
	if recent >= 10 {
		fail(w, 429, "Можно отправить не более 10 идей за час. Попробуйте позже.")
		return
	}
	var id int64
	if err = tx.QueryRow(ctx, `INSERT INTO player_ideas(user_id,author_name,request_id,category,title,description) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, u.ID, author, in.Request, in.Category, in.Title, in.Description).Scan(&id); err != nil {
		a.internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		a.internal(w, err)
		return
	}
	reply(w, 201, map[string]any{"id": id, "saved": true})
}
