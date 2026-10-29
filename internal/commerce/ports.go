package commerce

import (
	"context"
	"encoding/json"

	tradecatalog "github.com/yomezza/cosmosimulator/internal/commerce/catalog"
	"github.com/yomezza/cosmosimulator/internal/universe"
)

func (a *Service) globalPorts(ctx context.Context, uid int64, g *universe.Galaxy) ([]map[string]any, error) {
	rows, err := a.DB.Query(ctx, `SELECT s.star,s.object_id,s.name,s.specialization,s.goods,m.stock,COALESCE(m.credits,5000),CASE WHEN c.founder_key='founder-02' AND COALESCE(r.points,0)<20 THEN 2 ELSE 0 END FROM stations s LEFT JOIN local_markets m ON m.star=s.star AND m.station=s.object_id LEFT JOIN companies c ON c.id=s.founder LEFT JOIN reputation r ON r.company_id=s.founder AND r.user_id=$1 WHERE s.active ORDER BY s.star,s.object_id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var star, fee int
		var id, name, spec string
		var goods []string
		var raw []byte
		var credits int64
		if err = rows.Scan(&star, &id, &name, &spec, &goods, &raw, &credits, &fee); err != nil {
			return nil, err
		}
		if star < 1 || star > len(g.Stars) {
			continue
		}
		stock := tradecatalog.StockForStation(g.Stars[star-1], goods)
		if raw != nil {
			stock = map[string]int{}
			if err = json.Unmarshal(raw, &stock); err != nil {
				return nil, err
			}
		}
		list := tradecatalog.StationOffers(g.Stars[star-1], stock, goods)
		for i := range list {
			list[i] = tradecatalog.FeeOffer(list[i], fee)
		}
		out = append(out, map[string]any{"star": star, "station": id, "name": name, "specialization": spec, "credits": credits, "fee_percent": fee, "offers": list})
	}
	return out, rows.Err()
}
