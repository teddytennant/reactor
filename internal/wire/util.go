package wire

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"time"

	"github.com/reactor-sec/reactor/internal/mcpjson"
)

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func nowMs() int64 { return time.Now().UnixMilli() }

// WriteFrame re-exports the mcpjson framer for cmd/wire.
func WriteFrame(w io.Writer, raw []byte) error { return mcpjson.WriteFrame(w, raw) }
