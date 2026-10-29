package simulation

import (
	"context"
)

func (a *Service) Activities(ctx context.Context) error {
	g, err := a.World.Load(ctx)
	if err != nil {
		return err
	}
	rows, err := a.DB.Query(ctx, `SELECT DISTINCT u.id FROM users u LEFT JOIN ships s ON s.user_id=u.id LEFT JOIN research_jobs r ON r.user_id=u.id WHERE s.arrival<=now() OR s.local_arrival<=now() OR (NOT r.paused AND r.next_at<=now()) ORDER BY u.id`)
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err = a.Navigation.AdvancePlayer(ctx, id, g); err != nil {
			return err
		}
	}
	if err = a.Commerce.AdvanceLogistics(ctx); err != nil {
		return err
	}
	return a.Planner.AdvanceAutomation(ctx)
}
