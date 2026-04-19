package main

import "github.com/spf13/cobra"

func newTopicCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{Use: "topic", Short: "manage topics"}
}
