// Package app wires the production bot together: config -> storage -> Bale
// client -> bot core -> worker and update source -> HTTP server. It is the
// only place that knows concrete types; everything below talks interfaces.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	"github.com/aligh5331/personal-budget-manger/internal/backup"
	"github.com/aligh5331/personal-budget-manger/internal/bale"
	"github.com/aligh5331/personal-budget-manger/internal/bot"
	"github.com/aligh5331/personal-budget-manger/internal/clock"
	"github.com/aligh5331/personal-budget-manger/internal/config"
	"github.com/aligh5331/personal-budget-manger/internal/extract"
	"github.com/aligh5331/personal-budget-manger/internal/health"
	"github.com/aligh5331/personal-budget-manger/internal/storage/sqlite"
	"github.com/aligh5331/personal-budget-manger/internal/updates"
	"github.com/aligh5331/personal-budget-manger/internal/version"
)

// DBFile is the database file name inside DATA_DIR.
const DBFile = "bot.db"

// BackupsDir is the snapshot folder inside DATA_DIR.
const BackupsDir = "backups"

// Run starts the bot and blocks until ctx ends.
func Run(ctx context.Context, cfg config.Config, log *slog.Logger) error {
	log.Info("starting", "version", version.Version, "config", cfg)

	store, err := sqlite.Open(ctx, filepath.Join(cfg.DataDir, DBFile))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer func() { _ = store.Close() }()

	baleClient := bale.NewHTTPClient(bale.DefaultBaseURL, cfg.BotToken)
	clk := clock.System{}

	// The worker and the update source are built before the core, which
	// switches the source for /mode; the worker calls the core once built.
	var core *bot.Bot
	worker := updates.NewWorker(func(ctx context.Context, u bale.Update) error {
		return core.HandleUpdate(ctx, u)
	}, log)
	source := &updates.Manager{
		Bale:   baleClient,
		Store:  store,
		Worker: worker,
		Clock:  clk,
		Log:    log,
		Config: updates.ModeConfig{
			WebhookURL:  cfg.WebhookURL,
			SecretPath:  cfg.WebhookSecretPath,
			ModeDefault: cfg.ModeDefault,
		},
		Notify: func(ctx context.Context, text string) error {
			_, err := baleClient.SendMessage(ctx, bale.SendMessageParams{ChatID: cfg.OwnerID, Text: text})
			return err
		},
	}

	core, err = bot.New(bot.Deps{
		Bale:     baleClient,
		Clock:    clk,
		Settings: store,
		Log:      log,
		OwnerID:  cfg.OwnerID,
		Version:  version.Version,
		Updates:  source,

		Extractor:    extract.NewMetis(cfg.LLMAPIKey),
		Transactions: store,
	})
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	if webhook, err := updates.NewWebhookHandler(cfg.WebhookSecretPath, worker.Enqueue, log); err != nil {
		log.Info("webhook endpoint disabled", "reason", err)
	} else {
		mux.Handle(updates.WebhookPathPrefix, webhook)
	}
	mux.Handle("GET /healthz", health.Handler(
		health.Check{Name: "db", Func: func() error {
			pctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return store.Ping(pctx)
		}},
		health.Check{Name: "updates", Func: source.Healthy},
	))
	srv := &http.Server{Addr: cfg.ListenAddr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	backups := &backup.Job{
		DB:      store,
		Dir:     filepath.Join(cfg.DataDir, BackupsDir),
		Bale:    baleClient,
		OwnerID: cfg.OwnerID,
		Clock:   clk,
		Log:     log.With("component", "backup"),
	}

	runCtx, stopRun := context.WithCancel(ctx)
	defer stopRun()
	errc := make(chan error, 1)
	var runErr error
	// The backup job gets its own context so shutdown can stop it and wait,
	// and the database is never closed under a running VACUUM INTO.
	backupCtx, stopBackups := context.WithCancel(ctx)
	backupsDone := make(chan struct{})
	go func() {
		defer close(backupsDone)
		_ = backups.Run(backupCtx)
	}()
	defer func() {
		stopBackups()
		<-backupsDone
	}()
	go worker.Run(runCtx)
	go func() {
		log.Info("http listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("http server: %w", err)
		}
	}()
	// The webhook endpoint is being served before Bale is told about it.
	if err := source.Start(runCtx); err != nil {
		runErr = err
	} else {
		go source.RunChecks(runCtx)
		select {
		case <-ctx.Done():
		case runErr = <-errc:
		}
	}
	stopRun()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	source.Wait()
	log.Info("stopped")
	if ctx.Err() != nil {
		return nil
	}
	return runErr
}
