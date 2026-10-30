package ideas

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
	textutil "github.com/yomezza/cosmosimulator/internal/platform/text"
)

type playerIdea struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Request     string `json:"request"`
}

func (a *Service) Ideas(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if r.Method == "GET" {
		scope := r.URL.Query().Get("scope")
		if scope != "" && scope != "mine" && scope != "all" {
			httpkit.Fail(w, 400, "Некорректный раздел предложений")
			return
		}
		owner := u.ID
		if scope == "all" {
			if u.Role != "admin" {
				httpkit.Fail(w, 403, "Требуется роль администратора")
				return
			}
			owner = 0
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		page = max(1, min(100000, page))
		var total int
		if err := a.DB.QueryRow(ctx, `SELECT count(*) FROM player_ideas WHERE ($1::bigint=0 OR user_id=$1)`, owner).Scan(&total); err != nil {
			httpkit.Internal(w, err)
			return
		}
		pages := max(1, (total+9)/10)
		page = min(page, pages)
		items, err := postgres.QueryMaps(ctx, a.DB, `SELECT i.id,i.title,i.description,i.category,i.status,i.created_at,COALESCE(u.username,i.author_name) AS author FROM player_ideas i LEFT JOIN users u ON u.id=i.user_id WHERE ($1::bigint=0 OR i.user_id=$1) ORDER BY i.id DESC LIMIT 10 OFFSET $2`, owner, (page-1)*10)
		if err != nil {
			httpkit.Internal(w, err)
			return
		}
		if items == nil {
			items = []map[string]any{}
		}
		httpkit.Reply(w, 200, map[string]any{"items": items, "total": total, "page": page, "pages": pages})
		return
	}
	var in playerIdea
	if !httpkit.Decode(w, r, &in) {
		return
	}
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(in.Description, "\r\n", "\n"), "\r", "\n"))
	key, err := hex.DecodeString(in.Request)
	if err != nil || len(key) != 16 || strings.ToLower(in.Request) != in.Request {
		httpkit.Fail(w, 400, "Некорректное предложение")
		return
	}
	if !textutil.Valid(in.Title, false) || utf8.RuneCountInString(in.Title) < 3 || utf8.RuneCountInString(in.Title) > 80 {
		httpkit.Fail(w, 400, "Название идеи: от 3 до 80 символов")
		return
	}
	if !textutil.Valid(in.Description, true) || utf8.RuneCountInString(in.Description) < 10 || utf8.RuneCountInString(in.Description) > 3000 {
		httpkit.Fail(w, 400, "Описание идеи: от 10 до 3000 символов")
		return
	}
	if in.Category != "gameplay" && in.Category != "interface" && in.Category != "economy" && in.Category != "other" {
		httpkit.Fail(w, 400, "Выберите категорию идеи")
		return
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	var author string
	if err = tx.QueryRow(ctx, `SELECT username FROM users WHERE id=$1 FOR UPDATE`, u.ID).Scan(&author); err != nil {
		httpkit.Internal(w, err)
		return
	}
	var saved struct {
		ID                           int64
		Title, Description, Category string
	}
	err = tx.QueryRow(ctx, `SELECT id,title,description,category FROM player_ideas WHERE user_id=$1 AND request_id=$2`, u.ID, in.Request).Scan(&saved.ID, &saved.Title, &saved.Description, &saved.Category)
	if err == nil {
		if saved.Title != in.Title || saved.Description != in.Description || saved.Category != in.Category {
			httpkit.Fail(w, 409, "Этот запрос уже использован для другой идеи")
			return
		}
		httpkit.Reply(w, 200, map[string]any{"id": saved.ID, "saved": true})
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		httpkit.Internal(w, err)
		return
	}
	var recent int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM player_ideas WHERE user_id=$1 AND created_at>now()-interval '1 hour'`, u.ID).Scan(&recent); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if recent >= 10 {
		httpkit.Fail(w, 429, "Можно отправить не более 10 идей за час. Попробуйте позже.")
		return
	}
	var id int64
	if err = tx.QueryRow(ctx, `INSERT INTO player_ideas(user_id,author_name,request_id,category,title,description) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, u.ID, author, in.Request, in.Category, in.Title, in.Description).Scan(&id); err != nil {
		httpkit.Internal(w, err)
		return
	}
	if err = tx.Commit(ctx); err != nil {
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 201, map[string]any{"id": id, "saved": true})
}

// Review changes only moderation state; submissions cannot set their own status.
func (a *Service) Review(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	if u.Role != "admin" {
		httpkit.Fail(w, 403, "Требуется роль администратора")
		return
	}
	var in struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	if !httpkit.Decode(w, r, &in) {
		return
	}
	if in.ID <= 0 || (in.Status != "pending" && in.Status != "approved" && in.Status != "rejected") {
		httpkit.Fail(w, 400, "Некорректный статус идеи")
		return
	}
	tag, err := a.DB.Exec(r.Context(), `UPDATE player_ideas SET status=$2 WHERE id=$1`, in.ID, in.Status)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpkit.Fail(w, 404, "Идея не найдена")
		return
	}
	httpkit.Reply(w, 200, map[string]any{"id": in.ID, "status": in.Status})
}
