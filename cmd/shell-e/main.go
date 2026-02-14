package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"shell-e/internal/config"
	"shell-e/internal/executor"
	"shell-e/internal/llm"
	"shell-e/internal/logger"
	"shell-e/internal/memory"
	"shell-e/internal/planner"
	"shell-e/internal/safety"
	"shell-e/internal/ui"
)

func main() {
	// Initialize Logger
	if err := logger.Init("shell-e.log"); err != nil {
		fmt.Printf("Error initializing logger: %v\n", err)
	}
	defer logger.Close()

	logger.Info("Staritng Shell-E...")

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load config: %v", err)
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize memory
	mem := memory.NewMemory(cfg.DataDirectory())
	if err := mem.Load(); err != nil {
		log.Printf("Warning: could not load memory: %v", err)
	}

	// Initialize LLM server
	server := llm.NewLlamaServer(cfg.LlamaBinPath, cfg.ModelPath, cfg.ContextSize, cfg.ServerPort)
	server.SystemPrompt = planner.SystemPrompt

	// Setup signal handling for clean shutdown (Ctrl+C kills server)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		server.Stop()
		os.Exit(0)
	}()

	fmt.Println("🐚 Starting Shell-E...")
	fmt.Printf("   Model: %s\n", cfg.ModelPath)
	fmt.Println("   Starting local AI server — this may take a minute on first run...")
	fmt.Println("   (You don't need to do anything — just wait)")

	if err := server.Start(); err != nil {
		log.Fatalf("Failed to start AI server: %v", err)
	}
	defer server.Stop()

	fmt.Println("   ✅ AI server ready!")

	// Initialize components
	exec := executor.NewExecutor(mem.WorkingDir)
	safetyChecker := safety.NewChecker()
	plan := planner.NewPlanner(server, mem, cfg.Shell)

	// Build TUI
	m := ui.NewModel(plan, exec, safetyChecker, mem)

	// Start BubbleTea
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Save memory on exit
	mem.Save()
	fmt.Println("👋 Shell-E closed. Memory saved.")
}
