package planetserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestSlowStreamDeadline(t *testing.T) {
	result := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			result <- err
			return
		}
		defer c.CloseNow()
		// A real peer stops reading. Even a batch of reliable events has ONE budget.
		out := stream{ctx: r.Context(), c: c}
		messages := make([]packet, 32)
		text := strings.Repeat("x", 1<<20)
		for i := range messages {
			messages[i] = packet{"actor.enter", text}
		}
		result <- out.batch(messages)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, strings.Replace(server.URL, "http:", "ws:", 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.CloseNow()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("stalled stream accepted all data")
		}
	case <-ctx.Done():
		t.Fatal("slow reader retained writer")
	}
}
