package main

import (
	"context"
	"log/slog"
	"os"
)

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	if err := NewApplication().Run(ctx, logger, os.Args[1:]); err != nil {
		logger.Error(err.Error())
		os.Exit(1)
	}
}
