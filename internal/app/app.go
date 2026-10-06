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

	core, err := bot.New(bot.Deps{
		Bale:     baleClient,
		Clock:    clk,
		Settings: store,
		Log:      log,
		OwnerID:  cfg.OwnerID,
		Version:  version.Version,
	})
	if err != nil {
		return err
	}

	// A queue of 1 keeps the poller from confirming (via the next offset) more
	// than one update the worker has not started yet.
	worker := updates.NewWorker(core.HandleUpdate, log, 1)

	offset := int64(0)
	if last, ok, err := store.LastUpdateID(ctx); err != nil {
		return err
	} else if ok {
		offset = last + 1
	}
	poller := &updates.Poller{
		Bale:   baleClient,
		Handle: worker.Enqueue,
		Offset: offset,
		Clock:  clk,
		Log:    log,
	}

	mux := http.NewServeMux()
	mux.Handle("GET /healthz", health.Handler(
		health.Check{Name: "db", Func: func() error {
			pctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			return store.Ping(pctx)
		}},
		health.Check{Name: "updates", Func: poller.Healthy},
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

	errc := make(chan error, 2)
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
	go worker.Run(ctx)
	go func() { errc <- poller.Run(ctx) }()
	go func() {
		log.Info("http listening", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("http server: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
	case runErr = <-errc:
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("stopped")
	if ctx.Err() != nil {
		return nil
	}
	return runErr
}
