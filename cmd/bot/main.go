// Command bot runs the personal budget Bale bot. All configuration comes from
// environment variables (see .env.example). `bot -version` prints the version
// baked in at build time and exits.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aligh5331/personal-budget-manger/internal/app"
	"github.com/aligh5331/personal-budget-manger/internal/config"
	"github.com/aligh5331/personal-budget-manger/internal/version"
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version.Version)
		return
	}

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Error("bad configuration", "err", err)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := app.Run(ctx, cfg, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}
