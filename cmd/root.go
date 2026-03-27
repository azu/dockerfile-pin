package cmd

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "dockerfile-pin",
	Short: "Pin Dockerfile images to digests",
	Long:  "A CLI tool that adds @sha256:<digest> to FROM lines in Dockerfiles to prevent supply chain attacks.",
}

func Execute() error {
	return rootCmd.Execute()
}
