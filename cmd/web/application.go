package main

import (
	"context"
	"flag"
	"log/slog"
	"time"

	"github.com/grodier/rss-go/internal/discovery"
	"github.com/grodier/rss-go/internal/inmem"
	"github.com/grodier/rss-go/internal/queue"
	"github.com/grodier/rss-go/internal/server"
)

type Application struct {
	config config
	logger *slog.Logger
}

func NewApplication(logger *slog.Logger) *Application {
	return &Application{
		config: defaultConfig(),
		logger: logger,
	}
}

type config struct {
	env    string
	server serverConfig
}

type serverConfig struct {
	port int
}

func defaultConfig() config {
	return config{
		env: "development",
		server: serverConfig{
			port: 4000,
		},
	}
}

func (app *Application) Run(ctx context.Context, args []string) error {
	app.config = app.ParseConfigs(args)

	srv := server.NewServer(app.logger)
	srv.Port = app.config.server.port
	srv.Env = app.config.env

	// Initialize services
	feedService := inmem.NewFeedService()
	userService := inmem.NewUserService()

	srv.FeedService = feedService
	srv.UserService = userService

	// Initialize discovery infrastructure (inject FeedService for persistence)
	discoveryStore := inmem.NewDiscoveryStore(feedService)
	jobQueue := queue.NewInMemQueue(3, app.logger) // 3 worker threads
	discoveryService := discovery.NewService(discoveryStore, feedService, jobQueue, app.logger)

	srv.DiscoveryStore = discoveryStore
	srv.DiscoveryService = discoveryService

	// Start job queue
	if err := jobQueue.Start(ctx); err != nil {
		return err
	}
	defer func() {
		app.logger.Info("shutting down job queue")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		jobQueue.Stop(shutdownCtx)
	}()

	if err := srv.Serve(); err != nil {
		return err
	}

	return nil
}

func (app *Application) ParseConfigs(args []string) config {
	config := defaultConfig()

	fs := flag.NewFlagSet("rss-go", flag.ContinueOnError)
	fs.StringVar(&config.env, "env", config.env, "Environment (development|production)")
	fs.IntVar(&config.server.port, "port", config.server.port, "Server port")

	fs.Parse(args)

	// Validate env value
	if config.env != "development" && config.env != "production" {
		app.logger.Warn("invalid environment value, falling back to default", "provided", config.env, "default", "development")
		config.env = "development"
	}

	return config
}
