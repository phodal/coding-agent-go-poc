package main

import (
	"context"
	"os"

	"agent-platform/cmd/root"
)

func main() {
	ctx := context.Background()
	cmd := root.New(ctx)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
