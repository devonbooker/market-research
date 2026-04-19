package main

import "github.com/spf13/cobra"

func newDoctorCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{Use: "doctor", Short: "diagnose system health"}
}
