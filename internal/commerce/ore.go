package commerce

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/wallet"
)

func (a *Service) Sell(w http.ResponseWriter, r *http.Request) {
	u, ok := a.Auth.User(w, r)
	if !ok {
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	if err = wallet.Reason(r.Context(), tx, "training_sale"); err != nil {
		httpkit.Internal(w, err)
		return
	}
	var amount int64
	err = tx.QueryRow(r.Context(), `UPDATE users SET credits=credits+ore*5,ore=0 WHERE id=$1 AND ore>0 RETURNING credits`, u.ID).Scan(&amount)
	if errors.Is(err, pgx.ErrNoRows) {
		httpkit.Fail(w, 409, "Нет руды для продажи")
		return
	}
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 200, map[string]int64{"credits": amount})
}
