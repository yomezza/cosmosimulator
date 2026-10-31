package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/yomezza/cosmosimulator/internal/planet/movement"
	"github.com/yomezza/cosmosimulator/internal/planet/visits"
	"github.com/yomezza/cosmosimulator/internal/platform/telemetry"
)

type loadEvent230 struct {
	Type    string
	Payload json.RawMessage
}
type loadClient230 struct {
	c                                                        *websocket.Conn
	cookie                                                   *http.Cookie
	uid, ship                                                int64
	mu, write                                                sync.Mutex
	w                                                        visits.Welcome
	state                                                    movement.State
	actors                                                   map[string]int
	seq                                                      int64
	lastSequence                                             int64
	sent                                                     map[int64]time.Time
	events                                                   chan loadEvent230
	done                                                     chan struct{}
	expected                                                 atomic.Bool
	faults                                                   atomic.Uint64
	closeFaults, sequenceFaults, speedFaults, overflowFaults atomic.Uint64
	bytes                                                    atomic.Uint64
	deltas                                                   atomic.Uint64
	errors                                                   map[string]int
	latency                                                  *telemetry.Samples
}

func dialLoad230(base string, cookie *http.Cookie, uid, ship int64, hello any, latency *telemetry.Samples) (*loadClient230, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h := http.Header{"Cookie": []string{cookie.String()}, "Origin": []string{base}}
	c, _, err := websocket.Dial(ctx, strings.Replace(base, "http://", "ws://", 1)+"/planet/ws/v1", &websocket.DialOptions{HTTPHeader: h})
	if err != nil {
		return nil, fmt.Errorf("WS dial failed")
	}
	c.SetReadLimit(1 << 20)
	if err = wsjson.Write(ctx, c, map[string]any{"v": 1, "type": "hello", "seq": 1, "payload": hello}); err != nil {
		c.CloseNow()
		return nil, err
	}
	var first struct {
		Type     string          `json:"type"`
		Payload  json.RawMessage `json:"payload"`
		Sequence int64           `json:"server_seq"`
	}
	if err = wsjson.Read(ctx, c, &first); err != nil {
		c.CloseNow()
		return nil, fmt.Errorf("WS hello read failed")
	}
	if first.Type != "welcome" {
		c.CloseNow()
		return nil, fmt.Errorf("WS welcome refused")
	}
	v := &loadClient230{c: c, cookie: cookie, uid: uid, ship: ship, seq: 1, lastSequence: first.Sequence, sent: map[int64]time.Time{}, events: make(chan loadEvent230, 256), done: make(chan struct{}), errors: map[string]int{}, actors: map[string]int{}, latency: latency}
	if err = json.Unmarshal(first.Payload, &v.w); err != nil {
		c.CloseNow()
		return nil, err
	}
	v.state = movement.State{X: v.w.X, Y: v.w.Y}
	go v.read()
	return v, nil
}
func (v *loadClient230) read() {
	defer close(v.done)
	for {
		_, raw, err := v.c.Read(context.Background())
		if err != nil {
			if !v.expected.Load() {
				v.faults.Add(1)
				v.closeFaults.Add(1)
			}
			return
		}
		v.bytes.Add(uint64(len(raw)))
		var f struct {
			Type     string          `json:"type"`
			Payload  json.RawMessage `json:"payload"`
			Sequence int64           `json:"server_seq"`
		}
		if json.Unmarshal(raw, &f) != nil {
			v.faults.Add(1)
			continue
		}
		v.mu.Lock()
		if f.Sequence <= v.lastSequence {
			v.faults.Add(1)
			v.sequenceFaults.Add(1)
		}
		v.lastSequence = f.Sequence
		switch f.Type {
		case "state.delta":
			var d struct {
				movement.State
				Actors []movement.PublicActor `json:"actors"`
				Sector int                    `json:"sector"`
			}
			if json.Unmarshal(f.Payload, &d) != nil {
				v.faults.Add(1)
			} else {
				if v.state.Time > 0 && d.Time >= v.state.Time {
					distance := math.Hypot(float64(d.X-v.state.X), float64(d.Y-v.state.Y)) / 1024
					if distance > 4*float64(d.Time-v.state.Time)/1000+.25 {
						v.faults.Add(1)
						v.speedFaults.Add(1)
					}
				}
				v.state = d.State
				for _, a := range d.Actors {
					v.actors[a.ID] = d.Sector
				}
				v.deltas.Add(1)
			}
		case "sector.snapshot":
			var d struct {
				Actors []movement.PublicActor `json:"actors"`
				Self   movement.State         `json:"self"`
				Sector int                    `json:"sector"`
			}
			_ = json.Unmarshal(f.Payload, &d)
			v.state = d.Self
			v.actors = map[string]int{}
			for _, a := range d.Actors {
				if _, duplicate := v.actors[a.ID]; duplicate {
					v.faults.Add(1)
				}
				v.actors[a.ID] = d.Sector
			}
		case "actor.enter":
			var a movement.PublicActor
			_ = json.Unmarshal(f.Payload, &a)
			v.actors[a.ID] = v.w.Sector
		case "actor.leave":
			var a struct {
				Actor string `json:"actor"`
			}
			_ = json.Unmarshal(f.Payload, &a)
			delete(v.actors, a.Actor)
		case "path.accepted":
			var s movement.State
			_ = json.Unmarshal(f.Payload, &s)
			if began, ok := v.sent[s.Seq]; ok && v.latency != nil {
				v.latency.Observe(time.Since(began))
			}
			for n := range v.sent {
				if n <= s.Seq {
					delete(v.sent, n)
				}
			}
		case "error":
			var e struct {
				Code string `json:"code"`
				Seq  int64  `json:"seq"`
			}
			_ = json.Unmarshal(f.Payload, &e)
			v.errors[e.Code]++
			delete(v.sent, e.Seq)
		case "transition.complete":
			var d struct {
				Welcome visits.Welcome `json:"welcome"`
			}
			_ = json.Unmarshal(f.Payload, &d)
			v.w = d.Welcome
			v.actors = map[string]int{}
			v.state = movement.State{X: v.w.X, Y: v.w.Y}
		case "visit.ended":
			// The server closes after delivering the return receipt. The reader
			// may observe EOF before the action goroutine has consumed the receipt.
			v.expected.Store(true)
		}
		v.mu.Unlock()
		if f.Type != "state.delta" && f.Type != "actor.enter" && f.Type != "actor.leave" {
			select {
			case v.events <- loadEvent230{f.Type, f.Payload}:
			default:
				v.faults.Add(1)
				v.overflowFaults.Add(1)
			}
		}
	}
}
func (v *loadClient230) send(kind string, payload any) error {
	v.write.Lock()
	defer v.write.Unlock()
	v.seq++
	v.mu.Lock()
	w := v.w
	if kind == "move" {
		if len(v.sent) > 256 {
			clear(v.sent)
		}
		v.sent[v.seq] = time.Now()
	}
	v.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return wsjson.Write(ctx, v.c, map[string]any{"v": 1, "type": kind, "seq": v.seq, "visit": w.Visit, "connection_epoch": w.Epoch, "payload": payload})
}
func (v *loadClient230) wait(kind string, limit time.Duration) (map[string]any, error) {
	timer := time.NewTimer(limit)
	defer timer.Stop()
	for {
		select {
		case e := <-v.events:
			if e.Type == kind {
				var p map[string]any
				if err := json.Unmarshal(e.Payload, &p); err != nil {
					return nil, err
				}
				return p, nil
			}
			if e.Type == "error" {
				var p struct {
					Code string `json:"code"`
				}
				_ = json.Unmarshal(e.Payload, &p)
				return nil, fmt.Errorf("WS error: %s", p.Code)
			}
		case <-v.done:
			return nil, errors.New("WS closed while waiting for " + kind)
		case <-timer.C:
			return nil, errors.New("WS timed out waiting for " + kind)
		}
	}
}
func (v *loadClient230) welcome() visits.Welcome { v.mu.Lock(); defer v.mu.Unlock(); return v.w }
func (v *loadClient230) close()                  { v.expected.Store(true); v.c.CloseNow(); <-v.done }
func (v *loadClient230) counts() (int, int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	errors := 0
	for _, n := range v.errors {
		errors += n
	}
	return len(v.actors), errors
}
