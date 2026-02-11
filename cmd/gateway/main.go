package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/user/cc-web/internal/config"
	handler "github.com/user/cc-web/internal/http"
	"github.com/user/cc-web/internal/sessions"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	mgr := sessions.NewManager(cfg)

	// Recover existing sessions from tmux
	if err := mgr.Recover(); err != nil {
		log.Printf("Warning: session recovery: %v", err)
	}

	httpSrv := &http.Server{
		Addr:    cfg.ListenAddr,
		Handler: handler.NewServer(cfg, mgr),
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("Received %v, shutting down...", sig)

		// Give in-flight requests up to 10 seconds to complete
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpSrv.Shutdown(ctx); err != nil {
			log.Printf("HTTP shutdown error: %v", err)
		}

		// Stop all ttyd processes
		mgr.Cleanup()
		log.Println("Shutdown complete.")
	}()

	log.Printf("Claude Code Mobile Terminal listening on %s", cfg.ListenAddr)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
