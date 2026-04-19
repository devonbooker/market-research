package main

import (
	"context"
	"fmt"
	"time"

	"github.com/devonbooker/market-research/internal/sources"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/spf13/cobra"
)

func newRediscoverCmd(rt *runtime) *cobra.Command {
	var all bool
	var topic string
	var dryRun bool
	c := &cobra.Command{
		Use:   "rediscover",
		Short: "re-run source discovery for topics (weekly job)",
		RunE: func(cmd *cobra.Command, args []string) error {
			agent := &sources.Agent{Claude: sources.NewAnthropicClient(rt.cfg.AnthropicAPIKey)}
			if all || topic == "" {
				topics, err := rt.store.ListTopics(false)
				if err != nil {
					return err
				}
				for _, t := range topics {
					if err := rediscoverOne(cmd.Context(), cmd, rt, agent, t, dryRun); err != nil {
						fmt.Fprintf(cmd.OutOrStderr(), "rediscover %s: %v\n", t.Name, err)
					}
				}
				return nil
			}
			t, err := rt.store.GetTopicByName(topic)
			if err != nil {
				return err
			}
			return rediscoverOne(cmd.Context(), cmd, rt, agent, t, dryRun)
		},
	}
	c.Flags().BoolVar(&all, "all", false, "rediscover all active topics (default)")
	c.Flags().StringVar(&topic, "topic", "", "single topic by name")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "print proposed changes without writing")
	return c
}

func rediscoverOne(ctx context.Context, cmd *cobra.Command, rt *runtime, agent *sources.Agent, t *types.Topic, dryRun bool) error {
	stats, err := gatherStats(rt, t)
	if err != nil {
		return err
	}
	plan, err := agent.Rediscover(ctx, t.Name, t.Description, stats)
	if err != nil {
		return err
	}

	proposed := sources.PlanToSources(plan)
	if dryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "topic %s: proposed %d sources\n", t.Name, len(proposed))
		for _, s := range proposed {
			fmt.Fprintf(cmd.OutOrStdout(), "  %s/%s: %s\n", s.Platform, s.Kind, s.Value)
		}
		return nil
	}

	for _, s := range proposed {
		if _, _, err := rt.store.UpsertSource(t.ID, s.Platform, s.Kind, s.Value, types.AddedByAgent); err != nil {
			return err
		}
	}

	// Update signal scores for all (kept) agent-added sources based on stats.
	for _, s := range stats {
		srcs, _ := rt.store.ListSources(t.ID, s.Platform, true)
		for _, src := range srcs {
			if src.Kind == s.Kind && src.Value == s.Value && src.AddedBy == types.AddedByAgent {
				score := sources.Score(s.DocsLast7d, s.AvgScore)
				_ = rt.store.SetSourceSignalScore(src.ID, score)
			}
		}
	}
	return nil
}

func gatherStats(rt *runtime, t *types.Topic) ([]sources.SourceStat, error) {
	cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
	var out []sources.SourceStat
	for _, p := range []types.Platform{types.PlatformReddit, types.PlatformStackOverflow} {
		srcs, err := rt.store.ListSources(t.ID, p, true)
		if err != nil {
			return nil, err
		}
		for _, s := range srcs {
			n, avg, err := rt.store.SourceStatsSince(s.ID, cutoff)
			if err != nil {
				return nil, err
			}
			signal := 0.0
			if s.SignalScore != nil {
				signal = *s.SignalScore
			}
			out = append(out, sources.SourceStat{
				Platform: s.Platform, Kind: s.Kind, Value: s.Value,
				DocsLast7d: n, AvgScore: avg, SignalScore: signal,
			})
		}
	}
	return out, nil
}
