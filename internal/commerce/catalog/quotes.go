package tradecatalog

import (
	resourcecatalog "github.com/yomezza/cosmosimulator/internal/resources"
	"github.com/yomezza/cosmosimulator/internal/universe"
)

type Offer struct {
	Resource string `json:"resource"`
	Stock    int    `json:"stock"`
	Buy      int    `json:"buy"`
	Sell     int    `json:"sell"`
}

func marketStock(star universe.Star) map[string]int {
	stock := map[string]int{}
	for _, r := range resourcecatalog.All {
		stock[r.ID] = 30
	}
	for _, r := range universe.AllRegions() {
		if r.ID == star.Region {
			stock[r.Specialty] = 100
		}
	}
	stock["agni"] = 100
	return stock
}

func Offers(star universe.Star, stock map[string]int) []Offer {
	out := []Offer{}
	for _, r := range resourcecatalog.All {
		price := r.Price
		for _, reg := range universe.AllRegions() {
			if reg.ID == star.Region && reg.Specialty == r.ID {
				price = price * 2 / 3
			}
		}
		price += star.ID % 5
		// A reversible trade must never earn money by moving the stock across
		// a price threshold. Regional quotes stay stable between both legs.
		out = append(out, Offer{r.ID, stock[r.ID], price + 2, price})
	}
	return out
}

func StationOffers(star universe.Star, stock map[string]int, goods []string) []Offer {
	allowed := map[string]bool{}
	for _, id := range goods {
		allowed[id] = true
	}
	out := []Offer{}
	for _, o := range Offers(star, stock) {
		if allowed[o.Resource] {
			out = append(out, o)
		}
	}
	return out
}

func FeeOffer(o Offer, percent int) Offer {
	if percent > 0 {
		o.Buy += (o.Buy*percent + 99) / 100
		o.Sell = max(0, o.Sell-(o.Sell*percent+99)/100)
	}
	return o
}
