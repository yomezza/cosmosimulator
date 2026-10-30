package planetserver

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/yomezza/cosmosimulator/internal/account"
	"github.com/yomezza/cosmosimulator/internal/planet/access"
	"github.com/yomezza/cosmosimulator/internal/planet/visits"
	"github.com/yomezza/cosmosimulator/internal/platform/httpkit"
	"net/http"
	"strconv"
	"time"
)

type frame struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	Seq     int64           `json:"seq"`
	Visit   string          `json:"visit,omitempty"`
	Epoch   int64           `json:"connection_epoch,omitempty"`
	Payload json.RawMessage `json:"payload"`
}

func message(kind string, payload any) map[string]any {
	return map[string]any{"v": 1, "type": kind, "payload": payload}
}
func publicError(err error) string {
	switch {
	case errors.Is(err, access.ErrAuth), errors.Is(err, access.ErrDenied), errors.Is(err, visits.ErrUsed), errors.Is(err, visits.ErrExpired), errors.Is(err, visits.ErrReplaced), errors.Is(err, visits.ErrVisit):
		return err.Error()
	default:
		return "runtime_unavailable"
	}
}
func (a *App) websocket(w http.ResponseWriter, r *http.Request) {
	a.wsMu.Lock()
	if a.stopping.Load() || a.runContext == nil {
		a.wsMu.Unlock()
		httpkit.Fail(w, 503, "runtime_unavailable")
		return
	}
	a.wsWG.Add(1)
	a.wsMu.Unlock()
	defer a.wsWG.Done()
	connection, cancelConnection := context.WithCancel(r.Context())
	stopConnection := context.AfterFunc(a.runContext, cancelConnection)
	defer stopConnection()
	defer cancelConnection()
	r = r.WithContext(connection)

	if a.scenes == nil || a.stopping.Load() {
		a.health(w, 503, "runtime_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.config.DBTimeout)
	id, err := access.Authenticate(ctx, a.db, r)
	if err == nil && a.coordinator.Ready(ctx) != "" {
		err = errors.New("runtime unavailable")
	}
	cancel()
	if err != nil {
		code := 503
		if errors.Is(err, access.ErrAuth) {
			code = 401
		}
		httpkit.Fail(w, code, publicError(err))
		return
	}
	a.wsMu.Lock()
	if a.connections >= 256 || a.controllers[id.User] >= 4 {
		a.wsMu.Unlock()
		httpkit.Fail(w, 429, "connection_limit")
		return
	}
	a.connections++
	a.controllers[id.User]++
	a.wsMu.Unlock()
	defer func() {
		a.wsMu.Lock()
		a.connections--
		a.controllers[id.User]--
		if a.controllers[id.User] == 0 {
			delete(a.controllers, id.User)
		}
		a.wsMu.Unlock()
	}()
	// Exact Origin validation has already run in Handler; an empty browser Origin
	// is not an authorization mechanism. Cookies and a ticket/resume are required.
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true, CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		return
	}
	defer c.CloseNow()
	c.SetReadLimit(4096)
	auth, done := context.WithTimeout(r.Context(), 5*time.Second)
	var hello frame
	err = wsjson.Read(auth, c, &hello)
	done()
	if err != nil || hello.V != 1 || hello.Type != "hello" || hello.Seq != 1 {
		c.CloseNow()
		return
	}
	var input struct {
		Ticket string `json:"ticket"`
		Resume string `json:"resume_visit"`
	}
	if json.Unmarshal(hello.Payload, &input) != nil || (input.Ticket == "") == (input.Resume == "") || len(input.Ticket) > 64 {
		c.CloseNow()
		return
	}
	prepare, finish := context.WithTimeout(r.Context(), a.config.SceneLoadTimeout)
	hash := ""
	if input.Ticket != "" {
		if len(input.Ticket) != 64 {
			finish()
			return
		}
		hash = account.TokenHash(input.Ticket)
	}
	in, err := a.visits.Resolve(prepare, id, hash, input.Resume)
	var pinRelease func()
	var welcome visits.Welcome
	if err == nil {
		pin, e := a.scenes.Acquire(prepare, in.Key)
		err = e
		if err == nil {
			pinRelease = pin.Release
			f, e := a.sceneFence()
			err = e
			if err == nil {
				welcome, err = a.visits.Activate(prepare, id, in, pin.Scene(), f)
			}
		}
	}
	finish()
	if pinRelease != nil {
		defer pinRelease()
	}
	send := func(kind string, payload any) error {
		write, done := context.WithTimeout(r.Context(), 3*time.Second)
		defer done()
		return wsjson.Write(write, c, message(kind, payload))
	}
	if err != nil {
		_ = send("error", map[string]string{"code": publicError(err)})
		c.CloseNow()
		return
	}
	f, err := a.sceneFence()
	if err != nil {
		return
	}
	defer func() {
		save, done := context.WithTimeout(context.Background(), a.config.DBTimeout)
		defer done()
		_ = a.visits.Disconnect(save, id, welcome, f)
	}()
	if send("welcome", welcome) != nil {
		return
	}
	run, stop := context.WithCancel(r.Context())
	defer stop()
	received := make(chan frame, 1)
	readErrors := make(chan error, 1)
	go func() {
		for {
			var msg frame
			if e := wsjson.Read(run, c, &msg); e != nil {
				select {
				case readErrors <- e:
				case <-run.Done():
				}
				return
			}
			select {
			case received <- msg:
			case <-run.Done():
				return
			}
		}
	}()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	var seq int64 = 1
	lastCommand := time.Now().Add(-time.Second)
	for {
		select {
		case <-run.Done():
			return
		case <-readErrors:
			return
		case msg := <-received:
			if time.Since(lastCommand) < time.Second {
				c.CloseNow()
				return
			}
			lastCommand = time.Now()
			if msg.V != 1 || msg.Seq <= seq || msg.Visit != welcome.Visit || msg.Epoch != welcome.Epoch {
				c.CloseNow()
				return
			}
			seq = msg.Seq
			if msg.Type != "resync" {
				_ = send("error", map[string]string{"code": "movement_not_available"})
				c.CloseNow()
				return
			}
			if seq > 120 {
				c.CloseNow()
				return
			}
			check, done := context.WithTimeout(run, a.config.DBTimeout)
			err = a.visits.Check(check, id, welcome, f)
			done()
			if err != nil {
				return
			}
			if send("welcome", welcome) != nil {
				return
			}
		case <-tick.C:
			check, done := context.WithTimeout(run, a.config.DBTimeout)
			err = a.visits.Check(check, id, welcome, f)
			done()
			if err != nil {
				_ = send("error", map[string]string{"code": publicError(err)})
				c.CloseNow()
				return
			}
		}
	}
}
func (a *App) sceneView(w http.ResponseWriter, r *http.Request) {
	if a.scenes == nil || a.stopping.Load() {
		httpkit.Fail(w, 503, "runtime_unavailable")
		return
	}
	ctx, done := context.WithTimeout(r.Context(), a.config.SceneLoadTimeout)
	defer done()
	id, err := access.Authenticate(ctx, a.db, r)
	if err != nil {
		httpkit.Fail(w, 401, publicError(err))
		return
	}
	v := r.URL.Query().Get("visit")
	epoch, err := strconv.ParseInt(r.URL.Query().Get("epoch"), 10, 64)
	if err != nil || !access.ValidUUID(v) || epoch < 1 {
		httpkit.Fail(w, 400, "invalid_visit")
		return
	}
	in, err := a.visits.Resolve(ctx, id, "", v)
	if err != nil {
		httpkit.Fail(w, 403, publicError(err))
		return
	}
	f, err := a.sceneFence()
	if err != nil {
		httpkit.Fail(w, 503, "runtime_unavailable")
		return
	}
	welcome := visits.Welcome{Visit: v, Epoch: epoch}
	if err = a.visits.Check(ctx, id, welcome, f); err != nil {
		httpkit.Fail(w, 403, publicError(err))
		return
	}
	pin, err := a.scenes.Acquire(ctx, in.Key)
	if err != nil {
		httpkit.Fail(w, 503, "scene_unavailable")
		return
	}
	defer pin.Release()
	if err = a.visits.Check(ctx, id, welcome, f); err != nil {
		httpkit.Fail(w, 403, publicError(err))
		return
	}
	// No ticket, seed or private geology in this DTO or URL.
	httpkit.Reply(w, 200, pin.Scene().Public())
}
