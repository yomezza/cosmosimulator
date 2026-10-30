package scene

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"

	"github.com/yomezza/cosmosimulator/internal/planet/terrain"
)

const maxPacked = 2 << 20
const maxHeader = 1 << 20

// Encode stores all immutable data, including objects. Loading never has to run
// today's decor generator on yesterday's seed. Heights and flags use 3 bytes/cell.
func Encode(s *Scene) ([]byte, [32]byte, error) {
	var zero [32]byte
	header, err := json.Marshal(s.meta)
	if err != nil {
		return nil, zero, err
	}
	if len(header) > maxHeader || len(s.flags) != terrain.Size*terrain.Size || len(s.heights) != len(s.flags) {
		return nil, zero, ErrCorrupt
	}
	var out bytes.Buffer
	z := zlib.NewWriter(&out)
	_, err = z.Write([]byte{'P', 'S', 'C', FormatVersion})
	if err != nil {
		return nil, zero, err
	}
	if err = binary.Write(z, binary.LittleEndian, uint32(len(header))); err != nil {
		return nil, zero, err
	}
	if _, err = z.Write(header); err != nil {
		return nil, zero, err
	}
	if _, err = z.Write(s.flags); err != nil {
		return nil, zero, err
	}
	if err = binary.Write(z, binary.LittleEndian, s.heights); err != nil {
		return nil, zero, err
	}
	if err = z.Close(); err != nil {
		return nil, zero, err
	}
	b := out.Bytes()
	return b, sha256.Sum256(b), nil
}

func Decode(data []byte, checksum [32]byte) (*Scene, error) {
	if len(data) > maxPacked || sha256.Sum256(data) != checksum {
		return nil, ErrCorrupt
	}
	z, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, ErrCorrupt
	}
	defer z.Close()
	raw, err := io.ReadAll(io.LimitReader(z, maxHeader+8+terrain.Size*terrain.Size*3+1))
	if err != nil || len(raw) < 8 {
		return nil, ErrCorrupt
	}
	if !bytes.Equal(raw[:3], []byte("PSC")) || raw[3] != FormatVersion {
		return nil, ErrVersion
	}
	n := int(binary.LittleEndian.Uint32(raw[4:8]))
	if n > maxHeader || len(raw) != 8+n+terrain.Size*terrain.Size*3 {
		return nil, ErrCorrupt
	}
	s := &Scene{flags: append([]byte(nil), raw[8+n:8+n+terrain.Size*terrain.Size]...), heights: make([]uint16, terrain.Size*terrain.Size)}
	if json.Unmarshal(raw[8:8+n], &s.meta) != nil {
		return nil, ErrCorrupt
	}
	if !ReadableGenerator(s.meta.Generator) || s.meta.Policy != 1 {
		return nil, ErrVersion
	}
	if s.meta.Key.Surface <= 0 || s.meta.Key.Sector <= 0 || s.meta.Key.Revision < 0 || len(s.meta.ID) != 32 || len(s.meta.SourceChecksum) != 64 || len(s.meta.Seed) != 64 || math.IsNaN(s.meta.Center.X+s.meta.Center.Y) || math.IsInf(s.meta.Center.X+s.meta.Center.Y, 0) || len(s.meta.Objects) > MaxObjects || len(s.meta.Portals) > 6 {
		return nil, ErrCorrupt
	}
	if binary.Read(bytes.NewReader(raw[8+n+len(s.flags):]), binary.LittleEndian, s.heights) != nil {
		return nil, ErrCorrupt
	}
	for _, f := range s.flags {
		if f & ^byte(inside|water|core|reserved|walkable) != 0 || f&walkable != 0 && (f&core == 0 || f&inside == 0 || f&water != 0) {
			return nil, ErrCorrupt
		}
	}
	ids := map[string]bool{}
	validQ := func(p Position) bool {
		return p.X >= 0 && p.Y >= 0 && p.X < terrain.Size*terrain.Quantization && p.Y < terrain.Size*terrain.Quantization
	}
	for _, o := range s.meta.Objects {
		if ids[o.ID] || o.ID == "" || !validQ(o.Position) || !knownAsset(o.Asset) || o.RX < 0 || o.RY < 0 || o.RX > 8192 || o.RY > 8192 {
			return nil, ErrCorrupt
		}
		ids[o.ID] = true
	}
	for _, p := range s.meta.Portals {
		if p.Neighbor <= 0 || p.Neighbor == s.meta.Key.Sector || !validQ(p.Ingress) || math.IsNaN(p.WorldX+p.WorldY) || math.IsInf(p.WorldX+p.WorldY, 0) || !s.Walkable(p.Ingress.X/1024, p.Ingress.Y/1024) {
			return nil, ErrCorrupt
		}
	}
	if p := s.meta.Landing; p != nil {
		if !validQ(Position{p.X, p.Y}) || !validQ(Position{p.SpawnX, p.SpawnY}) || s.ValidateLanding(*p) != nil {
			return nil, ErrCorrupt
		}
	}
	return s, nil
}

func knownAsset(s string) bool {
	switch s {
	case "nature/tree_old.png", "nature/tree_young.png", "nature/bush_sparse.png", "nature/grass_tuft.png", "nature/rock_flat.png", "nature/rock_large_cracked.png", "nature/grass_dry.png":
		return true
	}
	return false
}
