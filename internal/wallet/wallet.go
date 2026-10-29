package wallet

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// Technical exact-integer bound shared with browser Number, not a gameplay cap.
const MaxCredits int64 = 9_007_199_254_740_991

func Debit(ctx context.Context, tx pgx.Tx, uid int64, n int64) error {
	if n < 0 || n > MaxCredits {
		return errors.New("Некорректная сумма")
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET credits=credits-$2 WHERE id=$1 AND credits>=$2`, uid, n)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("Недостаточно кредитов")
	}
	return err
}

func Credit(ctx context.Context, tx pgx.Tx, uid int64, n int64) error {
	if n < 0 || n > MaxCredits {
		return errors.New("Некорректная сумма")
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET credits=credits+$2::bigint WHERE id=$1 AND credits<=$3::bigint-$2::bigint`, uid, n, MaxCredits)
	if err == nil && tag.RowsAffected() == 0 {
		return errors.New("Сумма превышает технический диапазон")
	}
	return err
}

func Room(ctx context.Context, tx pgx.Tx, uid, amount int64) (bool, error) {
	var room bool
	err := tx.QueryRow(ctx, `SELECT credits<=$2::bigint-$3::bigint FROM users WHERE id=$1`, uid, MaxCredits, amount).Scan(&room)
	return room, err
}

func Reason(ctx context.Context, tx pgx.Tx, text string) error {
	_, err := tx.Exec(ctx, `SELECT set_config('cosmos.reason',$1,true)`, text)
	return err
}
