package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/grodier/rss-go/internal/server"
)

type Application struct {
}

func NewApplication() *Application {
	return &Application{}
}

func (app *Application) Run(ctx context.Context, logger *slog.Logger, args []string) error {
	fmt.Println("Hello, World!")

	srv := server.NewServer()
	if err := srv.Serve(); err != nil {
		return err
	}

	return nil
}
