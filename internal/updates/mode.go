package updates

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
)

// Update modes.
const (
	ModePolling = "polling"
	ModeWebhook = "webhook"
)

// CheckInterval is how often CheckWebhook runs in webhook mode. A
// getWebhookInfo last_error younger than this counts as "recent".
const CheckInterval = 10 * time.Minute

// ModeConfig is the webhook part of the configuration.
type ModeConfig struct {
	WebhookURL  string // public base URL, no trailing slash
	SecretPath  string // WEBHOOK_SECRET_PATH; never logged
	ModeDefault string // used until the Owner picks a mode with /mode
}

// SourceClient is the part of bale.Client the Manager needs.
type SourceClient interface {
	PollClient
	SetWebhook(ctx context.Context, url string) error
	GetWebhookInfo(ctx context.Context) (bale.WebhookInfo, error)
}

// ModeStore is the persisted state the Manager reads and writes.
type ModeStore interface {
	LastUpdateID(ctx context.Context) (id int64, ok bool, err error)
	UpdateMode(ctx context.Context) (mode string, ok bool, err error)
	SaveUpdateMode(ctx context.Context, mode string) error
}

// Manager owns the update source: it runs the poller or keeps the webhook
// registered, switches between them for /mode, falls back to polling when
// the webhook breaks, and reports the active source's health.
//
// Every update from either source goes to Worker. A switch may run inside
// the worker (the /mode tap is itself an update): stopping the poller never
// waits for the worker, because Worker.Enqueue never blocks.
type Manager struct {
	Bale   SourceClient
	Store  ModeStore
	Worker *Worker
	Clock  clock.Clock
	Log    *slog.Logger
	Config ModeConfig
	// Notify messages the Owner (boot fallback and failover).
	Notify func(ctx context.Context, text string) error

	mu         sync.Mutex
	ctx        context.Context // from Start; pollers run under it
	mode       string
	note       string
	webhookErr error // result of the last webhook check
	poller     *Poller
	pollStop   chan struct{}
	pollDone   chan error
	pollExited chan struct{}
}

// Start boots the update source: the mode comes from the Store, else
// Config.ModeDefault, else polling, and is asserted against Bale. If webhook
// mode cannot be set up, the bot polls for this run without changing the
// saved mode and tells the Owner. Start returns an error only when the Store
// cannot be read. The poller stops when ctx ends.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Log == nil {
		m.Log = slog.Default()
	}
	m.ctx = ctx

	want, ok, err := m.Store.UpdateMode(ctx)
	if err != nil {
		return fmt.Errorf("read update mode: %w", err)
	}
	if !ok || (want != ModePolling && want != ModeWebhook) {
		want = m.Config.ModeDefault
	}
	if want != ModeWebhook {
		if err := m.Bale.DeleteWebhook(ctx); err != nil {
			// The poller deletes it itself if getUpdates is refused.
			m.Log.Warn("deleteWebhook at startup failed", "err", m.redact(err))
		}
		if err := m.startPolling(); err != nil {
			return err
		}
		m.mode = ModePolling
		m.Log.Info("update mode", "mode", m.mode)
		return nil
	}

	if err := m.enterWebhook(ctx); err != nil {
		err = m.redact(err)
		m.Log.Error("webhook mode failed at startup; polling for this run", "err", err)
		if derr := m.Bale.DeleteWebhook(ctx); derr != nil {
			m.Log.Warn("deleteWebhook after failed webhook setup", "err", m.redact(derr))
		}
		if perr := m.startPolling(); perr != nil {
			return perr
		}
		m.mode = ModePolling
		m.note = "webhook failed at startup: " + err.Error()
		m.notify(ctx, "Webhook mode failed at startup: "+err.Error()+
			"\nThe bot is polling for this run. The saved mode is still webhook; fix the setup and restart, or use /mode.")
		return nil
	}
	m.mode = ModeWebhook
	m.Log.Info("update mode", "mode", m.mode)
	return nil
}

// RunChecks runs CheckWebhook every CheckInterval until ctx ends.
func (m *Manager) RunChecks(ctx context.Context) {
	t := time.NewTicker(CheckInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.CheckWebhook(ctx)
		}
	}
}

// CheckWebhook asks Bale for the webhook status when in webhook mode and
// records the result for Healthy. A wrong URL or a recent last_error fails
// over to polling for this run (the saved mode is kept) and tells the Owner;
// after that the bot is polling, so the Owner is told once. A failed
// getWebhookInfo call only marks the webhook unhealthy.
func (m *Manager) CheckWebhook(ctx context.Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.mode != ModeWebhook {
		return
	}
	info, err := m.Bale.GetWebhookInfo(ctx)
	if err != nil {
		m.webhookErr = fmt.Errorf("getWebhookInfo: %w", m.redact(err))
		m.Log.Warn("webhook check failed", "err", m.webhookErr)
		return
	}
	problem := m.webhookProblem(info, true)
	m.webhookErr = problem
	if problem == nil {
		return
	}
	m.Log.Error("webhook broken; polling for this run", "err", problem)
	if err := m.Bale.DeleteWebhook(ctx); err != nil {
		m.Log.Warn("deleteWebhook during failover failed", "err", m.redact(err))
	}
	if err := m.startPolling(); err != nil {
		m.Log.Error("failover to polling failed", "err", err)
		return
	}
	m.mode = ModePolling
	m.note = "webhook failed its check: " + problem.Error()
	m.notify(ctx, "The webhook is broken: "+problem.Error()+
		"\nSwitched to polling for this run. Use /mode to go back to webhook once it is fixed.")
}

// Mode returns the active mode and, after a fallback, why it differs from
// the saved choice.
func (m *Manager) Mode() (mode, note string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.mode, m.note
}

// SwitchMode switches to mode and saves it. Going to polling deletes the
// webhook and starts polling. Going to webhook checks the config, stops
// polling after its in-flight call and commits its offset, sets the webhook
// and verifies it with getWebhookInfo. Any failure rolls back to the previous
// mode and returns the error (with the secret redacted).
func (m *Manager) SwitchMode(ctx context.Context, mode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx == nil {
		return errors.New("the update source is not running")
	}
	if mode == m.mode {
		return m.saveMode(ctx, mode)
	}
	var err error
	switch mode {
	case ModeWebhook:
		err = m.toWebhook(ctx)
	case ModePolling:
		err = m.toPolling(ctx)
	default:
		return fmt.Errorf("unknown mode %q", mode)
	}
	if err != nil {
		err = m.redact(err)
		m.Log.Warn("mode switch failed", "to", mode, "err", err)
		return err
	}
	m.mode, m.note = mode, ""
	m.Log.Info("update mode switched", "mode", mode)
	return nil
}

func (m *Manager) saveMode(ctx context.Context, mode string) error {
	if err := m.Store.SaveUpdateMode(ctx, mode); err != nil {
		return fmt.Errorf("save mode: %w", err)
	}
	if mode == m.mode {
		m.note = ""
	}
	return nil
}

func (m *Manager) toWebhook(ctx context.Context) error {
	if err := m.checkConfig(); err != nil {
		return err
	}
	if err := m.stopPolling(); err != nil {
		return errors.Join(err, m.rollbackToPolling(ctx, false))
	}
	if err := m.enterWebhook(ctx); err != nil {
		return errors.Join(err, m.rollbackToPolling(ctx, true))
	}
	if err := m.saveMode(ctx, ModeWebhook); err != nil {
		return errors.Join(err, m.rollbackToPolling(ctx, true))
	}
	return nil
}

func (m *Manager) toPolling(ctx context.Context) error {
	if err := m.Bale.DeleteWebhook(ctx); err != nil {
		return fmt.Errorf("deleteWebhook: %w", err)
	}
	if err := m.startPolling(); err != nil {
		return errors.Join(err, m.rollbackToWebhook(ctx))
	}
	if err := m.saveMode(ctx, ModePolling); err != nil {
		perr := m.stopPolling()
		return errors.Join(err, perr, m.rollbackToWebhook(ctx))
	}
	m.webhookErr = nil
	return nil
}

func (m *Manager) rollbackToPolling(ctx context.Context, deleteWebhook bool) error {
	var errs []error
	if deleteWebhook {
		if err := m.Bale.DeleteWebhook(ctx); err != nil {
			errs = append(errs, fmt.Errorf("deleteWebhook: %w", err))
		}
	}
	if err := m.startPolling(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("rollback to polling: %w", errors.Join(errs...))
	}
	return nil
}

func (m *Manager) rollbackToWebhook(ctx context.Context) error {
	if err := m.enterWebhook(ctx); err != nil {
		return fmt.Errorf("rollback to webhook: %w", err)
	}
	return nil
}

// webhookURL is where Bale posts updates. It contains the secret.
func (m *Manager) webhookURL() string {
	return m.Config.WebhookURL + WebhookPathPrefix + m.Config.SecretPath
}

func (m *Manager) checkConfig() error {
	if m.Config.WebhookURL == "" {
		return errors.New("WEBHOOK_URL is not set")
	}
	return CheckSecret(m.Config.SecretPath)
}

// enterWebhook registers the webhook and verifies it.
func (m *Manager) enterWebhook(ctx context.Context) error {
	if err := m.checkConfig(); err != nil {
		return err
	}
	if err := m.Bale.SetWebhook(ctx, m.webhookURL()); err != nil {
		return fmt.Errorf("setWebhook: %w", err)
	}
	info, err := m.Bale.GetWebhookInfo(ctx)
	if err != nil {
		return fmt.Errorf("verify webhook: getWebhookInfo: %w", err)
	}
	if err := m.webhookProblem(info, false); err != nil {
		return fmt.Errorf("verify webhook: %w", err)
	}
	m.webhookErr = nil
	return nil
}

// webhookProblem says what is wrong with info, or nil. withLastError also
// treats a delivery error within CheckInterval as broken.
func (m *Manager) webhookProblem(info bale.WebhookInfo, withLastError bool) error {
	if info.URL != m.webhookURL() {
		if info.URL == "" {
			return errors.New("no webhook is set at Bale")
		}
		return errors.New("the webhook URL at Bale is a different one")
	}
	if withLastError && info.LastErrorDate > 0 {
		at := time.Unix(info.LastErrorDate, 0)
		if m.Clock.Now().Sub(at) < CheckInterval {
			msg := info.LastErrorMessage
			if msg == "" {
				msg = "unknown error"
			}
			return fmt.Errorf("delivery error reported by Bale %s ago: %s", m.Clock.Now().Sub(at).Round(time.Second), m.redactString(msg))
		}
	}
	return nil
}

// startPolling starts a poller from the highest processed update_id.
func (m *Manager) startPolling() error {
	offset := int64(0)
	if last, ok, err := m.Store.LastUpdateID(m.ctx); err != nil {
		return fmt.Errorf("start polling: %w", err)
	} else if ok {
		offset = last + 1
	}
	p := &Poller{
		Bale:   m.Bale,
		Handle: m.Worker.Enqueue,
		Wait:   m.Worker.WaitIdle,
		Offset: offset,
		Clock:  m.Clock,
		Log:    m.Log,
	}
	stop, done, exited := make(chan struct{}), make(chan error, 1), make(chan struct{})
	go func() {
		defer close(exited)
		done <- p.RunUntil(m.ctx, stop)
	}()
	m.poller, m.pollStop, m.pollDone, m.pollExited = p, stop, done, exited
	return nil
}

// stopPolling stops the poller after its in-flight call and commits its
// offset. Updates it fetched are already with the worker.
func (m *Manager) stopPolling() error {
	if m.poller == nil {
		return nil
	}
	close(m.pollStop)
	err := <-m.pollDone
	m.poller, m.pollStop, m.pollDone = nil, nil, nil
	return err
}

// Wait blocks until the poller (if any) has exited after Start's ctx ended.
func (m *Manager) Wait() {
	m.mu.Lock()
	exited := m.pollExited
	m.mu.Unlock()
	if exited != nil {
		<-exited
	}
}

// Healthy reports the active update source's health: for polling, a recent
// successful poll; for webhook, the last getWebhookInfo check.
func (m *Manager) Healthy() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.mode == ModeWebhook && m.webhookErr != nil:
		return fmt.Errorf("webhook: %w", m.webhookErr)
	case m.mode == ModeWebhook:
		return nil
	case m.poller != nil:
		if err := m.poller.Healthy(); err != nil {
			return fmt.Errorf("polling: %w", err)
		}
		return nil
	default:
		return errors.New("update source not started")
	}
}

func (m *Manager) notify(ctx context.Context, text string) {
	if m.Notify == nil {
		return
	}
	if err := m.Notify(ctx, text); err != nil {
		m.Log.Warn("could not message the Owner", "err", m.redact(err))
	}
}

// redact keeps the webhook secret out of errors shown or logged. (A secret
// too short for webhook mode is never sent anywhere, so it can't leak.)
func (m *Manager) redact(err error) error {
	if err == nil {
		return nil
	}
	if s := m.redactString(err.Error()); s != err.Error() {
		return errors.New(s)
	}
	return err
}

func (m *Manager) redactString(s string) string {
	if CheckSecret(m.Config.SecretPath) != nil {
		return s
	}
	return strings.ReplaceAll(s, m.Config.SecretPath, "<secret>")
}
