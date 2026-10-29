package gameplay

import (
	"context"
	"fmt"
	"time"

	"github.com/yomezza/cosmosimulator/internal/platform/postgres"
)

type Settings struct {
	TickSeconds      int       `json:"tick_seconds"`
	DateStep         int       `json:"date_step"`
	Common           int       `json:"quota_common"`
	RareMin          int       `json:"rare_min"`
	RareMax          int       `json:"rare_max"`
	ResetSeconds     int       `json:"reset_seconds"`
	Epoch            time.Time `json:"epoch"`
	Generation       int64     `json:"generation"`
	ScanSeconds      int       `json:"scan_seconds"`
	CompanyCost      int64     `json:"company_cost"`
	RescueCost       int       `json:"rescue_cost"`
	ReservationHours int       `json:"reservation_hours"`
}

func ReadGameplay(ctx context.Context, q postgres.Querier) (Settings, error) {
	var g Settings
	err := q.QueryRow(ctx, `SELECT tick_seconds,date_step,quota_common,rare_min,rare_max,reset_seconds,quota_epoch,generation,scan_seconds,company_cost,rescue_cost,reservation_hours FROM game_settings WHERE id=1`).Scan(&g.TickSeconds, &g.DateStep, &g.Common, &g.RareMin, &g.RareMax, &g.ResetSeconds, &g.Epoch, &g.Generation, &g.ScanSeconds, &g.CompanyCost, &g.RescueCost, &g.ReservationHours)
	return g, err
}

func EdemDate(pulses int64) string {
	return fmt.Sprintf("ЭД %04d·%03d·%03d", 1+pulses/360000, 1+(pulses/1000)%360, pulses%1000)
}
