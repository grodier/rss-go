package main

import (
	"context"
	"fmt"
	"log/slog"
)

type Application struct {
}

func NewApplication() *Application {
	return &Application{}
}

func (app *Application) Run(ctx context.Context, logger *slog.Logger, args []string) error {
	fmt.Println("Hello, World!")
	return nil
}
