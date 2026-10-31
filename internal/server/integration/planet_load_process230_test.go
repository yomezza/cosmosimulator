package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// Owned child processes only. No target URL, PID or schema is accepted from CLI.
type loadProcess230 struct {
	cmd  *exec.Cmd
	log  *os.File
	done chan error
	once sync.Once
}

func startLoadProcess230(t *testing.T, binary string, env map[string]string) *loadProcess230 {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "process-*.log")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary)
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		if _, set := env[k]; !set {
			cmd.Env = append(cmd.Env, v)
		}
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Stdout, cmd.Stderr = f, f
	if err = cmd.Start(); err != nil {
		f.Close()
		t.Fatal("child start failed:", err)
	}
	p := &loadProcess230{cmd: cmd, log: f, done: make(chan error, 1)}
	go func() { p.done <- cmd.Wait() }()
	t.Cleanup(func() {
		p.stop(t, true)
		if t.Failed() {
			data, _ := os.ReadFile(f.Name())
			text := string(data)
			for k, v := range env {
				if v != "" && (k == "DATABASE_URL" || strings.Contains(k, "PASSWORD")) {
					text = strings.ReplaceAll(text, v, "[redacted]")
				}
			}
			t.Logf("child diagnostic: %s", text)
		}
		f.Close()
	})
	return p
}
func (p *loadProcess230) stop(t *testing.T, crash bool) {
	t.Helper()
	p.once.Do(func() {
		if crash {
			_ = p.cmd.Process.Kill()
		} else {
			_ = p.cmd.Process.Signal(os.Interrupt)
		}
		select {
		case err := <-p.done:
			if !crash && err != nil {
				t.Error("child did not stop cleanly:", err)
			}
		case <-time.After(12 * time.Second):
			_ = p.cmd.Process.Kill()
			<-p.done
			t.Error("child shutdown exceeded deadline")
		}
	})
}
func buildLoad230(t *testing.T, command string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), command)
	c := exec.Command("go", "build", "-buildvcs=false", "-trimpath", "-o", out, "./cmd/"+command)
	c.Dir = "../../.."
	if data, err := c.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, data)
	}
	return out
}
func loadPort230(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return "http://" + addr
}

var loadHTTP230 = &http.Client{Timeout: 25 * time.Second}

func loadRequest230(base string, cookie *http.Cookie, method, path string, body any) (map[string]any, int, error) {
	var input io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		input = bytes.NewReader(raw)
	}
	r, err := http.NewRequest(method, base+path, input)
	if err != nil {
		return nil, 0, err
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if method != http.MethodGet {
		r.Header.Set("Origin", base)
	}
	r.Header.Set("Content-Type", "application/json")
	res, err := loadHTTP230.Do(r)
	if err != nil {
		return nil, 0, fmt.Errorf("HTTP unavailable")
	}
	defer res.Body.Close()
	var data map[string]any
	err = json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&data)
	return data, res.StatusCode, err
}
func requireLoad230(t *testing.T, base string, cookie *http.Cookie, method, path string, body any, want int) map[string]any {
	t.Helper()
	v, status, err := loadRequest230(base, cookie, method, path, body)
	if err != nil || status != want {
		t.Fatalf("%s %s: HTTP %d (want %d), decode=%v", method, path, status, want, err)
	}
	return v
}
func waitLoadReady230(t *testing.T, base, path string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		_, s, e := loadRequest230(base, nil, "GET", path, nil)
		if e == nil && s == 200 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("child readiness deadline exceeded")
}

type processUsage230 struct {
	CPUSeconds float64 `json:"cpu_seconds"`
	RSSBytes   int64   `json:"rss_bytes"`
}

func processUsageLoad230(pid int) processUsage230 {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return processUsage230{}
	}
	_, tail, _ := strings.Cut(string(data), ") ")
	fields := strings.Fields(tail)
	if len(fields) < 22 {
		return processUsage230{}
	}
	u, _ := strconv.ParseFloat(fields[11], 64)
	s, _ := strconv.ParseFloat(fields[12], 64)
	rss, _ := strconv.ParseInt(fields[21], 10, 64)
	return processUsage230{(u + s) / 100, rss * int64(os.Getpagesize())} // Linux USER_HZ=100.
}
func waitConditionLoad230(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("load scenario condition deadline exceeded")
}
func loadParallel230(t *testing.T, n int, fn func(int) error) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := fn(i); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if t.Failed() {
		t.FailNow()
	}
}

// No credentials or process output is included in the published report.
func writeLoadReport230(t *testing.T, value any) {
	t.Helper()
	path := os.Getenv("PLANET_LOAD_REPORT")
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Error(err)
		return
	}
	if err = os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Error(err)
	}
}
