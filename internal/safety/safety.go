package safety

import (
	"fmt"
	"strings"
)

// Level indicates the safety assessment of a command
type Level int

const (
	Safe         Level = iota // Execute immediately
	NeedsConfirm              // Ask user for confirmation
	Blocked                   // Never execute
)

// Assessment is the result of checking a command
type Assessment struct {
	Level   Level
	Reason  string
	Command string
}

// Checker validates commands before execution
type Checker struct {
	blockedPatterns []pattern
	confirmPatterns []pattern
}

type pattern struct {
	match  string
	reason string
}

func NewChecker() *Checker {
	return &Checker{
		blockedPatterns: []pattern{
			// System destruction
			{"format-volume", "Cannot format volumes — this destroys data permanently"},
			{"format c:", "Cannot format system drive"},
			{"format d:", "Cannot format drives"},
			{"remove-item -recurse c:\\windows", "Cannot delete Windows system directory"},
			{"remove-item -recurse c:/windows", "Cannot delete Windows system directory"},
			{"del /s /q c:\\windows", "Cannot delete Windows system directory"},
			{"rd /s /q c:\\windows", "Cannot delete Windows system directory"},
			{"rm -rf /", "Cannot delete root directory"},
			{"del /s /q c:\\", "Cannot recursively delete system drive"},
			{"rd /s /q c:\\", "Cannot recursively delete system drive"},
			{":(){:|:&};:", "Fork bomb detected"},
			// Registry destruction
			{"reg delete hklm", "Cannot modify system registry"},
			{"remove-itemproperty hklm:", "Cannot modify system registry"},
			// Privilege escalation
			{"net user administrator", "Cannot modify administrator account"},
			{"set-executionpolicy unrestricted", "Cannot weaken security policy"},
		},
		confirmPatterns: []pattern{
			// Process management
			{"stop-process", "This will terminate a running process"},
			{"taskkill", "This will terminate a running process"},
			{"kill", "This will terminate processes"},
			// System power
			{"shutdown", "This will shut down the computer"},
			{"restart-computer", "This will restart the computer"},
			{"stop-computer", "This will shut down the computer"},
			// File deletion
			{"remove-item", "This will delete files or folders"},
			{"del ", "This will delete files"},
			{"rmdir", "This will remove a directory"},
			{"rd ", "This will remove a directory"},
			// Network changes
			{"netsh", "This modifies network configuration"},
			{"set-dnsclientserveraddress", "This changes DNS settings"},
			// Service management
			{"stop-service", "This will stop a system service"},
			{"set-service", "This modifies a system service"},
			{"sc stop", "This will stop a system service"},
			{"sc delete", "This will delete a system service"},
		},
	}
}

// Check evaluates a command and returns a safety assessment
func (c *Checker) Check(command string) *Assessment {
	lower := strings.ToLower(strings.TrimSpace(command))

	// Check blocked patterns first
	for _, p := range c.blockedPatterns {
		if strings.Contains(lower, p.match) {
			return &Assessment{
				Level:   Blocked,
				Reason:  fmt.Sprintf("🚫 BLOCKED: %s", p.reason),
				Command: command,
			}
		}
	}

	// Check patterns that need confirmation
	for _, p := range c.confirmPatterns {
		if strings.Contains(lower, p.match) {
			return &Assessment{
				Level:   NeedsConfirm,
				Reason:  fmt.Sprintf("⚠️  %s — confirm? (y/n)", p.reason),
				Command: command,
			}
		}
	}

	// Default: safe
	return &Assessment{
		Level:   Safe,
		Reason:  "",
		Command: command,
	}
}
