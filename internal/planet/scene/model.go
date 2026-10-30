// Package scene builds immutable local scenery from saved cosmic geography.
// It has no database, HTTP, economy or character simulation dependencies.
package scene

import (
	"errors"
	"math"

	"github.com/yomezza/cosmosimulator/internal/geography"
	"github.com/yomezza/cosmosimulator/internal/planet/terrain"
)

const GeneratorVersion = 1
const FormatVersion = 1
const MaxObjects = 2048

// Keep readers for existing versions when a later generator becomes the default.
// Unknown versions fail closed; they are never a reason to reroll a location.
func ReadableGenerator(version int) bool { return version == 1 }

var (
	ErrSource  = errors.New("local scene source changed or unavailable")
	ErrVersion = errors.New("unsupported local scene version; explicit migration required")
	ErrCorrupt = errors.New("invalid local scene snapshot")
	ErrLayout  = errors.New("landing layout is outside the reserved scene area")
)

type Key struct {
	Surface  int64 `json:"surface"`
	Sector   int   `json:"sector"`
	Revision int64 `json:"revision"`
}
type Position struct{ X, Y int }
type Portal struct {
	Neighbor       int `json:"neighbor"`
	WorldX, WorldY float64
	Ingress        Position
}
type Object struct {
	ID       string   `json:"id"`
	Asset    string   `json:"asset"`
	Position Position `json:"position"`
	// Ellipse at the ground anchor, in q units; zero means decorative only.
	RX, RY int
}
type Metadata struct {
	Key                      Key
	Generator, Policy        int
	ID, SourceChecksum, Seed string // private persistence data, never a public DTO
	Biome                    string
	Center                   geography.SurfacePoint // planet-global texture UV origin
	Landing                  *terrain.Placement
	Portals                  []Portal
	Objects                  []Object
}

const (
	inside byte = 1 << iota
	water
	core
	reserved
	walkable
)

// Scene owns its slices. Accessors return values/copies, so pins may share it.
type Scene struct {
	meta    Metadata
	flags   []byte
	heights []uint16
}

func (s *Scene) Metadata() Metadata {
	m := s.meta
	m.Portals = append([]Portal(nil), m.Portals...)
	m.Objects = append([]Object(nil), m.Objects...)
	if m.Landing != nil {
		p := *m.Landing
		m.Landing = &p
	}
	return m
}
func (s *Scene) Key() Key               { return s.meta.Key }
func (s *Scene) ID() string             { return s.meta.ID }
func (s *Scene) SourceChecksum() string { return s.meta.SourceChecksum }
func (s *Scene) cell(x, y int) byte {
	if x < 0 || y < 0 || x >= terrain.Size || y >= terrain.Size {
		return 0
	}
	return s.flags[y*terrain.Size+x]
}
func (s *Scene) Walkable(x, y int) bool { return s.cell(x, y)&walkable != 0 }
func (s *Scene) Reserved(x, y int) bool { return s.cell(x, y)&reserved != 0 }
func (s *Scene) Water(x, y int) bool    { return s.cell(x, y)&water != 0 }
func (s *Scene) Height(x, y int) uint16 {
	if x < 0 || y < 0 || x >= terrain.Size || y >= terrain.Size {
		return 0
	}
	return s.heights[y*terrain.Size+x]
}

// ValidateLanding never moves a cosmic-owned platform or silently rerolls decor.
// The canonical candidate is reserved even before a module exists in this sector.
func (s *Scene) ValidateLanding(p terrain.Placement) error {
	if s.meta.Landing == nil || p != *s.meta.Landing {
		return ErrLayout
	}
	if !s.Walkable(p.SpawnX/terrain.Quantization, p.SpawnY/terrain.Quantization) {
		return ErrLayout
	}
	return nil
}
func quant(p terrain.Point) Position {
	return Position{int(math.Round(p.X * terrain.Quantization)), int(math.Round(p.Y * terrain.Quantization))}
}

// Public contains only presentation data; source seeds/checksums stay server-side.
type Public struct {
	ID      string             `json:"id"`
	Size    int                `json:"size"`
	Cells   []byte             `json:"cells"`
	Objects []Object           `json:"objects"`
	Landing *terrain.Placement `json:"landing"`
}

func (s *Scene) Public() Public {
	p := Public{ID: s.meta.ID, Size: terrain.Size, Cells: make([]byte, len(s.flags)), Objects: s.Metadata().Objects, Landing: s.Metadata().Landing}
	for i, f := range s.flags {
		if f&inside != 0 {
			p.Cells[i] |= 1
		}
		if f&water != 0 {
			p.Cells[i] |= 2
		}
	}
	return p
}
