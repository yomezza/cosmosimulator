package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func Digest(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
