package main

import (
	"os"

	"github.com/azu/dockerfile-pin/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
