package account

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"github.com/yomezza/cosmosimulator/internal/wallet"
)

func (a *Service) Profile(w http.ResponseWriter, r *http.Request) {
	u, ok := a.User(w, r)
	if !ok {
		return
	}
	var level, xp int
	var company *int64
	var name *string
	var tag, color, flag *string
	var flagDesign []byte
	err := a.DB.QueryRow(r.Context(), `SELECT u.level,u.xp,u.company_id,c.name,c.tag,c.color,c.flag,COALESCE(c.flag_design,'{}'::jsonb) FROM users u LEFT JOIN companies c ON c.id=u.company_id WHERE u.id=$1`, u.ID).Scan(&level, &xp, &company, &name, &tag, &color, &flag, &flagDesign)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 200, map[string]any{"user": u, "level": level, "xp": xp, "required_xp": level * 100, "upgrade_cost": level * 100, "company_id": company, "company": name, "company_tag": tag, "company_color": color, "company_flag": flag, "company_flag_design": json.RawMessage(flagDesign)})
}

func (a *Service) Upgrade(w http.ResponseWriter, r *http.Request) {
	u, ok := a.User(w, r)
	if !ok {
		return
	}
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	defer tx.Rollback(context.Background())
	if err = wallet.Reason(r.Context(), tx, "level_up"); err != nil {
		httpkit.Internal(w, err)
		return
	}
	tag, err := tx.Exec(r.Context(), `WITH changed AS(UPDATE users SET xp=xp-level*100,credits=credits-level*100,level=level+1 WHERE id=$1 AND level<1000000 AND xp>=level*100 AND credits>=level*100 RETURNING id,level) INSERT INTO game_events(user_id,kind,data) SELECT id,'level_up',jsonb_build_object('level',level) FROM changed`, u.ID)
	if err != nil {
		httpkit.Internal(w, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpkit.Fail(w, 409, "Недостаточно опыта или кредитов")
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpkit.Internal(w, err)
		return
	}
	httpkit.Reply(w, 200, map[string]bool{"ok": true})
}
