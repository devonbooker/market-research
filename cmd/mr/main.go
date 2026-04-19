package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/devonbooker/market-research/internal/config"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/spf13/cobra"
)

type runtime struct {
	cfg   *config.Config
	store *store.Store
}

func newRootCmd() (*cobra.Command, *runtime) {
	rt := &runtime{}
	root := &cobra.Command{
		Use:   "mr",
		Short: "market-research data collection CLI",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			rt.cfg = cfg
			s, err := store.Open(cfg.DBPath)
			if err != nil {
				return fmt.Errorf("open store: %w", err)
			}
			rt.store = s
			if n, err := s.MarkOrphanRunsErrored("process restarted"); err == nil && n > 0 {
				slog.Warn("recovered orphan runs", "count", n)
			}
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if rt.store != nil {
				_ = rt.store.Close()
			}
		},
	}
	root.AddCommand(newTopicCmd(rt))
	root.AddCommand(newFetchCmd(rt))
	root.AddCommand(newRediscoverCmd(rt))
	root.AddCommand(newDoctorCmd(rt))
	return root, rt
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))

	defer func() {
		if r := recover(); r != nil {
			slog.Error("fatal panic", "panic", r)
			os.Exit(2)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	root, _ := newRootCmd()
	root.SetContext(ctx)
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
