package server

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

type ShipClass struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role"`
	Description string `json:"description"`
	Cargo       int    `json:"cargo"`
	Fuel        int    `json:"fuel"`
	JumpSeconds int    `json:"jump_seconds"`
	JumpFuel    int    `json:"jump_fuel"`
	MiningBonus int    `json:"mining_bonus"`
	Price       int64  `json:"price"`
	Model       string `json:"model"`
}

var shipClasses = []ShipClass{
	{"pilgrim", "Пилигрим", "Универсальный", "Начальный корабль для первых экспедиций, торговли и добычи.", 60, 40, 5, 2, 0, 10000, "pilgrim.svg"},
	{"wanderer", "Скиталец", "Разведчик", "Быстрый разведчик с большим баком и компактным трюмом.", 35, 80, 3, 2, 0, 28000, "wanderer.svg"},
	{"forge", "Горн", "Добывающий", "Промышленный корпус с усиленной добывающей установкой: +2 единицы за цикл.", 120, 60, 6, 3, 2, 65000, "forge.svg"},
	{"ark", "Ковчег", "Грузовой", "Вместительный транспорт для крупных партий. Медленнее и расходует больше топлива.", 240, 80, 7, 4, 0, 95000, "ark.svg"},
	{"hermes", "Гермес", "Курьер", "Дальнобойный курьер для срочных грузов: большой бак и умеренный трюм.", 90, 100, 4, 2, 0, 110000, "hermes.svg"},
}

func hull(id string) (ShipClass, bool) {
	for _, c := range shipClasses {
		if c.ID == id {
			return c, true
		}
	}
	return shipClasses[0], false
}
func (s Ship) hull() ShipClass { c, _ := hull(s.Class); return c }
func (s Ship) capacity() int   { return s.hull().Cargo }
func buyShip(ctx context.Context, tx pgx.Tx, u User, s Ship, in Command) error {
	if !docked(s) {
		return errors.New("Стыкуйтесь с верфью")
	}
	if in.Star > 0 && (in.Star != s.Star || in.Station != s.ObjectID) {
		return errors.New("Прибудьте на выбранную верфь")
	}
	id := in.Target
	if id == "" {
		id = "pilgrim"
	}
	c, ok := hull(id)
	if !ok {
		return errors.New("Неизвестная модель корабля")
	}
	var price int64
	var quantity int
	err := tx.QueryRow(ctx, `SELECT y.price,y.quantity FROM shipyard_stock y JOIN stations p ON p.star=y.star AND p.object_id=y.station WHERE y.star=$1 AND y.station=$2 AND y.ship_class=$3 AND p.shipyard AND p.active FOR UPDATE OF y`, s.Star, s.ObjectID, id).Scan(&price, &quantity)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && quantity < 1 {
		return errors.New("На верфи нет доступных кораблей")
	}
	if err != nil {
		return err
	}
	if in.Price != 0 && in.Price != price {
		return errors.New("Цена изменилась. Проверьте предложение ещё раз")
	}
	if err = debit(ctx, tx, u.ID, price); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE shipyard_stock SET quantity=quantity-1 WHERE star=$1 AND station=$2 AND ship_class=$3`, s.Star, s.ObjectID, id); err != nil {
		return err
	}
	if id == "pilgrim" {
		if _, err = tx.Exec(ctx, `UPDATE stations SET ship_stock=GREATEST(0,ship_stock-1) WHERE star=$1 AND object_id=$2`, s.Star, s.ObjectID); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `INSERT INTO ships(user_id,star,object_id,active,class,name,fuel) VALUES($1,$2,$3,FALSE,$4,$5,$6)`, u.ID, s.Star, s.ObjectID, c.ID, c.Name, c.Fuel)
	return err
}
