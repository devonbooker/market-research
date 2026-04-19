package main

import "github.com/spf13/cobra"

func newRediscoverCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{Use: "rediscover", Short: "re-run source discovery"}
}
