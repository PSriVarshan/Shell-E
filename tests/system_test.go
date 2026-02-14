package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shell-e/internal/executor"
	"shell-e/internal/memory"
	"shell-e/internal/planner"
	"shell-e/internal/safety"
)

// SystemTest runs the full pipeline: MockLLM → Planner → Safety → Executor
// in a sandboxed temp directory. This tests the entire flow without a real model.

func setupSandbox(t *testing.T) (string, func()) {
	t.Helper()
	dir, err := os.MkdirTemp("", "shell-e-system-test-*")
	if err != nil {
		t.Fatalf("Failed to create sandbox: %v", err)
	}
	return dir, func() {
		os.RemoveAll(dir)
	}
}

// mockPlanner creates a planner that returns a preset JSON response
func mockPlanner(response string) (*planner.Planner, *MockLLM) {
	mock := &MockLLM{Running: true, Response: response}
	mem := memory.NewMemory(os.TempDir())
	return planner.NewPlanner(mock, mem, "powershell"), mock
}

// --- System Tests: Full Pipeline ---

func TestSystem_CreateFolder(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	folderName := "AlphaProject"

	// Simulate LLM generating a create-folder command
	llmResponse := fmt.Sprintf(`{
		"command": "New-Item -ItemType Directory -Name '%s'",
		"shell": "powershell",
		"response": "Creating folder '%s'",
		"reasoning": "User wants to create a folder",
		"safe": true
	}`, folderName, folderName)

	plan, _ := mockPlanner(llmResponse)
	exec := executor.NewExecutor(sandbox)
	checker := safety.NewChecker()

	// Run planner
	cmdPlan, err := plan.Plan("create a folder called " + folderName)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	if cmdPlan.Command == nil {
		t.Fatal("Expected command, got nil")
	}

	// Check safety
	assessment := checker.Check(*cmdPlan.Command)
	if assessment.Level != safety.Safe {
		t.Fatalf("Expected safe, got %v: %s", assessment.Level, assessment.Reason)
	}

	// Execute
	result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)
	if !result.Success {
		t.Fatalf("Execution failed: %s", result.Error)
	}

	// Verify folder exists
	expectedPath := filepath.Join(sandbox, folderName)
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Folder '%s' was not created at %s", folderName, expectedPath)
	} else {
		t.Logf("✓ Folder '%s' created successfully at %s", folderName, expectedPath)
	}
}

func TestSystem_WriteAndReadFile(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	fileName := "notes.txt"
	fileContent := "Shell-E system test data"

	// Phase 1: Write a file
	writeResponse := fmt.Sprintf(`{
		"command": "Set-Content -Path '%s' -Value '%s'",
		"shell": "powershell",
		"response": "Creating file '%s'",
		"reasoning": "Write text to file",
		"safe": true
	}`, fileName, fileContent, fileName)

	plan, _ := mockPlanner(writeResponse)
	exec := executor.NewExecutor(sandbox)

	cmdPlan, err := plan.Plan("write a file called " + fileName)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)
	if !result.Success {
		t.Fatalf("Write failed: %s", result.Error)
	}

	// Verify file exists and has content
	fullPath := filepath.Join(sandbox, fileName)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("File not readable: %v", err)
	}
	if !strings.Contains(string(data), fileContent) {
		t.Errorf("File content mismatch: expected '%s', got '%s'", fileContent, string(data))
	} else {
		t.Logf("✓ File '%s' contains expected content", fileName)
	}

	// Phase 2: Read the file
	readResponse := fmt.Sprintf(`{
		"command": "Get-Content -Path '%s'",
		"shell": "powershell",
		"response": "Reading file '%s'",
		"reasoning": "Read file contents",
		"safe": true
	}`, fileName, fileName)

	plan2, _ := mockPlanner(readResponse)
	cmdPlan2, _ := plan2.Plan("read the file " + fileName)
	result2 := exec.Execute(*cmdPlan2.Command, cmdPlan2.Shell)

	if !result2.Success {
		t.Fatalf("Read failed: %s", result2.Error)
	}
	if !strings.Contains(result2.Output, fileContent) {
		t.Errorf("Read output mismatch: expected '%s' in output, got '%s'", fileContent, result2.Output)
	} else {
		t.Logf("✓ File read returned correct content: %s", strings.TrimSpace(result2.Output))
	}
}

func TestSystem_ListDirectoryContents(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	// Pre-create some items in the sandbox
	os.MkdirAll(filepath.Join(sandbox, "Documents"), 0755)
	os.MkdirAll(filepath.Join(sandbox, "Downloads"), 0755)
	os.WriteFile(filepath.Join(sandbox, "readme.md"), []byte("# Test"), 0644)

	llmResponse := `{
		"command": "Get-ChildItem | Format-Table Name, Mode -AutoSize",
		"shell": "powershell",
		"response": "Listing directory contents",
		"reasoning": "Show files and folders",
		"safe": true
	}`

	plan, _ := mockPlanner(llmResponse)
	exec := executor.NewExecutor(sandbox)

	cmdPlan, _ := plan.Plan("list files here")
	result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)

	if !result.Success {
		t.Fatalf("List failed: %s", result.Error)
	}

	output := result.Output
	if !strings.Contains(output, "Documents") {
		t.Error("Expected 'Documents' in listing")
	}
	if !strings.Contains(output, "Downloads") {
		t.Error("Expected 'Downloads' in listing")
	}
	if !strings.Contains(output, "readme.md") {
		t.Error("Expected 'readme.md' in listing")
	}

	t.Logf("✓ Directory listing:\n%s", output)
}

func TestSystem_CheckInstalledSoftware(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	// Check for Go (we know it's installed since we're running tests)
	llmResponse := `{
		"command": "where.exe go",
		"shell": "powershell",
		"response": "Checking if Go is installed",
		"reasoning": "Find Go executable in PATH",
		"safe": true
	}`

	plan, _ := mockPlanner(llmResponse)
	exec := executor.NewExecutor(sandbox)

	cmdPlan, _ := plan.Plan("do I have Go installed?")

	assessment := safety.NewChecker().Check(*cmdPlan.Command)
	if assessment.Level != safety.Safe {
		t.Fatalf("where.exe should be safe, got: %v", assessment.Level)
	}

	result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)
	if !result.Success {
		t.Fatalf("Check failed: %s", result.Error)
	}

	if !strings.Contains(strings.ToLower(result.Output), "go") {
		t.Errorf("Expected 'go' path in output, got: %s", result.Output)
	}

	t.Logf("✓ Go found at: %s", strings.TrimSpace(result.Output))
}

func TestSystem_GetSystemInfo(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	llmResponse := `{
		"command": "$env:COMPUTERNAME",
		"shell": "powershell",
		"response": "Getting computer name",
		"reasoning": "Retrieve system hostname",
		"safe": true
	}`

	plan, _ := mockPlanner(llmResponse)
	exec := executor.NewExecutor(sandbox)

	cmdPlan, _ := plan.Plan("what is this computer's name?")
	result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)

	if !result.Success {
		t.Fatalf("System info failed: %s", result.Error)
	}

	hostname := strings.TrimSpace(result.Output)
	if hostname == "" {
		t.Error("Expected non-empty hostname")
	}

	t.Logf("✓ Computer name: %s", hostname)
}

func TestSystem_SafetyBlocksDangerous(t *testing.T) {
	// Simulate model generating a dangerous command
	llmResponse := `{
		"command": "Remove-Item -Path C:\\ -Recurse -Force",
		"shell": "powershell",
		"response": "Deleting everything",
		"reasoning": "Nuclear option",
		"safe": false
	}`

	plan, _ := mockPlanner(llmResponse)
	cmdPlan, _ := plan.Plan("delete everything on C drive")

	checker := safety.NewChecker()
	assessment := checker.Check(*cmdPlan.Command)

	if assessment.Level == safety.Safe {
		t.Error("Expected dangerous command to NOT be safe")
	}

	t.Logf("✓ Dangerous command correctly caught: level=%v, reason=%s", assessment.Level, assessment.Reason)
}

func TestSystem_ChatOnlyResponse(t *testing.T) {
	llmResponse := `{
		"command": null,
		"shell": "powershell",
		"response": "Hi! I'm Shell-E, your local AI assistant. I work offline and can run commands for you.",
		"reasoning": "Greeting response",
		"safe": true
	}`

	plan, _ := mockPlanner(llmResponse)
	cmdPlan, err := plan.Plan("hello, who are you?")
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	if cmdPlan.Command != nil {
		t.Error("Expected null command for chat response")
	}
	if cmdPlan.Response == "" {
		t.Error("Expected non-empty chat response")
	}

	t.Logf("✓ Chat response: %s", cmdPlan.Response)
}

func TestSystem_DeleteFolder(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	// Pre-create a folder to delete
	targetFolder := filepath.Join(sandbox, "TempData")
	os.MkdirAll(targetFolder, 0755)
	os.WriteFile(filepath.Join(targetFolder, "temp.txt"), []byte("temporary"), 0644)

	llmResponse := `{
		"command": "Remove-Item -Path 'TempData' -Recurse -Force",
		"shell": "powershell",
		"response": "Deleting folder 'TempData'",
		"reasoning": "Remove the temporary folder",
		"safe": false
	}`

	plan, _ := mockPlanner(llmResponse)
	exec := executor.NewExecutor(sandbox)
	checker := safety.NewChecker()

	cmdPlan, _ := plan.Plan("delete the TempData folder")

	// Safety should flag this for confirmation
	assessment := checker.Check(*cmdPlan.Command)
	if assessment.Level == safety.Safe {
		t.Error("Expected Remove-Item -Recurse to require confirmation")
	}
	t.Logf("✓ Safety flagged correctly: %s", assessment.Reason)

	// Execute anyway (simulating user confirmation)
	result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)
	if !result.Success {
		t.Fatalf("Delete failed: %s", result.Error)
	}

	// Verify folder is gone
	if _, err := os.Stat(targetFolder); !os.IsNotExist(err) {
		t.Error("Folder should have been deleted")
	} else {
		t.Logf("✓ Folder 'TempData' successfully deleted")
	}
}

func TestSystem_GetCurrentDate(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	llmResponse := `{
		"command": "Get-Date -Format 'yyyy-MM-dd HH:mm'",
		"shell": "powershell",
		"response": "Getting current date and time",
		"reasoning": "Show current timestamp",
		"safe": true
	}`

	plan, _ := mockPlanner(llmResponse)
	exec := executor.NewExecutor(sandbox)

	cmdPlan, _ := plan.Plan("what is the current date and time?")
	result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)

	if !result.Success {
		t.Fatalf("Date command failed: %s", result.Error)
	}

	dateStr := strings.TrimSpace(result.Output)
	if len(dateStr) < 10 {
		t.Errorf("Expected date string, got: %s", dateStr)
	}

	t.Logf("✓ Current date/time: %s", dateStr)
}

func TestSystem_GetDiskSpace(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	llmResponse := `{
		"command": "Get-PSDrive C | Select-Object Used, Free | Format-List",
		"shell": "powershell",
		"response": "Checking disk space on C: drive",
		"reasoning": "Show disk usage",
		"safe": true
	}`

	plan, _ := mockPlanner(llmResponse)
	exec := executor.NewExecutor(sandbox)

	cmdPlan, _ := plan.Plan("how much disk space do I have?")
	result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)

	if !result.Success {
		t.Fatalf("Disk space failed: %s", result.Error)
	}

	output := result.Output
	if !strings.Contains(output, "Used") || !strings.Contains(output, "Free") {
		t.Errorf("Expected Used/Free in output, got: %s", output)
	}

	t.Logf("✓ Disk space info:\n%s", output)
}

func TestSystem_JSONParsing_MalformedResponse(t *testing.T) {
	// Simulate model generating non-JSON garbage
	llmResponse := "I'm sorry, I can't do that."

	plan, _ := mockPlanner(llmResponse)
	cmdPlan, err := plan.Plan("do something")
	if err != nil {
		t.Fatalf("Plan should not error, got: %v", err)
	}

	// Should fallback to chat mode
	if cmdPlan.Command != nil {
		t.Error("Expected null command for malformed response")
	}
	if cmdPlan.Response != llmResponse {
		t.Errorf("Expected raw response as fallback, got: %s", cmdPlan.Response)
	}

	t.Logf("✓ Malformed JSON correctly handled as chat fallback")
}

func TestSystem_JSONParsing_WrappedInMarkdown(t *testing.T) {
	// Simulate model wrapping JSON in markdown code block
	llmResponse := "```json\n" + `{
		"command": "Get-Process | Sort-Object CPU -Descending | Select-Object -First 5",
		"shell": "powershell",
		"response": "Showing top 5 processes by CPU",
		"reasoning": "Process monitoring",
		"safe": true
	}` + "\n```"

	plan, _ := mockPlanner(llmResponse)
	cmdPlan, err := plan.Plan("show top processes")
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	if cmdPlan.Command == nil {
		t.Fatal("Expected command even from markdown-wrapped JSON")
	}

	if !strings.Contains(*cmdPlan.Command, "Get-Process") {
		t.Errorf("Expected Get-Process command, got: %s", *cmdPlan.Command)
	}

	t.Logf("✓ Extracted JSON from markdown wrapper")
}

func TestSystem_MemoryTracking(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	mem := memory.NewMemory(sandbox)

	// Record some exchanges
	mem.RecordExchange("create folder BetaProject", "New-Item -ItemType Directory -Name 'BetaProject'", "Created", "Creating folder")
	mem.RecordExchange("list files", "Get-ChildItem", "BetaProject\nREADME.md", "Listing files")

	ctx := mem.GetContext()
	if ctx.LastCreated != "BetaProject" {
		t.Errorf("Expected LastCreated='BetaProject', got '%s'", ctx.LastCreated)
	}

	// Save and reload
	if err := mem.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	mem2 := memory.NewMemory(sandbox)
	if err := mem2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	history := mem2.GetHistory()
	if len(history) != 2 {
		t.Errorf("Expected 2 history entries, got %d", len(history))
	}

	t.Logf("✓ Memory persisted %d exchanges, LastCreated=%s", len(history), ctx.LastCreated)
}

func TestSystem_FullPipelineIntegration(t *testing.T) {
	sandbox, cleanup := setupSandbox(t)
	defer cleanup()

	mem := memory.NewMemory(sandbox)
	checker := safety.NewChecker()
	exec := executor.NewExecutor(sandbox)

	// Scenario: User asks to create a workspace, then list its contents
	steps := []struct {
		name     string
		input    string
		response string
		wantCmd  bool
		wantSafe bool
	}{
		{
			name:     "Say hello",
			input:    "hey there!",
			response: `{"command":null,"shell":"powershell","response":"Hello! How can I help?","reasoning":"greeting","safe":true}`,
			wantCmd:  false,
			wantSafe: true,
		},
		{
			name:     "Create workspace",
			input:    "create a folder called workspace",
			response: `{"command":"New-Item -ItemType Directory -Name 'workspace'","shell":"powershell","response":"Creating workspace","reasoning":"mkdir","safe":true}`,
			wantCmd:  true,
			wantSafe: true,
		},
		{
			name:     "Create file in workspace",
			input:    "write config.json in workspace",
			response: `{"command":"Set-Content -Path 'workspace/config.json' -Value '{\"version\":1}'","shell":"powershell","response":"Writing config","reasoning":"create file","safe":true}`,
			wantCmd:  true,
			wantSafe: true,
		},
		{
			name:     "List workspace",
			input:    "list the workspace folder",
			response: `{"command":"Get-ChildItem workspace","shell":"powershell","response":"Listing workspace","reasoning":"ls","safe":true}`,
			wantCmd:  true,
			wantSafe: true,
		},
	}

	for i, step := range steps {
		t.Run(step.name, func(t *testing.T) {
			mock := &MockLLM{Running: true, Response: step.response}
			plan := planner.NewPlanner(mock, mem, "powershell")

			cmdPlan, err := plan.Plan(step.input)
			if err != nil {
				t.Fatalf("Step %d plan failed: %v", i, err)
			}

			if step.wantCmd {
				if cmdPlan.Command == nil {
					t.Fatalf("Step %d: expected command, got nil", i)
				}

				assessment := checker.Check(*cmdPlan.Command)
				if step.wantSafe && assessment.Level == safety.Blocked {
					t.Fatalf("Step %d: command blocked: %s", i, assessment.Reason)
				}

				result := exec.Execute(*cmdPlan.Command, cmdPlan.Shell)
				if !result.Success {
					t.Fatalf("Step %d: execution failed: %s", i, result.Error)
				}

				// Record in memory
				mem.RecordExchange(step.input, *cmdPlan.Command, result.Output, cmdPlan.Response)
				t.Logf("  ✓ [%s] Command: %s → %s", step.name, *cmdPlan.Command, truncate(result.Output, 60))
			} else {
				if cmdPlan.Command != nil {
					t.Fatalf("Step %d: expected no command, got: %s", i, *cmdPlan.Command)
				}
				t.Logf("  ✓ [%s] Chat: %s", step.name, cmdPlan.Response)
			}
		})
	}

	// Verify final state
	workspacePath := filepath.Join(sandbox, "workspace")
	if _, err := os.Stat(workspacePath); os.IsNotExist(err) {
		t.Error("workspace folder should exist")
	}
	configPath := filepath.Join(workspacePath, "config.json")
	if data, err := os.ReadFile(configPath); err != nil {
		t.Errorf("config.json should exist: %v", err)
	} else {
		var parsed map[string]interface{}
		if err := json.Unmarshal(data, &parsed); err == nil {
			t.Logf("✓ config.json is valid JSON: %v", parsed)
		}
	}
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}
