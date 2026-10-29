package administration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/yomezza/cosmosimulator/internal/account"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/wallet"
)

func (a *Service) AdminTick(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	if err := a.Simulation.AdvanceTick(r.Context(), true, u.ID); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if err := a.Simulation.Activities(r.Context()); err != nil {
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 200, map[string]bool{"ok": true})
}

func (a *Service) EditUser(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	var in struct {
		ID       int64  `json:"id"`
		Username string `json:"username"`
		Role     string `json:"role"`
		Credits  int64  `json:"credits"`
		Level    int    `json:"level"`
		XP       int    `json:"xp"`
		Password string `json:"password"`
	}
	if !httpkit.Decode(w, r, &in) {
		return
	}
	if (in.Role != "admin" && in.Role != "player") || in.Credits < 0 || in.Credits > wallet.MaxCredits || in.Level < 1 || in.Level > 1000000 || in.XP < 0 || in.XP > 1000000000 {
		httpkit.Fail(w, 400, "Некорректные данные пользователя")
		return
	}
	var hash []byte
	var err error
	if in.Password != "" {
		if len(in.Password) < 1 || len(in.Password) > 72 {
			httpkit.Fail(w, 400, "Пароль: 1–72 байта")
			return
		}
		hash, err = bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
		if err != nil {
			httpkit.Internal(w, err)
			return
		}
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	if err = wallet.Reason(r.Context(), tx, "admin_adjustment"); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if r.Method == "POST" {
		if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(784218)`); err != nil {
			httpkit.Internal(w, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(784217)`); err != nil {
		httpkit.Internal(w, err)
		return
	}
	var role string
	if err = tx.QueryRow(r.Context(), `SELECT role FROM users WHERE id=$1`, u.ID).Scan(&role); err != nil || role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	var bot bool
	var oldName string
	var oldRole string
	err = tx.QueryRow(r.Context(), `SELECT is_bot,username,role FROM users WHERE id=$1 FOR UPDATE`, in.ID).Scan(&bot, &oldName, &oldRole)
	if errors.Is(err, pgx.ErrNoRows) {
		httpkit.Fail(w, 404, "Пользователь не найден")
		return
	}
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if bot && (in.Username != oldName || in.Role != oldRole || len(hash) > 0) {
		httpkit.Fail(w, 400, "Нельзя изменять идентификацию бота")
		return
	}
	if !bot && !account.ValidName(in.Username) {
		httpkit.Fail(w, 400, "Некорректное имя пользователя")
		return
	}
	if oldRole == "admin" && in.Role != "admin" {
		var count int
		if err = tx.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE role='admin'`).Scan(&count); err != nil {
			httpkit.Internal(w, err)
			return
		}
		if count <= 1 || in.ID == u.ID {
			httpkit.Fail(w, 409, "Нельзя понизить себя или последнего администратора")
			return
		}
	}
	tag, err := tx.Exec(r.Context(), `UPDATE users SET username=$2,role=$3,credits=$4,level=$5,xp=$6,password_hash=CASE WHEN $7='' THEN password_hash ELSE $7 END WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM users WHERE username=$2 AND id<>$1)`, in.ID, in.Username, in.Role, in.Credits, in.Level, in.XP, string(hash))
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpkit.Fail(w, 409, "Имя уже занято")
		return
	}
	if len(hash) > 0 || oldRole != in.Role {
		if _, err = tx.Exec(r.Context(), `DELETE FROM sessions WHERE user_id=$1`, in.ID); err != nil {
			httpkit.Internal(w, err)
			return
		}
	}
	audit, _ := json.Marshal(map[string]any{"user": in.ID, "role": in.Role, "credits": in.Credits, "level": in.Level, "xp": in.XP, "password_reset": len(hash) > 0})
	if _, err = tx.Exec(r.Context(), `INSERT INTO admin_log(actor,action,data) VALUES($1,'user_edit',$2)`, u.ID, audit); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 200, map[string]bool{"ok": true})
}
