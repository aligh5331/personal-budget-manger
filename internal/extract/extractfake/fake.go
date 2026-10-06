// Package extractfake is a scripted extract.Extractor for bot-core tests.
package extractfake

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

// Fake returns scripted results in order; once the script runs out it keeps
// returning the last entry. With nothing scripted it returns an error.
type Fake struct {
	mu     sync.Mutex
	script []step
	inputs []string
}

type step struct {
	res extract.Result
	err error
}

var _ extract.Extractor = (*Fake)(nil)

// New returns an empty Fake.
func New() *Fake { return &Fake{} }

// Return queues a successful reading.
func (f *Fake) Return(r extract.Result) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script = append(f.script, step{res: r})
}

// Fail queues a failure (both Metis attempts failed).
func (f *Fake) Fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script = append(f.script, step{err: err})
}

// Reset drops every scripted entry (the last one is otherwise kept for
// good), so a test can script a new reading after sending an Input.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script = nil
}

// Inputs returns every Input text Extract was called with.
func (f *Fake) Inputs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.inputs...)
}

// Extract implements extract.Extractor.
func (f *Fake) Extract(_ context.Context, input string, _ time.Time) (extract.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inputs = append(f.inputs, input)
	if len(f.script) == 0 {
		return extract.Result{}, errors.New("extractfake: nothing scripted")
	}
	s := f.script[0]
	if len(f.script) > 1 {
		f.script = f.script[1:]
	}
	return s.res, s.err
}
