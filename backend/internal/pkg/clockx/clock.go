package clockx

import "time"

// Clock provides a mock-friendly time source.
type Clock interface {
	Now() time.Time
}

// RealClock is the production implementation of Clock.
type RealClock struct{}

// Now returns the current UTC time.
func (RealClock) Now() time.Time {
	return time.Now().UTC()
}
