package runner

import (
	"sync"
)

type limitedBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func newLimitedBuffer(limit int) *limitedBuffer {
	if limit <= 0 {
		limit = 1 << 20
	}
	return &limitedBuffer{limit: limit}
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	original := len(p)
	remaining := b.limit - len(b.data)
	if remaining > 0 {
		if len(p) > remaining {
			b.data = append(b.data, p[:remaining]...)
			b.truncated = true
		} else {
			b.data = append(b.data, p...)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return original, nil
}

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(append([]byte(nil), b.data...))
}

func (b *limitedBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.truncated
}
