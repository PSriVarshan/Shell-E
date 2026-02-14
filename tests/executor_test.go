package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell-e/internal/executor"
)

func TestExecute_PowerShell_SimpleCommand(t *testing.T) {
	e := executor.NewExecutor(os.TempDir())
	result := e.Execute("Write-Output 'hello world'", "powershell")

	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello world") {
		t.Errorf("Expected 'hello world' in output, got: %s", result.Output)
	}
}

func TestExecute_PowerShell_CreateFolder(t *testing.T) {
	tmpDir := t.TempDir()
	e := executor.NewExecutor(tmpDir)

	folderName := "test_folder_shell_e"
	result := e.Execute("New-Item -ItemType Directory -Name '"+folderName+"'", "powershell")

	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}

	path := filepath.Join(tmpDir, folderName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Folder not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected directory, got file")
	}
}

func TestExecute_PowerShell_ListFiles(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte("hello"), 0644)

	e := executor.NewExecutor(tmpDir)
	result := e.Execute("Get-ChildItem | Select-Object -ExpandProperty Name", "powershell")

	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "test.txt") {
		t.Errorf("Expected 'test.txt' in output, got: %s", result.Output)
	}
}

func TestExecute_CMD_SimpleCommand(t *testing.T) {
	e := executor.NewExecutor(os.TempDir())
	result := e.Execute("echo hello", "cmd")

	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello") {
		t.Errorf("Expected 'hello' in output, got: %s", result.Output)
	}
}

func TestExecute_InvalidCommand(t *testing.T) {
	e := executor.NewExecutor(os.TempDir())
	result := e.Execute("nonexistent_command_12345", "powershell")

	if result.Success {
		t.Error("Expected failure for invalid command")
	}
}

func TestExecute_WorkingDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	e := executor.NewExecutor(tmpDir)

	result := e.Execute("Get-Location | Select-Object -ExpandProperty Path", "powershell")
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}

	if !strings.Contains(strings.ToLower(result.Output), strings.ToLower(filepath.Base(tmpDir))) {
		t.Errorf("Expected working dir in output, got: %s", result.Output)
	}
}

func TestExecute_Duration(t *testing.T) {
	e := executor.NewExecutor(os.TempDir())
	result := e.Execute("Write-Output 'fast'", "powershell")

	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}
}

func TestExecute_PowerShell_StderrCapture(t *testing.T) {
	e := executor.NewExecutor(os.TempDir())
	// Use a command that writes to stderr but exits with 0
	cmd := "[Console]::Error.WriteLine('hello stderr'); exit 0"
	result := e.Execute(cmd, "powershell")

	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "hello stderr") {
		t.Errorf("Expected 'hello stderr' in output, got: %s", result.Output)
	}
}

func TestExecute_SearchExitCode(t *testing.T) {
	e := executor.NewExecutor(os.TempDir())
	// findstr in cmd returns exit code 1 if string not found
	// We want this to be handled as "No matches found"
	cmd := "echo apple | findstr orange"
	result := e.Execute(cmd, "cmd")

	if result.Success {
		t.Error("Expected failure (Success=false) for no matches")
	}
	if result.Error != "No matches found" {
		t.Errorf("Expected 'No matches found' error, got: %s", result.Error)
	}
}

func TestExecute_DeletedWorkingDir(t *testing.T) {
	// Create a temp dir
	tmpDir := t.TempDir()
	deletedDir := filepath.Join(tmpDir, "to_be_deleted")
	os.Mkdir(deletedDir, 0755)

	// Initialize executor with this dir
	e := executor.NewExecutor(deletedDir)

	// DELETE the directory
	os.RemoveAll(deletedDir)

	// Execute a simple command
	// Should NOT fail with "no such file or directory" because of fallback
	result := e.Execute("Write-Output 'fallback worked'", "powershell")

	if !result.Success {
		t.Fatalf("Expected success after fallback, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "fallback worked") {
		t.Errorf("Expected output from fallback execution, got: %s", result.Output)
	}
}
