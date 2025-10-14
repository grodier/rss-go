package main

import (
	"context"
	"flag"
	"log/slog"

	"github.com/grodier/rss-go/internal/server"
)

type Application struct {
	config config
}

func NewApplication() *Application {
	return &Application{
		config: defaultConfig(),
	}
}

type config struct {
	server serverConfig
}

type serverConfig struct {
	port int
}

func defaultConfig() config {
	return config{
		server: serverConfig{
			port: 4000,
		},
	}
}

func (app *Application) Run(ctx context.Context, logger *slog.Logger, args []string) error {
	app.config = app.ParseConfigs(args)

	srv := server.NewServer(logger)
	srv.Port = app.config.server.port

	if err := srv.Serve(); err != nil {
		return err
	}

	return nil
}

func (app *Application) ParseConfigs(args []string) config {
	config := defaultConfig()

	fs := flag.NewFlagSet("rss-go", flag.ContinueOnError)
	fs.IntVar(&config.server.port, "port", config.server.port, "Server port")

	fs.Parse(args)

	return config
}
