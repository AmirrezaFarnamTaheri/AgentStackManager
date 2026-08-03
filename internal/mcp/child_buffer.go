package mcp

import (
	"bytes"
	"sync"
)

type cappedThreadSafeBuffer struct {
	mu        sync.Mutex
	data      bytes.Buffer
	limit     int
	truncated bool
}

func newCappedThreadSafeBuffer(limit int) *cappedThreadSafeBuffer {
	return &cappedThreadSafeBuffer{limit: limit}
}
func (b *cappedThreadSafeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(p)
	remaining := b.limit - b.data.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.data.Write(p[:remaining])
			b.truncated = true
		} else {
			_, _ = b.data.Write(p)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return original, nil
}
func (b *cappedThreadSafeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.String()
}
func (b *cappedThreadSafeBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
