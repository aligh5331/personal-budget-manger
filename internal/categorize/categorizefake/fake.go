// Package categorizefake is a scripted categorize.Categorizer for bot-core
// tests.
package categorizefake

import (
	"context"
	"fmt"
	"sync"

	"github.com/aligh5331/personal-budget-manger/internal/categorize"
)

// Fake answers from a script, one entry per call; once the script runs out it
// keeps using the last entry. With nothing scripted it picks the "none"
// option (Uncategorized) at confidence 1.
type Fake struct {
	mu     sync.Mutex
	script []step
	calls  []Call
}

type step struct {
	name       string
	confidence float64
	err        error
}

// Call is one recorded Categorize call.
type Call struct {
	Text    string
	Options []categorize.Option
}

var _ categorize.Categorizer = (*Fake)(nil)

// New returns an empty Fake.
func New() *Fake { return &Fake{} }

// Choose queues a pick of the offered option named name. If no such option
// was offered, Categorize returns an error.
func (f *Fake) Choose(name string, confidence float64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script = append(f.script, step{name: name, confidence: confidence})
}

// Fail queues a failure (both Jev attempts failed).
func (f *Fake) Fail(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.script = append(f.script, step{err: err})
}

// Calls returns every call so far.
func (f *Fake) Calls() []Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Call(nil), f.calls...)
}

// OptionNames returns the option names offered in call i.
func (c Call) OptionNames() []string {
	names := make([]string, len(c.Options))
	for i, o := range c.Options {
		names[i] = o.Name
	}
	return names
}

// Categorize implements categorize.Categorizer.
func (f *Fake) Categorize(_ context.Context, text string, options []categorize.Option) (categorize.Pick, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, Call{Text: text, Options: append([]categorize.Option(nil), options...)})
	var s step
	if len(f.script) > 0 {
		s = f.script[0]
		if len(f.script) > 1 {
			f.script = f.script[1:]
		}
	} else {
		s = step{confidence: 1}
	}
	if s.err != nil {
		return categorize.Pick{}, s.err
	}
	for _, o := range options {
		if (s.name == "" && o.None) || (s.name != "" && o.Name == s.name) {
			return categorize.Pick{ID: o.ID, Confidence: s.confidence}, nil
		}
	}
	return categorize.Pick{}, fmt.Errorf("categorizefake: %q was not offered", s.name)
}
