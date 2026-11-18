package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Arseniy22877/game/internal/httpapi"
)

type config struct {
	adminToken   string
	port         string
	dbPath       string
	scenariosDir string
	webDir       string
}

func loadConfig() (config, error) {
	cfg := config{
		adminToken:   os.Getenv("ADMIN_TOKEN"),
		port:         envOr("PORT", "8080"),
		dbPath:       envOr("DB_PATH", "game.db"),
		scenariosDir: envOr("SCENARIOS_DIR", "scenarios"),
		webDir:       envOr("WEB_DIR", "web/dist"),
	}
	if len(cfg.adminToken) < 8 {
		return config{}, errors.New("ADMIN_TOKEN must contain at least 8 bytes")
	}
	if value, err := strconv.Atoi(cfg.port); err != nil || value < 1 || value > 65535 {
		return config{}, errors.New("PORT must be a number from 1 to 65535")
	}
	return cfg, nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	server := &http.Server{
		Addr:              net.JoinHostPort("", cfg.port),
		Handler:           httpapi.New(logger, cfg.webDir, cfg.scenariosDir),
		ReadHeaderTimeout: 5 * time.Second,
	}
	listener, err := net.Listen("tcp", server.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- server.Serve(listener)
	}()
	logger.Info("server listening", "addr", listener.Addr().String())

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
