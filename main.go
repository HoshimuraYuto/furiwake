package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "furiwake.yaml", "path to config yaml")
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	logger := NewLogger()
	srv := NewServer(cfg, logger)

	go func() {
		logger.Infof("furiwake listening on %s (timeout=%ds)", cfg.Listen, cfg.TimeoutSeconds)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Errorf("server error: %v", err)
			os.Exit(1)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Warnf("received signal %s, shutting down", sig.String())

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Errorf("graceful shutdown failed: %v", err)
		os.Exit(1)
	}
}
