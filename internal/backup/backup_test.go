package backup_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/backup"
	"github.com/aligh5331/personal-budget-manger/internal/bale/balefake"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/storage/sqlite"
)

const ownerID int64 = 424242

// start is 14 Mehr 1405, 12:00 in Tehran (the bottest clock).
var start = time.Date(2026, 10, 6, 12, 0, 0, 0, clock.Tehran())

type fixture struct {
	job   *backup.Job
	store *sqlite.Store
	bale  *balefake.Fake
	clock *clock.Fake
	dir   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	tmp := t.TempDir()
	st, err := sqlite.Open(context.Background(), filepath.Join(tmp, "bot.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	f := &fixture{store: st, bale: balefake.New(), clock: clock.NewFake(start), dir: filepath.Join(tmp, "backups")}
	f.job = &backup.Job{DB: st, Dir: f.dir, Bale: f.bale, OwnerID: ownerID, Clock: f.clock}
	return f
}

func TestSnapshotOpensWithSameRows(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.store.SaveLastUpdateID(ctx, 1234); err != nil {
		t.Fatal(err)
	}

	path, err := f.job.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if filepath.Dir(path) != f.dir {
		t.Errorf("snapshot %s not in %s", path, f.dir)
	}

	snap, err := sqlite.Open(ctx, path)
	if err != nil {
		t.Fatalf("open snapshot: %v", err)
	}
	defer func() { _ = snap.Close() }()
	got, ok, err := snap.LastUpdateID(ctx)
	if err != nil || !ok || got != 1234 {
		t.Fatalf("snapshot LastUpdateID = %d, %v, %v; want 1234", got, ok, err)
	}
}

func TestSnapshotIsSentToOwnerAsDocument(t *testing.T) {
	f := newFixture(t)
	path, err := f.job.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	docs := f.bale.Documents()
	if len(docs) != 1 {
		t.Fatalf("sent %d documents, want 1", len(docs))
	}
	d := docs[0]
	if d.ChatID != ownerID {
		t.Errorf("sent to chat %d, want Owner %d", d.ChatID, ownerID)
	}
	if d.FileName != "bot-20261006-120000.db" {
		t.Errorf("file name %q", d.FileName)
	}
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(d.Content, onDisk) {
		t.Errorf("sent content differs from the snapshot on disk")
	}
}

func TestFailedSendIsLoggedAndKeepsTheSnapshot(t *testing.T) {
	f := newFixture(t)
	logs := &logRecorder{}
	f.job.Log = slog.New(logs)
	f.bale.Fail(balefake.MethodSendDocument, errors.New("bale down"))

	path, err := f.job.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce failed because of the send: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("snapshot missing after failed send: %v", err)
	}
	if logs.count(slog.LevelError) == 0 {
		t.Errorf("failed send was not logged as an error")
	}
}

// backupFiles lists snapshot file names in the backups folder.
func backupFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestRetentionKeeps14DailyAnd8Weekly(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	// 70 daily snapshots at 03:00, Wed 2026-10-07 through Tue 2026-12-15.
	f.clock.Set(time.Date(2026, 10, 7, 3, 0, 0, 0, clock.Tehran()))
	for range 70 {
		if _, err := f.job.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(24 * time.Hour)
	}

	// Daily: the last 14 days, 2 Dec to 15 Dec. Weekly (weeks run Saturday to
	// Friday): the newest copy of each of the last 8 weeks. The three newest
	// weeks are already covered by the daily copies (15, 11 and 4 Dec), which
	// adds the Fridays 27, 20, 13, 6 Nov and 30 Oct.
	want := []string{
		"bot-20261030-030000.db",
		"bot-20261106-030000.db",
		"bot-20261113-030000.db",
		"bot-20261120-030000.db",
		"bot-20261127-030000.db",
	}
	for d := 2; d <= 15; d++ {
		want = append(want, fmt.Sprintf("bot-202612%02d-030000.db", d))
	}
	if got := backupFiles(t, f.dir); !slices.Equal(got, want) {
		t.Errorf("kept\n%v\nwant\n%v", got, want)
	}
}

func TestRetentionKeepsNewestCopyOfADay(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	for range 2 {
		if _, err := f.job.RunOnce(ctx); err != nil {
			t.Fatal(err)
		}
		f.clock.Advance(time.Hour)
	}
	want := []string{"bot-20261006-130000.db"}
	if got := backupFiles(t, f.dir); !slices.Equal(got, want) {
		t.Errorf("kept %v, want %v", got, want)
	}
}

func TestRunBacksUpAtStartupThenDailyAt0300Tehran(t *testing.T) {
	f := newFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The fake sleep moves the clock instead of waiting, and stops the job
	// once three backups went out.
	f.job.Sleep = func(ctx context.Context, d time.Duration) error {
		if len(f.bale.Documents()) >= 3 {
			cancel()
			return ctx.Err()
		}
		f.clock.Advance(d)
		return nil
	}

	if err := f.job.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}

	var got []string
	for _, d := range f.bale.Documents() {
		got = append(got, d.FileName)
	}
	want := []string{
		"bot-20261006-120000.db", // startup
		"bot-20261007-030000.db",
		"bot-20261008-030000.db",
	}
	if !slices.Equal(got, want) {
		t.Errorf("backups sent %v, want %v", got, want)
	}
}

func TestRunKeepsGoingAfterAFailedSnapshot(t *testing.T) {
	f := newFixture(t)
	logs := &logRecorder{}
	f.job.Log = slog.New(logs)
	failing := &failOnce{Snapshotter: f.store}
	f.job.DB = failing
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f.job.Sleep = func(ctx context.Context, d time.Duration) error {
		if len(f.bale.Documents()) >= 1 {
			cancel()
			return ctx.Err()
		}
		f.clock.Advance(d)
		return nil
	}

	if err := f.job.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
	if logs.count(slog.LevelError) == 0 {
		t.Errorf("failed startup snapshot was not logged")
	}
	docs := f.bale.Documents()
	if len(docs) != 1 || docs[0].FileName != "bot-20261007-030000.db" {
		t.Errorf("after a failed startup snapshot, sent %v; want the 03:00 backup", docs)
	}
}

type failOnce struct {
	backup.Snapshotter
	failed bool
}

func (f *failOnce) SnapshotTo(ctx context.Context, path string) error {
	if !f.failed {
		f.failed = true
		return errors.New("disk full")
	}
	return f.Snapshotter.SnapshotTo(ctx, path)
}

type logRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (*logRecorder) Enabled(context.Context, slog.Level) bool { return true }

func (r *logRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec.Clone())
	return nil
}

func (r *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }

func (r *logRecorder) WithGroup(string) slog.Handler { return r }

func (r *logRecorder) count(level slog.Level) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, rec := range r.records {
		if rec.Level == level {
			n++
		}
	}
	return n
}
