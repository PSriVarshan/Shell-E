package tests

import (
	"os"
	"path/filepath"
	"testing"

	"shell-e/internal/config"
)

func TestLoadConfig_Defaults(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.ModelPath != "assets/localmodel/qwen2.5-3b-instruct-q4_k_m.gguf" {
		t.Errorf("Expected Qwen2.5 model path, got: %s", cfg.ModelPath)
	}
	if cfg.LlamaBinPath != "assets/bin/llama-server.exe" {
		t.Errorf("Expected llama-server bin path, got: %s", cfg.LlamaBinPath)
	}
	if cfg.ContextSize != 4096 {
		t.Errorf("Expected context size 4096, got: %d", cfg.ContextSize)
	}
	if cfg.Shell != "powershell" {
		t.Errorf("Expected shell 'powershell', got: %s", cfg.Shell)
	}
	if cfg.ServerPort != 8055 {
		t.Errorf("Expected server port 8055, got: %d", cfg.ServerPort)
	}
}

func TestLoadConfig_DataDirectory(t *testing.T) {
	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	dir := cfg.DataDirectory()
	if dir == "" {
		t.Error("Expected non-empty data directory")
	}
}

func TestLoadConfig_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(configPath, []byte("context_size: 8192\nshell: cmd\n"), 0644)

	origDir, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origDir)

	cfg, err := config.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.ContextSize != 8192 {
		t.Errorf("Expected ContextSize 8192, got %d", cfg.ContextSize)
	}
	if cfg.Shell != "cmd" {
		t.Errorf("Expected Shell 'cmd', got %s", cfg.Shell)
	}
}
