package main

import "github.com/spf13/cobra"

func newFetchCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{Use: "fetch", Short: "fetch new content for topics"}
}
