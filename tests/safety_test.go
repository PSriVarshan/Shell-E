package tests

import (
	"testing"

	"shell-e/internal/safety"
)

func TestChecker_SafeCommands(t *testing.T) {
	c := safety.NewChecker()
	safe := []string{
		"Get-ChildItem",
		"mkdir test",
		"echo hello",
		"where.exe python",
		"New-Item -ItemType Directory -Name 'project'",
		"Get-Process | Sort-Object CPU -Descending | Select-Object -First 10",
		"systeminfo",
		"ipconfig",
		"explorer.exe .",
	}

	for _, cmd := range safe {
		a := c.Check(cmd)
		if a.Level != safety.Safe {
			t.Errorf("Expected Safe for %q, got level %d: %s", cmd, a.Level, a.Reason)
		}
	}
}

func TestChecker_BlockedCommands(t *testing.T) {
	c := safety.NewChecker()
	blocked := []string{
		"Format-Volume -DriveLetter C",
		"format c:",
		"Remove-Item -Recurse C:\\Windows",
		"del /s /q C:\\Windows\\System32",
		"rd /s /q C:\\",
		"Set-ExecutionPolicy Unrestricted",
		"reg delete HKLM\\Software\\Test",
	}

	for _, cmd := range blocked {
		a := c.Check(cmd)
		if a.Level != safety.Blocked {
			t.Errorf("Expected Blocked for %q, got level %d", cmd, a.Level)
		}
	}
}

func TestChecker_ConfirmCommands(t *testing.T) {
	c := safety.NewChecker()
	confirm := []string{
		"Stop-Process -Name notepad",
		"taskkill /IM notepad.exe",
		"shutdown /s /t 0",
		"Restart-Computer",
		"Remove-Item test.txt",
		"del test.txt",
		"rmdir /s testdir",
		"Stop-Service wuauserv",
		"netsh interface set interface 'Wi-Fi' disable",
	}

	for _, cmd := range confirm {
		a := c.Check(cmd)
		if a.Level != safety.NeedsConfirm {
			t.Errorf("Expected NeedsConfirm for %q, got level %d", cmd, a.Level)
		}
	}
}

func TestChecker_CaseInsensitive(t *testing.T) {
	c := safety.NewChecker()

	a := c.Check("FORMAT-VOLUME -DriveLetter C")
	if a.Level != safety.Blocked {
		t.Errorf("Expected Blocked for uppercase, got level %d", a.Level)
	}

	a = c.Check("SHUTDOWN /s /t 0")
	if a.Level != safety.NeedsConfirm {
		t.Errorf("Expected NeedsConfirm for uppercase, got level %d", a.Level)
	}
}

func TestChecker_EmptyCommand(t *testing.T) {
	c := safety.NewChecker()
	a := c.Check("")
	if a.Level != safety.Safe {
		t.Errorf("Expected Safe for empty command, got level %d", a.Level)
	}
}
