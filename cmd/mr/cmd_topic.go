package main

import (
	"fmt"

	"github.com/devonbooker/market-research/internal/sources"
	"github.com/devonbooker/market-research/internal/store"
	"github.com/devonbooker/market-research/internal/types"
	"github.com/spf13/cobra"
)

func newTopicCmd(rt *runtime) *cobra.Command {
	c := &cobra.Command{Use: "topic", Short: "manage topics"}
	c.AddCommand(newTopicAddCmd(rt))
	c.AddCommand(newTopicListCmd(rt))
	c.AddCommand(newTopicRemoveCmd(rt))
	return c
}

func newTopicAddCmd(rt *runtime) *cobra.Command {
	var description string
	c := &cobra.Command{
		Use:   "add <name>",
		Short: "add a topic and run initial source discovery",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			if _, err := rt.store.GetTopicByName(name); err == nil {
				return fmt.Errorf("topic %q already exists", name)
			}

			// Start inactive; activate after discovery succeeds.
			id, err := rt.store.CreateTopic(name, description, false)
			if err != nil {
				return err
			}

			agent := &sources.Agent{Claude: sources.NewAnthropicClient(rt.cfg.AnthropicAPIKey)}
			plan, err := agent.Discover(cmd.Context(), name, description)
			if err != nil {
				fmt.Fprintf(cmd.OutOrStderr(), "warning: discovery failed, topic kept inactive: %v\n", err)
				return nil
			}

			for _, s := range sources.PlanToSources(plan) {
				if _, _, err := rt.store.UpsertSource(id, s.Platform, s.Kind, s.Value, types.AddedByAgent); err != nil {
					return err
				}
			}
			if err := rt.store.SetTopicActive(id, true); err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "added topic %q with %d sources\n", name, len(sources.PlanToSources(plan)))
			return nil
		},
	}
	c.Flags().StringVar(&description, "description", "", "optional description passed to the agent")
	return c
}

func newTopicListCmd(rt *runtime) *cobra.Command {
	var showIssues bool
	c := &cobra.Command{
		Use:   "list",
		Short: "list topics and their sources",
		RunE: func(cmd *cobra.Command, args []string) error {
			topics, err := rt.store.ListTopics(true)
			if err != nil {
				return err
			}
			for _, t := range topics {
				fmt.Fprintf(cmd.OutOrStdout(), "topic: %s (active=%t)\n", t.Name, t.Active)
				for _, p := range []types.Platform{types.PlatformReddit, types.PlatformStackOverflow} {
					srcs, err := rt.store.ListSources(t.ID, p, showIssues)
					if err != nil {
						return err
					}
					for _, s := range srcs {
						marker := ""
						if !s.Active {
							marker = " [INACTIVE]"
						}
						fmt.Fprintf(cmd.OutOrStdout(), "  %s/%s: %s%s\n", s.Platform, s.Kind, s.Value, marker)
					}
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&showIssues, "issues", false, "include inactive (errored) sources")
	return c
}

func newTopicRemoveCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "remove a topic and all its data",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			t, err := rt.store.GetTopicByName(args[0])
			if err != nil {
				return err
			}
			return rt.store.DeleteTopic(t.ID)
		},
	}
}

var _ = store.ErrNotFound
