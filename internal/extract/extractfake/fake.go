// Package extractfake is a scripted extract.Extractor for bot-core tests.
package extractfake

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/extract"
)

// Fake returns scripted results in order. Asking for more readings than were
// queued fails the test, unless the test called Repeat to say the last
// reading should answer every further call. With nothing scripted it returns
// an error.
type Fake struct {
	mu     sync.Mutex
	t      reporter
	script []step
	next   int
	repeat bool
	inputs []string
}

// reporter is the part of testing.TB the fake needs.
type reporter interface {
	Helper()
	Errorf(format string, args ...any)
}

type step struct {
	res extract.Result
	err error
}

var _ extract.Extractor = (*Fake)(nil)

// New returns an empty Fake that reports misuse to t.
func New(t reporter) *Fake { return &Fake{t: t} }

// Repeat makes the last scripted reading answer every call after the script
// runs out.
func (f *Fake) Repeat() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.repeat = true
}

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

// Reset drops every scripted entry and Repeat, so a test can script new
// readings after sending an Input.
func (f *Fake) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script, f.next, f.repeat = nil, 0, false
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
	if f.next >= len(f.script) && !f.repeat {
		f.t.Helper()
		f.t.Errorf("extractfake: call %d but only %d readings are scripted; queue more or call Repeat()", f.next+1, len(f.script))
	}
	i := min(f.next, len(f.script)-1)
	f.next++
	s := f.script[i]
	return s.res, s.err
}
