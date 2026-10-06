// Package backup snapshots the database with VACUUM INTO, keeps a rolling
// set of copies under the data directory's backups folder and sends each new
// snapshot to the Owner's Bale chat as a document.
package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
)

// Snapshotter writes a consistent copy of the database to a new file.
type Snapshotter interface {
	SnapshotTo(ctx context.Context, path string) error
}

// DocumentSender is the part of bale.Client the job needs.
type DocumentSender interface {
	SendDocument(ctx context.Context, p bale.SendDocumentParams) (bale.Message, error)
}

// Job takes, prunes and sends backups.
type Job struct {
	DB      Snapshotter
	Dir     string // where snapshots live, normally DATA_DIR/backups
	Bale    DocumentSender
	OwnerID int64
	Clock   clock.Clock
	Log     *slog.Logger
	// Sleep waits d or until ctx ends. Nil means a real timer.
	Sleep func(ctx context.Context, d time.Duration) error
}

// DailyAt is the hour of the scheduled backup, Tehran time.
const DailyAt = 3

// maxWait caps one sleep so a wall-clock jump (suspend, NTP step) delays a
// backup by at most this much.
const maxWait = time.Hour

// Run takes a backup now, then every day at 03:00 Tehran time, until ctx
// ends. A failed backup is logged and the schedule goes on. It returns ctx's
// error.
func (j *Job) Run(ctx context.Context) error {
	for {
		if _, err := j.RunOnce(ctx); err != nil {
			j.log().Error("backup failed", "err", err)
		}
		next := NextRun(j.Clock.Now())
		for {
			wait := next.Sub(j.Clock.Now())
			if wait <= 0 {
				break
			}
			if err := j.sleep(ctx, min(wait, maxWait)); err != nil {
				return err
			}
		}
	}
}

// NextRun returns the first 03:00 Tehran time strictly after now.
func NextRun(now time.Time) time.Time {
	t := now.In(clock.Tehran())
	next := time.Date(t.Year(), t.Month(), t.Day(), DailyAt, 0, 0, 0, t.Location())
	if !next.After(t) {
		next = time.Date(t.Year(), t.Month(), t.Day()+1, DailyAt, 0, 0, 0, t.Location())
	}
	return next
}

func (j *Job) sleep(ctx context.Context, d time.Duration) error {
	if j.Sleep != nil {
		return j.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Snapshot files are named bot-YYYYMMDD-HHMMSS.db in Tehran time.
const (
	filePrefix     = "bot-"
	fileSuffix     = ".db"
	fileTimeLayout = "20060102-150405"
)

// Retention: the newest copy of each of the last KeepDaily days and of each
// of the last KeepWeekly weeks (Saturday to Friday, Tehran time) that have a
// copy. A copy kept by both rules counts toward both.
const (
	KeepDaily  = 14
	KeepWeekly = 8
)

// maxDocumentBytes is Bale's upload limit for bots (50 MB).
const maxDocumentBytes = 50 << 20

// RunOnce takes one snapshot and returns its path.
func (j *Job) RunOnce(ctx context.Context) (string, error) {
	if err := os.MkdirAll(j.Dir, 0o750); err != nil {
		return "", fmt.Errorf("create backups dir: %w", err)
	}
	now := j.Clock.Now().In(clock.Tehran())
	path := filepath.Join(j.Dir, filePrefix+now.Format(fileTimeLayout)+fileSuffix)
	// Write under a temporary name so a crash never leaves a half-written
	// file that looks like a snapshot.
	tmp := path + ".tmp"
	_ = os.Remove(tmp)
	if err := j.DB.SnapshotTo(ctx, tmp); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", fmt.Errorf("finish snapshot: %w", err)
	}
	if err := j.prune(); err != nil {
		j.log().Error("pruning old backups failed", "err", err)
	}
	if err := j.send(ctx, path); err != nil {
		j.log().Error("backup not sent to Owner", "file", filepath.Base(path), "err", err)
	}
	return path, nil
}

// send uploads the snapshot to the Owner's chat.
func (j *Job) send(ctx context.Context, path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(content) > maxDocumentBytes {
		return fmt.Errorf("snapshot is %d bytes, over Bale's %d byte limit", len(content), maxDocumentBytes)
	}
	name := filepath.Base(path)
	_, err = j.Bale.SendDocument(ctx, bale.SendDocumentParams{
		ChatID:   j.OwnerID,
		FileName: name,
		Content:  content,
		Caption:  "Database backup " + name,
	})
	return err
}

func (j *Job) log() *slog.Logger {
	if j.Log == nil {
		return slog.Default()
	}
	return j.Log
}
