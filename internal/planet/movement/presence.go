package movement

import (
	"sort"
	"time"

	"github.com/yomezza/cosmosimulator/internal/planet/scene"
)

// PublicActor deliberately excludes account IDs, visits, sessions and paths.
// The slice in Sector is immutable after publication, shared by subscribers.
type PublicActor struct {
	ID        string `json:"actor"`
	Nickname  string `json:"nickname"`
	Sprite    string `json:"sprite"`
	X         int    `json:"x_q"`
	Y         int    `json:"y_q"`
	Moving    bool   `json:"moving"`
	Suspended bool   `json:"suspended"`
	Time      int64  `json:"server_time"`
}
type Sector struct {
	Key      scene.Key
	Sequence int64
	Time     int64
	Actors   []PublicActor
}

func (m *Member) Presence() <-chan Sector { return m.presence }
func (m *Member) Sector() Sector {
	m.hub.mu.Lock()
	defer m.hub.mu.Unlock()
	return m.hub.sector(m.actor, time.Now())
}
func (h *Hub) sector(a *actor, now time.Time) Sector {
	v := Sector{Key: a.scene.Key(), Sequence: a.sequence, Time: now.UnixMilli(), Actors: make([]PublicActor, 0, len(a.members))}
	for _, m := range a.members {
		x, y := m.pos.Quantized()
		v.Actors = append(v.Actors, PublicActor{ID: m.stamp.Actor, Nickname: m.stamp.Nickname, Sprite: "colonist", X: x, Y: y, Moving: len(m.path) > 0 && !m.frozen && now.Before(m.permit), Suspended: m.frozen || !now.Before(m.permit), Time: v.Time})
	}
	sort.Slice(v.Actors, func(i, j int) bool { return v.Actors[i].ID < v.Actors[j].ID })
	return v
}
func (h *Hub) broadcast(a *actor, now time.Time) {
	a.sequence++
	v := h.sector(a, now)
	for _, m := range a.members {
		// Latest complete membership replaces an old frame. A receiver derives
		// enter/leave from its LAST DELIVERED set, so drops cannot leave ghosts.
		select {
		case m.presence <- v:
		default:
			select {
			case <-m.presence:
			default:
			}
			select {
			case m.presence <- v:
			default:
			}
		}
	}
}
