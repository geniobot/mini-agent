package tools

import "bytes"

// limitBuf is a bytes.Buffer that caps writes at limit bytes.
// This prevents runaway command output from filling RAM on constrained hardware.
type limitBuf struct {
	bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitBuf) Write(p []byte) (int, error) {
	if b.Len() >= b.limit {
		b.truncated = true
		return len(p), nil
	}
	room := b.limit - b.Len()
	if len(p) > room {
		b.truncated = true
		if _, err := b.Buffer.Write(p[:room]); err != nil {
			return 0, err
		}
		return len(p), nil
	}
	return b.Buffer.Write(p)
}
