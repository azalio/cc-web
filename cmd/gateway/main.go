package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

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

	srv := handler.NewServer(cfg, mgr)

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("Shutting down...")
		os.Exit(0)
	}()

	log.Printf("Claude Code Mobile Terminal listening on %s", cfg.ListenAddr)
	if err := http.ListenAndServe(cfg.ListenAddr, srv); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
