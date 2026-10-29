package integration

import (
	"testing"
)

func TestTransientAcknowledgementsDoNotFillInbox(t *testing.T) {
	f := newFixture033(t)
	cookie, uid := f.signup("notifications035")
	for _, message := range []string{"Действие выполнено", "Тик выполнен", "Action completed", "Tick advanced", " Тик выполнен "} {
		f.req(cookie, "POST", "/api/notifications", map[string]any{
			"action": "add", "message": message, "level": "success",
		}, 200)
	}
	if count := f.integer(`SELECT count(*) FROM notifications WHERE user_id=$1`, uid); count != 0 {
		t.Fatal("transient acknowledgements saved", count)
	}

	// Actual events and other existing messages must survive the cleanup.
	f.exec(`INSERT INTO game_events(user_id,kind,data) VALUES($1,'arrival','{}'),($1,'mining_complete','{}')`, uid)
	f.req(cookie, "POST", "/api/notifications", map[string]any{
		"action": "add", "message": "Important warning", "level": "warning",
	}, 200)
	f.exec(`INSERT INTO notifications(user_id,kind,message) VALUES
	 ($1,'message','Действие выполнено'),($1,'message','Тик выполнен'),
	 ($1,'message','Action completed'),($1,'message','Tick advanced'),
	 ($1,'arrival','Действие выполнено')`, uid)
	var schema string
	if err := f.db.QueryRow(f.ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		db, err := openDatabase(f.ctx, f.db.Config().ConnString(), schema)
		if err != nil {
			t.Fatal(err)
		}
		db.Close()
		if count := f.integer(`SELECT count(*) FROM notifications WHERE user_id=$1`, uid); count != 4 {
			t.Fatal("migration removed meaningful notifications or left acknowledgements", count)
		}
		if count := f.integer(`SELECT count(*) FROM notifications WHERE user_id=$1 AND kind IN('arrival','mining_complete')`, uid); count != 3 {
			t.Fatal("gameplay events lost", count)
		}
	}
}
