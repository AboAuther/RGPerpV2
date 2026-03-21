package idgen

import (
	"fmt"
	"sync/atomic"
	"time"
)

// Generator provides a mock-friendly ID generation abstraction.
type Generator interface {
	NewID(prefix string) string
}

// TimeBasedGenerator is a deterministic-enough generator for service IDs.
type TimeBasedGenerator struct {
	counter atomic.Uint64
}

// NewID returns a prefixed ID suitable for business objects and traces.
func (g *TimeBasedGenerator) NewID(prefix string) string {
	seq := g.counter.Add(1)
	return fmt.Sprintf("%s_%d_%06d", prefix, time.Now().UTC().UnixMilli(), seq)
}
