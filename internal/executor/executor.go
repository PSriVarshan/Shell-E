package executor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"shell-e/internal/logger"
	"strings"
	"time"
)

// Result captures the output of a command execution
type Result struct {
	Success        bool
	Output         string
	Error          string
	Duration       time.Duration
	NewWorkDir     string // Set when a cd/Set-Location command changes directory
	CurrentWorkDir string // The actual working directory after execution
}

// Executor runs shell commands
type Executor struct {
	WorkingDir string
	Timeout    time.Duration
}

func NewExecutor(workingDir string) *Executor {
	return &Executor{
		WorkingDir: workingDir,
		Timeout:    30 * time.Second,
	}
}

// Execute runs a command in the specified shell.
// It detects cd/Set-Location commands and updates the working directory.
func (e *Executor) Execute(command, shell string) *Result {
	start := time.Now()
	logger.Info("Executing command: %s (shell: %s)", command, shell)

	// Detect directory change commands and handle them natively
	if newDir, ok := extractCDTarget(command); ok {
		return e.handleCD(newDir, start)
	}

	// VALIDATE WorkingDir: If it doesn't exist, fallback to current process directory
	// This prevents persistence errors where a previously valid folder was deleted
	if _, err := os.Stat(e.WorkingDir); os.IsNotExist(err) {
		logger.Error("Working directory '%s' does not exist. Falling back to default.", e.WorkingDir)
		cwd, err := os.Getwd()
		if err == nil {
			e.WorkingDir = cwd
			logger.Info("Executor working directory reset to: %s", e.WorkingDir)
		} else {
			// Absolute fallback if os.Getwd fails (rare)
			e.WorkingDir = "."
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), e.Timeout)
	defer cancel()

	var cmd *exec.Cmd

	switch strings.ToLower(shell) {
	case "cmd":
		cmd = exec.CommandContext(ctx, "cmd", "/C", command)
	default: // powershell
		cmd = exec.CommandContext(ctx, "powershell",
			"-NoProfile",
			"-NonInteractive",
			"-Command", command,
		)
	}

	cmd.Dir = e.WorkingDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	if ctx.Err() == context.DeadlineExceeded {
		logger.Error("Command timed out: %s", command)
		return &Result{
			Success:        false,
			Error:          fmt.Sprintf("Command timed out after %v", e.Timeout),
			Duration:       duration,
			CurrentWorkDir: e.WorkingDir,
		}
	}

	output := strings.TrimSpace(stdout.String())
	errStr := strings.TrimSpace(stderr.String())

	// Combine stdout and stderr for the user output
	// Many tools (like java -version) print to stderr even on success
	if errStr != "" {
		if output != "" {
			output = output + "\n" + errStr
		} else {
			output = errStr
		}
	}

	if err != nil {
		logger.Error("Command failed: %s (err: %v, stderr: %s)", command, err, errStr)

		// Handle search commands where exit code 1 means "Not Found" rather than error
		// e.g., Select-String, grep, findstr
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 {
				lowerCmd := strings.ToLower(command)
				if strings.Contains(lowerCmd, "select-string") ||
					strings.Contains(lowerCmd, "grep") ||
					strings.Contains(lowerCmd, "findstr") {
					return &Result{
						Success:        false, // Technically failed to find, but valid execution
						Output:         cleanTerminalOutput(output),
						Error:          "No matches found",
						Duration:       duration,
						CurrentWorkDir: e.WorkingDir,
					}
				}
			}
		}

		errorMsg := errStr
		if errorMsg == "" {
			errorMsg = err.Error()
		}
		return &Result{
			Success:        false,
			Output:         cleanTerminalOutput(output),
			Error:          errorMsg,
			Duration:       duration,
			CurrentWorkDir: e.WorkingDir,
		}
	}

	logger.Info("Command success: %s", command)
	return &Result{
		Success:        true,
		Output:         cleanTerminalOutput(output),
		Duration:       duration,
		CurrentWorkDir: e.WorkingDir,
	}
}

// handleCD changes the executor's working directory natively.
// This is necessary because cd/Set-Location in a subprocess doesn't
// affect the parent process.
func (e *Executor) handleCD(target string, start time.Time) *Result {
	// Resolve relative to current working dir
	var newDir string
	if filepath.IsAbs(target) {
		newDir = target
	} else {
		newDir = filepath.Join(e.WorkingDir, target)
	}

	// Clean the path
	newDir = filepath.Clean(newDir)

	// Verify directory exists
	info, err := os.Stat(newDir)
	if err != nil {
		return &Result{
			Success:        false,
			Error:          fmt.Sprintf("Cannot navigate to '%s': %v", target, err),
			Duration:       time.Since(start),
			CurrentWorkDir: e.WorkingDir,
		}
	}
	if !info.IsDir() {
		return &Result{
			Success:        false,
			Error:          fmt.Sprintf("'%s' is not a directory", target),
			Duration:       time.Since(start),
			CurrentWorkDir: e.WorkingDir,
		}
	}

	e.WorkingDir = newDir

	return &Result{
		Success:        true,
		Output:         fmt.Sprintf("Directory: %s", newDir),
		Duration:       time.Since(start),
		NewWorkDir:     newDir,
		CurrentWorkDir: newDir,
	}
}

// SetWorkingDir updates the working directory
func (e *Executor) SetWorkingDir(dir string) {
	e.WorkingDir = dir
}

// extractCDTarget detects cd/Set-Location commands and extracts the target path.
// Returns the target and true if it's a cd command, or ("", false) otherwise.
func extractCDTarget(command string) (string, bool) {
	cmd := strings.TrimSpace(command)

	// Check various cd patterns (case-insensitive)
	lower := strings.ToLower(cmd)

	// "cd 'path'" or "cd path"
	if strings.HasPrefix(lower, "cd ") {
		return cleanPathArg(cmd[3:]), true
	}

	// "Set-Location 'path'" or "Set-Location -Path 'path'"
	if strings.HasPrefix(lower, "set-location ") {
		rest := strings.TrimSpace(cmd[13:])
		// Handle -Path parameter
		lowerRest := strings.ToLower(rest)
		if strings.HasPrefix(lowerRest, "-path ") {
			rest = strings.TrimSpace(rest[6:])
		}
		return cleanPathArg(rest), true
	}

	// "sl 'path'" (alias)
	if strings.HasPrefix(lower, "sl ") {
		return cleanPathArg(cmd[3:]), true
	}

	return "", false
}

// cleanPathArg strips quotes and whitespace from a path argument
func cleanPathArg(s string) string {
	s = strings.TrimSpace(s)
	// Remove surrounding single or double quotes
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			s = s[1 : len(s)-1]
		}
	}
	return s
}

// cleanTerminalOutput handles control characters like \r to clean up progress bars
func cleanTerminalOutput(s string) string {
	// Normalize Windows line endings to prevent \r at end of line being interpreted as overwrite
	s = strings.ReplaceAll(s, "\r\n", "\n")

	if !strings.ContainsAny(s, "\r") {
		return s
	}

	var result strings.Builder
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i > 0 {
			result.WriteByte('\n')
		}

		// Handle Carriage Return (\r)
		// We keep the text after the last \r, simulating the final state of the line
		if idx := strings.LastIndexByte(line, '\r'); idx != -1 {
			line = line[idx+1:]
		}
		result.WriteString(line)
	}
	return result.String()
}
