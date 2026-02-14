package tests

import (
	"os"
	"path/filepath"
	"testing"

	"shell-e/internal/memory"
)

func TestMemory_NewMemory(t *testing.T) {
	m := memory.NewMemory(t.TempDir())
	if m.WorkingDir == "" {
		t.Error("Expected non-empty working directory")
	}
}

func TestMemory_SaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	m1 := memory.NewMemory(dir)
	m1.RecordExchange("create folder", "mkdir test", "", "Created test folder")
	if err := m1.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	m2 := memory.NewMemory(dir)
	if err := m2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if len(m2.Exchanges) != 1 {
		t.Errorf("Expected 1 exchange, got %d", len(m2.Exchanges))
	}
	if m2.Exchanges[0].UserInput != "create folder" {
		t.Errorf("Expected UserInput 'create folder', got: %s", m2.Exchanges[0].UserInput)
	}
}

func TestMemory_LoadNonExistent(t *testing.T) {
	m := memory.NewMemory(t.TempDir())
	err := m.Load()
	if err != nil {
		t.Errorf("Expected no error for non-existent file, got: %v", err)
	}
}

func TestMemory_GetContext(t *testing.T) {
	m := memory.NewMemory(t.TempDir())
	m.RecordExchange("hello", "", "", "Hi!")
	m.RecordExchange("list files", "dir", "file1.txt", "Here are your files")

	ctx := m.GetContext()
	if ctx.WorkingDirectory == "" {
		t.Error("Expected non-empty working directory")
	}
	if len(ctx.RecentExchanges) != 2 {
		t.Errorf("Expected 2 recent exchanges, got %d", len(ctx.RecentExchanges))
	}
}

func TestMemory_ContextLimit(t *testing.T) {
	m := memory.NewMemory(t.TempDir())
	m.MaxExchanges = 3

	for i := 0; i < 10; i++ {
		m.RecordExchange("msg", "", "", "resp")
	}

	ctx := m.GetContext()
	if len(ctx.RecentExchanges) > 3 {
		t.Errorf("Expected at most 3 recent exchanges, got %d", len(ctx.RecentExchanges))
	}
}

func TestMemory_Clear(t *testing.T) {
	m := memory.NewMemory(t.TempDir())
	m.RecordExchange("test", "cmd", "result", "response")
	m.Clear()

	ctx := m.GetContext()
	if len(ctx.RecentExchanges) != 0 {
		t.Errorf("Expected 0 exchanges after clear, got %d", len(ctx.RecentExchanges))
	}
}

func TestMemory_Compaction(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "memory"), 0755)
	m := memory.NewMemory(dir)
	m.CompactAfter = 5
	m.MaxExchanges = 3

	for i := 0; i < 10; i++ {
		m.RecordExchange("msg", "cmd", "result", "response")
	}

	entries, _ := os.ReadDir(filepath.Join(dir, "memory"))
	if len(entries) == 0 {
		t.Error("Expected daily note to be created during compaction")
	}
}

func TestExtractNameFromCommand(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{"New-Item -ItemType Directory -Name 'test'", "test"},
		{"New-Item -ItemType Directory -Name myFolder", "myFolder"},
		{"mkdir hello", "hello"},
		{"echo nothing", ""},
	}

	for _, tt := range tests {
		got := memory.ExtractNameFromCommand(tt.cmd)
		if got != tt.want {
			t.Errorf("ExtractNameFromCommand(%q) = %q, want %q", tt.cmd, got, tt.want)
		}
	}
}

func TestContextInfo_FormatForPrompt(t *testing.T) {
	m := memory.NewMemory(t.TempDir())
	m.RecordExchange("hello", "", "", "Hi!")

	ctx := m.GetContext()
	formatted := ctx.FormatForPrompt()
	if formatted == "" {
		t.Error("Expected non-empty formatted context")
	}
}
