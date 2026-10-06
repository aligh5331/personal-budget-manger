// Package bottest is the bot-core test seam. A Harness wires a real bot.Bot to
// a recording fake Bale client, a fixed Asia/Tehran clock and a real SQLite
// database (migrations applied) in a temp directory.
//
// Tests feed Bale updates in and assert on what the Owner would see: messages
// sent and edited, callback answers, and stored rows read through the storage
// interfaces.
package bottest

import (
	"context"
	"log/slog"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bale/balefake"
	"github.com/aligh5331/personal-budget-manger/internal/bot"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/storage/sqlite"
)

// OwnerID is the Owner's Bale user id (and private chat id) in tests.
const OwnerID int64 = 424242

// StrangerID is some other Bale account.
const StrangerID int64 = 777

// Version is the version string the harness bot reports.
const Version = "v0.0.0-test"

// Start is the harness clock's initial time: 14 Mehr 1405, 12:00 in Tehran.
var Start = time.Date(2026, 10, 6, 12, 0, 0, 0, clock.Tehran())

// Harness is one bot under test.
type Harness struct {
	t     testing.TB
	Bot   *bot.Bot
	Bale  *balefake.Fake
	Clock *clock.Fake
	Store *sqlite.Store
	Logs  *LogRecorder

	dbPath        string
	nextUpdateID  int64
	nextMessageID int64
}

// New returns a Harness with a fresh database.
func New(t testing.TB) *Harness {
	t.Helper()
	h := &Harness{
		t:             t,
		Bale:          balefake.New(),
		Clock:         clock.NewFake(Start),
		Logs:          &LogRecorder{},
		dbPath:        filepath.Join(t.TempDir(), "bot.db"),
		nextUpdateID:  1,
		nextMessageID: 1,
	}
	h.Store = openStore(t, h.dbPath)
	h.Bot = h.newBot()
	return h
}

func openStore(t testing.TB, path string) *sqlite.Store {
	t.Helper()
	st, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

// Deps returns the dependencies the harness wires into the bot. Tickets that
// add dependencies (Extractor, Categorizer, ...) add their fakes here.
func (h *Harness) Deps() bot.Deps {
	return bot.Deps{
		Bale:     h.Bale,
		Clock:    h.Clock,
		Settings: h.Store,
		Log:      slog.New(h.Logs),
		OwnerID:  OwnerID,
		Version:  Version,
	}
}

func (h *Harness) newBot() *bot.Bot {
	b, err := bot.New(h.Deps())
	if err != nil {
		h.t.Fatalf("bot.New: %v", err)
	}
	return b
}

// Restart simulates a process restart: the database is reopened from disk and
// a new Bot is built. The fake Bale client and clock are kept.
func (h *Harness) Restart() {
	h.t.Helper()
	_ = h.Store.Close()
	h.Store = openStore(h.t, h.dbPath)
	h.Bot = h.newBot()
}

// Feed hands one update to the bot core and fails the test on error.
func (h *Harness) Feed(u bale.Update) {
	h.t.Helper()
	if err := h.Bot.HandleUpdate(context.Background(), u); err != nil {
		h.t.Fatalf("HandleUpdate(%d): %v", u.UpdateID, err)
	}
}

// NextUpdateID returns a fresh, increasing update_id.
func (h *Harness) NextUpdateID() int64 {
	id := h.nextUpdateID
	h.nextUpdateID++
	return id
}

// OwnerMessage builds a private message from the Owner with a fresh message id.
func (h *Harness) OwnerMessage(text string) *bale.Message {
	return h.MessageFrom(OwnerID, text)
}

// MessageFrom builds a private message from userID with a fresh message id.
func (h *Harness) MessageFrom(userID int64, text string) *bale.Message {
	id := h.nextMessageID
	h.nextMessageID++
	return &bale.Message{
		MessageID: id,
		From:      &bale.User{ID: userID, FirstName: "user"},
		Date:      h.Clock.Now().Unix(),
		Chat:      bale.Chat{ID: userID, Type: bale.ChatPrivate},
		Text:      text,
	}
}

// SendText feeds a Text note or command from the Owner and returns the update.
func (h *Harness) SendText(text string) bale.Update {
	h.t.Helper()
	u := bale.Update{UpdateID: h.NextUpdateID(), Message: h.OwnerMessage(text)}
	h.Feed(u)
	return u
}

// SendMessage feeds an arbitrary message (fresh update_id) and returns the update.
func (h *Harness) SendMessage(m *bale.Message) bale.Update {
	h.t.Helper()
	u := bale.Update{UpdateID: h.NextUpdateID(), Message: m}
	h.Feed(u)
	return u
}

// Tap feeds a button tap by the Owner on message msg with callback data.
func (h *Harness) Tap(msg bale.Message, data string) bale.Update {
	h.t.Helper()
	return h.TapAs(OwnerID, msg, data)
}

// TapAs feeds a button tap by userID.
func (h *Harness) TapAs(userID int64, msg bale.Message, data string) bale.Update {
	h.t.Helper()
	id := h.NextUpdateID()
	u := bale.Update{
		UpdateID: id,
		CallbackQuery: &bale.CallbackQuery{
			ID:      "cb" + strconv.FormatInt(id, 10),
			From:    bale.User{ID: userID},
			Message: &msg,
			Data:    data,
		},
	}
	h.Feed(u)
	return u
}

// Sent returns every message the bot sent so far.
func (h *Harness) Sent() []bale.SendMessageParams { return h.Bale.Sent() }

// LastSent returns the last message the bot sent, failing if there is none.
func (h *Harness) LastSent() bale.SendMessageParams {
	h.t.Helper()
	s := h.Bale.Sent()
	if len(s) == 0 {
		h.t.Fatal("bot sent nothing")
	}
	return s[len(s)-1]
}

// LogRecorder is a slog.Handler that keeps records for assertions.
type LogRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

// Enabled implements slog.Handler.
func (*LogRecorder) Enabled(context.Context, slog.Level) bool { return true }

// Handle implements slog.Handler.
func (r *LogRecorder) Handle(_ context.Context, rec slog.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, rec.Clone())
	return nil
}

// WithAttrs implements slog.Handler (attributes are dropped).
func (r *LogRecorder) WithAttrs([]slog.Attr) slog.Handler { return r }

// WithGroup implements slog.Handler (groups are dropped).
func (r *LogRecorder) WithGroup(string) slog.Handler { return r }

// Count returns how many records have the given message.
func (r *LogRecorder) Count(msg string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, rec := range r.records {
		if rec.Message == msg {
			n++
		}
	}
	return n
}
