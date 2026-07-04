package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/Soltus/encv-go/internal/config"
	"github.com/Soltus/encv-go/internal/logger"
	"github.com/Soltus/encv-go/pkg/encv"
)

var Version = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	configPath, err := config.FindConfigPath("")
	if err != nil {
		slog.Warn("Config file not found, using defaults", "error", err)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	logLevel := logger.LevelInfo
	switch cfg.Log.Level {
	case "debug":
		logLevel = logger.LevelDebug
	case "info":
		logLevel = logger.LevelInfo
	case "warn":
		logLevel = logger.LevelWarn
	case "error":
		logLevel = logger.LevelError
	}

	var logFile string
	if cfg.Log.File != "" {
		logFile = cfg.Log.File
	}

	if err := logger.Init(logLevel, logFile); err != nil {
		slog.Warn("Failed to initialize structured logging, using console defaults", "error", err)
	}

	slog.Info("encv mobile started", "version", Version, "config_path", configPath)

	ctx := config.NewContext(context.Background(), cfg)

	if err := encv.Init(ctx); err != nil {
		slog.Error("Failed to initialize encv", "error", err)
		os.Exit(1)
	}

	s := encv.NewServer(ctx, configPath)
	addr, err := s.Start(Version)
	if err != nil {
		slog.Error("Failed to start server", "error", err)
		os.Exit(1)
	}

	slog.Info("Server started", "addr", addr)

	select {}
}
