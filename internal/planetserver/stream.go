package planetserver

import (
	"context"
	"strconv"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/yomezza/cosmosimulator/internal/planet/movement"
)

type packet struct {
	kind    string
	payload any
}

// One writer with a deadline for the WHOLE batch. There is no unbounded socket
// queue: motion and sector mailboxes each hold only the newest complete frame.
type stream struct {
	c               *websocket.Conn
	ctx             context.Context
	visit           string
	epoch, sequence int64
	sectorSequence  int64
	known           map[string]movement.PublicActor
}

func (s *stream) send(kind string, payload any) error { return s.batch([]packet{{kind, payload}}) }
func (s *stream) batch(messages []packet) error {
	ctx, cancel := context.WithTimeout(s.ctx, 3*time.Second)
	defer cancel()
	for _, m := range messages {
		s.sequence++
		body := message(m.kind, m.payload)
		body["server_seq"] = s.sequence
		if s.visit != "" {
			body["visit"] = s.visit
			body["connection_epoch"] = s.epoch
		}
		if err := wsjson.Write(ctx, s.c, body); err != nil {
			return err
		}
	}
	return nil
}
func (s *stream) snapshot(m *movement.Member) error {
	v := m.Sector()
	err := s.send("sector.snapshot", map[string]any{"surface": fmtSurface(v.Key.Surface), "sector": v.Key.Sector, "revision": v.Key.Revision, "server_time": v.Time, "actors": v.Actors, "self": m.Snapshot()})
	if err == nil {
		s.known = actorSet(v)
		s.sectorSequence = v.Sequence
	}
	return err
}
func actorSet(v movement.Sector) map[string]movement.PublicActor {
	out := make(map[string]movement.PublicActor, len(v.Actors))
	for _, a := range v.Actors {
		out[a.ID] = a
	}
	return out
}
func samePose(a, b movement.PublicActor) bool {
	return a.Facing == b.Facing && a.X == b.X && a.Y == b.Y && a.Moving == b.Moving && a.Suspended == b.Suspended && a.Nickname == b.Nickname
}
func (s *stream) delta(m *movement.Member, v movement.Sector) error {
	if v.Sequence <= s.sectorSequence {
		return nil
	}
	next := actorSet(v)
	messages := make([]packet, 0)
	for id := range s.known {
		if _, ok := next[id]; !ok {
			messages = append(messages, packet{"actor.leave", map[string]string{"actor": id, "reason": "disconnected"}})
		}
	}
	changes := make([]movement.PublicActor, 0)
	self := m.Stamp().Actor
	for _, a := range v.Actors {
		old, exists := s.known[a.ID]
		if !exists {
			messages = append(messages, packet{"actor.enter", a})
		} else if a.ID != self && !samePose(a, old) {
			changes = append(changes, a)
		}
	}
	state := m.Snapshot()
	state.Path = nil
	messages = append(messages, packet{"state.delta", struct {
		movement.State
		Actors   []movement.PublicActor `json:"actors"`
		Surface  string                 `json:"surface"`
		Sector   int                    `json:"sector"`
		Revision int64                  `json:"revision"`
	}{state, changes, fmtSurface(v.Key.Surface), v.Key.Sector, v.Key.Revision}})
	if err := s.batch(messages); err != nil {
		return err
	}
	s.known = next
	s.sectorSequence = v.Sequence
	return nil
}

func fmtSurface(v int64) string { return strconv.FormatInt(v, 10) }
