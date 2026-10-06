// Package clock gives the bot its notion of "now". Production uses System;
// tests use Fake so time-dependent behaviour (Follow-up expiry, "today" in
// Tehran, log rate limits) is deterministic.
package clock

import (
	"sync"
	"time"
	_ "time/tzdata" // Asia/Tehran must resolve on every platform, including Windows dev boxes.
)

// Clock returns the current time.
type Clock interface {
	Now() time.Time
}

// System is the real wall clock.
type System struct{}

// Now returns time.Now().
func (System) Now() time.Time { return time.Now() }

// Tehran returns the Asia/Tehran location. It panics if the zone is missing,
// which cannot happen because tzdata is embedded.
func Tehran() *time.Location {
	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		panic(err)
	}
	return loc
}

// Fake is a settable clock for tests. It is safe for concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake returns a Fake clock set to t.
func NewFake(t time.Time) *Fake { return &Fake{now: t} }

// Now returns the fake time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Set moves the clock to t.
func (f *Fake) Set(t time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = t
}

// Advance moves the clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}
