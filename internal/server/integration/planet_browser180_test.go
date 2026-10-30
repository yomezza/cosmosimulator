package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/yomezza/cosmosimulator/internal/planetserver"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestPlanetBrowser180(t *testing.T) {
	if os.Getenv("RUN_PLANET_BROWSER") != "1" {
		t.Skip("set RUN_PLANET_BROWSER=1 with Node/Playwright")
	}
	f, c, _, _, site, _ := prepare180(t)
	web := httptest.NewUnstartedServer(http.NotFoundHandler())
	defer web.Close()
	base := "http://" + web.Listener.Addr().String()
	cfg := planetserver.DefaultConfig()
	cfg.AllowedOrigins = []string{base}
	app, err := planetserver.New(f.db, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	run, cancel := context.WithCancel(f.ctx)
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() { done <- app.Serve(run, listener, func() { close(started) }) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	<-started
	t.Setenv("PLANET_INTERNAL_URL", "http://"+listener.Addr().String())
	nonce := fmt.Sprintf("terminal-test-%d", time.Now().UnixNano())
	handler := f.app.Handler()
	web.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Cosmos-Test-Run", nonce)
		handler.ServeHTTP(w, r)
	})
	web.Start()
	script, err := filepath.Abs("../../../tests/ui/planet-live.mjs")
	if err != nil {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(f.ctx, 90*time.Second)
	defer stop()
	cmd := exec.CommandContext(ctx, "node", script)
	input, _ := json.Marshal(map[string]any{"cookie": c.Value, "site": fmt.Sprint(site)})
	cmd.Stdin = bytes.NewReader(input)
	cmd.Env = append(os.Environ(), "TERMINAL_TEST_URL="+web.URL, "TERMINAL_TEST_NONCE="+nonce)
	out, err := cmd.CombinedOutput()
	t.Log(string(out))
	if err != nil {
		t.Fatal(err)
	}
}
