package main

import (
	"fmt"
	"os"
	"time"

	"github.com/devonbooker/market-research/internal/types"
	"github.com/spf13/cobra"
)

func newDoctorCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "report system health (last runs, stale sources, db stats)",
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()

			// DB file stats
			if fi, err := os.Stat(rt.cfg.DBPath); err == nil {
				fmt.Fprintf(out, "db: %s (%.2f MB)\n", rt.cfg.DBPath, float64(fi.Size())/1024/1024)
			} else {
				fmt.Fprintf(out, "db: %s (stat error: %v)\n", rt.cfg.DBPath, err)
			}

			topics, err := rt.store.ListTopics(true)
			if err != nil {
				return err
			}

			fmt.Fprintf(out, "\ntopics (%d):\n", len(topics))
			for _, t := range topics {
				fmt.Fprintf(out, "  %s (active=%t)\n", t.Name, t.Active)
				for _, p := range []types.Platform{types.PlatformReddit, types.PlatformStackOverflow} {
					var startedAt *time.Time
					var status string
					row := rt.store.DB().QueryRow(
						`SELECT started_at, status FROM fetch_runs
						 WHERE topic_id = ? AND platform = ? AND status = 'success'
						 ORDER BY started_at DESC LIMIT 1`, t.ID, p)
					var ts time.Time
					if err := row.Scan(&ts, &status); err == nil {
						startedAt = &ts
					}
					if startedAt == nil {
						fmt.Fprintf(out, "    %s: no successful runs\n", p)
					} else {
						age := time.Since(*startedAt).Round(time.Minute)
						fmt.Fprintf(out, "    %s: last success %s ago\n", p, age)
					}
				}
			}

			// Stale sources (no fetch in >7 days)
			rows, err := rt.store.DB().Query(
				`SELECT topic_id, platform, kind, value, last_fetched
				 FROM sources WHERE active = 1 AND (last_fetched IS NULL OR last_fetched < ?)`,
				time.Now().UTC().Add(-7*24*time.Hour))
			if err != nil {
				return err
			}
			defer rows.Close()

			fmt.Fprintln(out, "\nstale sources (>7d since last fetch):")
			any := false
			for rows.Next() {
				var tID int64
				var plat, kind, val string
				var last *time.Time
				if err := rows.Scan(&tID, &plat, &kind, &val, &last); err != nil {
					return err
				}
				any = true
				fmt.Fprintf(out, "  topic=%d %s/%s: %s\n", tID, plat, kind, val)
			}
			if !any {
				fmt.Fprintln(out, "  (none)")
			}
			return nil
		},
	}
}
