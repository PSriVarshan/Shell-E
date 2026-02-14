package tests

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"shell-e/internal/llm"
)

// Verify LLM interface is satisfied by MockLLM
var _ llm.LLM = (*MockLLM)(nil)

// MockLLM simulates the LLM for testing the pipeline without a real model
type MockLLM struct {
	Running  bool
	Response string // What to return from Infer
}

func (m *MockLLM) Start() error    { m.Running = true; return nil }
func (m *MockLLM) Stop() error     { m.Running = false; return nil }
func (m *MockLLM) IsRunning() bool { return m.Running }

func (m *MockLLM) Infer(prompt string, onToken func(string)) (string, error) {
	if !m.Running {
		return "", fmt.Errorf("LLM not running")
	}

	resp := m.Response
	if resp == "" {
		resp = `{"command": null, "shell": "powershell", "response": "Hello!", "reasoning": "mock", "safe": true}`
	}

	// Simulate token streaming
	if onToken != nil {
		onToken(resp)
	}

	return resp, nil
}

// --- MockLLM Tests ---

func TestMockLLM_Interface(t *testing.T) {
	var l llm.LLM = &MockLLM{}

	if err := l.Start(); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	if !l.IsRunning() {
		t.Error("Expected running after Start")
	}

	resp, err := l.Infer("test", nil)
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	if resp == "" {
		t.Error("Expected non-empty response")
	}

	if err := l.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if l.IsRunning() {
		t.Error("Expected stopped after Stop")
	}
}

func TestMockLLM_InferNotRunning(t *testing.T) {
	m := &MockLLM{Running: false}

	_, err := m.Infer("test", nil)
	if err == nil {
		t.Error("Expected error when not running")
	}
}

func TestMockLLM_InferCustomResponse(t *testing.T) {
	m := &MockLLM{
		Running:  true,
		Response: `{"command": "mkdir test", "shell": "powershell", "response": "Creating", "reasoning": "test", "safe": true}`,
	}

	resp, err := m.Infer("create folder test", nil)
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	if !strings.Contains(resp, "mkdir test") {
		t.Errorf("Expected 'mkdir test' in response, got: %s", resp)
	}
}

func TestMockLLM_TokenCallback(t *testing.T) {
	m := &MockLLM{Running: true, Response: "Hello World"}

	var received string
	_, err := m.Infer("test", func(token string) {
		received = token
	})
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}

	if received != "Hello World" {
		t.Errorf("Expected 'Hello World' from callback, got: %s", received)
	}
}

func TestMockLLM_DefaultResponse(t *testing.T) {
	m := &MockLLM{Running: true}

	resp, err := m.Infer("hi", nil)
	if err != nil {
		t.Fatalf("Infer failed: %v", err)
	}
	if !strings.Contains(resp, `"command": null`) {
		t.Errorf("Expected null command in default response, got: %s", resp)
	}
}

// --- LlamaServer Unit Tests (no real model needed) ---

func TestLlamaServer_NewServer(t *testing.T) {
	s := llm.NewLlamaServer("test-bin", "test-model", 4096, 8055)

	if s.BinPath != "test-bin" {
		t.Errorf("Expected BinPath 'test-bin', got '%s'", s.BinPath)
	}
	if s.ModelPath != "test-model" {
		t.Errorf("Expected ModelPath 'test-model', got '%s'", s.ModelPath)
	}
	if s.ContextSize != 4096 {
		t.Errorf("Expected ContextSize 4096, got %d", s.ContextSize)
	}
	if s.Port != 8055 {
		t.Errorf("Expected Port 8055, got %d", s.Port)
	}
}

func TestLlamaServer_NotRunning(t *testing.T) {
	s := llm.NewLlamaServer("nonexistent", "nonexistent", 4096, 9999)

	if s.IsRunning() {
		t.Error("Expected not running before Start")
	}

	_, err := s.Infer("test", nil)
	if err == nil {
		t.Error("Expected error when not running")
	}
}

func TestLlamaServer_StopWhenNotStarted(t *testing.T) {
	s := llm.NewLlamaServer("test", "test", 4096, 9999)

	// Stop should not error even if never started
	err := s.Stop()
	if err != nil {
		t.Errorf("Stop on unstarted server should not error: %v", err)
	}
}

func TestCouldBePartialEnd(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"hello\n", true},     // partial "\n> "
		{"hello\n>", true},    // partial "\n> "
		{"hello\n> ", true},   // full match
		{"hello\r\n", true},   // partial "\r\n> "
		{"hello\r\n> ", true}, // full match
		{"hello world", false},
		{"no match", false},
		{">", false}, // bare > is not a partial match of "\n> "
	}

	for _, tt := range tests {
		got := llm.CouldBePartialEnd(tt.text)
		if got != tt.want {
			t.Errorf("CouldBePartialEnd(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

// Ensure LlamaServer startup fails gracefully with invalid binary
func TestLlamaServer_StartFailsWithBadBinary(t *testing.T) {
	// Use a very short timeout port that won't conflict
	s := llm.NewLlamaServer("nonexistent_binary_that_doesnt_exist", "model.gguf", 4096, 19999)

	err := s.Start()
	if err == nil {
		s.Stop()
		t.Error("Expected error starting with nonexistent binary")
	}
}

// Verify time import is used (prevent lint issues)
var _ = time.Millisecond
