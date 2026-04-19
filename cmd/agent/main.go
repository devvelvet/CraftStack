package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	agent "craftstack/internal/agent"
	"craftstack/internal/common"
)

// Build-time variables (injected via -ldflags)
var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	configPath := flag.String("config", "configs/agent.yaml", "path to agent config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("CraftStack Agent %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	// Load configuration (falls back to embedded defaults if file not found)
	cfg, err := common.LoadAgentConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := common.NewLogger(cfg.Log)
	slog.SetDefault(log)

	log.Info("CraftStack Agent starting",
		"version", version,
		"agent_name", cfg.Agent.Name,
		"master_addr", cfg.Master.Addr,
	)

	// Create and start agent
	a := agent.New(cfg, log)
	if err := a.Start(); err != nil {
		log.Error("failed to start agent", "error", err)
		os.Exit(1)
	}

	log.Info("CraftStack Agent is running")

	// Wait for shutdown signal (os.Interrupt works on all platforms including Windows)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info("shutdown new number receive, instance shutdown during...", "signal", sig)

	// second new number receive when force shutdown
	go func() {
		<-quit
		log.Warn("force shutdown")
		os.Exit(1)
	}()

	if err := a.Stop(); err != nil {
		log.Error("agent shutdown during error", "error", err)
	}

	log.Info("CraftStack Agent stopped")
}
