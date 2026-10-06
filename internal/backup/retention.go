package backup

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/clock"
)

type snapshotFile struct {
	name string
	at   time.Time
}

// prune deletes snapshots outside the retention policy. Files in the folder
// that are not named like a snapshot are left alone.
func (j *Job) prune() error {
	entries, err := os.ReadDir(j.Dir)
	if err != nil {
		return err
	}
	loc := clock.Tehran()
	var files []snapshotFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, filePrefix) || !strings.HasSuffix(name, fileSuffix) {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(name, filePrefix), fileSuffix)
		at, err := time.ParseInLocation(fileTimeLayout, stamp, loc)
		if err != nil {
			continue
		}
		files = append(files, snapshotFile{name: name, at: at})
	}
	// Newest first, so the first file seen in a period is the one to keep.
	slices.SortFunc(files, func(a, b snapshotFile) int { return b.at.Compare(a.at) })

	keep := map[string]bool{}
	keepNewestPer(files, KeepDaily, dayOf, keep)
	keepNewestPer(files, KeepWeekly, weekOf, keep)

	var errs []error
	for _, f := range files {
		if !keep[f.name] {
			if err := os.Remove(filepath.Join(j.Dir, f.name)); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// keepNewestPer marks the newest file of each of the n newest periods.
// files must be sorted newest first.
func keepNewestPer(files []snapshotFile, n int, period func(time.Time) string, keep map[string]bool) {
	seen := map[string]bool{}
	for _, f := range files {
		if len(seen) == n {
			return
		}
		p := period(f.at)
		if seen[p] {
			continue
		}
		seen[p] = true
		keep[f.name] = true
	}
}

const dateKey = "2006-01-02"

// dayOf names t's calendar day (t is in Tehran time).
func dayOf(t time.Time) string { return t.Format(dateKey) }

// weekOf names t's week by the Saturday that starts it, the Iranian week.
func weekOf(t time.Time) string {
	back := (int(t.Weekday()) - int(time.Saturday) + 7) % 7
	return time.Date(t.Year(), t.Month(), t.Day()-back, 0, 0, 0, 0, t.Location()).Format(dateKey)
}
