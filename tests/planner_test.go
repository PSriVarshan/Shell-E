package tests

import (
	"testing"

	"shell-e/internal/planner"
)

func TestExtractJSON_Simple(t *testing.T) {
	input := `{"command": "mkdir test", "shell": "powershell", "response": "Creating folder", "reasoning": "test", "safe": true}`
	result := planner.ExtractJSON(input)
	if result != input {
		t.Errorf("Expected exact JSON, got: %s", result)
	}
}

func TestExtractJSON_WithMarkdown(t *testing.T) {
	input := "```json\n{\"command\": \"mkdir test\"}\n```"
	result := planner.ExtractJSON(input)
	if result != `{"command": "mkdir test"}` {
		t.Errorf("Expected extracted JSON, got: %s", result)
	}
}

func TestExtractJSON_WithTextBefore(t *testing.T) {
	input := "Here is the command:\n{\"command\": \"dir\"}"
	result := planner.ExtractJSON(input)
	if result != `{"command": "dir"}` {
		t.Errorf("Expected extracted JSON, got: %s", result)
	}
}

func TestExtractJSON_NoJSON(t *testing.T) {
	input := "Just some text without any JSON"
	result := planner.ExtractJSON(input)
	if result != "" {
		t.Errorf("Expected empty string, got: %s", result)
	}
}

func TestExtractJSON_NestedBraces(t *testing.T) {
	input := `{"command": "echo {hello}", "safe": true}`
	result := planner.ExtractJSON(input)
	if result != input {
		t.Errorf("Expected full JSON with nested braces, got: %s", result)
	}
}

func TestExtractJSON_Empty(t *testing.T) {
	result := planner.ExtractJSON("")
	if result != "" {
		t.Errorf("Expected empty string, got: %s", result)
	}
}

func TestParseResponse_ValidCommand(t *testing.T) {
	p := planner.NewPlanner(nil, nil, "powershell")
	raw := `{"command": "mkdir test", "shell": "powershell", "response": "Creating folder", "reasoning": "test", "safe": true}`

	plan, err := p.ParseResponse(raw)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if plan.Command == nil || *plan.Command != "mkdir test" {
		t.Errorf("Expected command 'mkdir test', got: %v", plan.Command)
	}
	if plan.Shell != "powershell" {
		t.Errorf("Expected shell 'powershell', got: %s", plan.Shell)
	}
	if !plan.Safe {
		t.Error("Expected safe=true")
	}
}

func TestParseResponse_NullCommand(t *testing.T) {
	p := planner.NewPlanner(nil, nil, "powershell")
	raw := `{"command": null, "response": "Hello!", "reasoning": "greeting", "safe": true}`

	plan, err := p.ParseResponse(raw)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if plan.Command != nil {
		t.Errorf("Expected null command, got: %v", plan.Command)
	}
	if plan.Response != "Hello!" {
		t.Errorf("Expected response 'Hello!', got: %s", plan.Response)
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	p := planner.NewPlanner(nil, nil, "powershell")
	raw := "This is not JSON at all"

	_, err := p.ParseResponse(raw)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
}

func TestParseResponse_UnsafeCommand(t *testing.T) {
	p := planner.NewPlanner(nil, nil, "powershell")
	raw := `{"command": "Remove-Item -Recurse temp", "shell": "powershell", "response": "Deleting temp", "reasoning": "cleanup", "safe": false}`

	plan, err := p.ParseResponse(raw)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if plan.Safe {
		t.Error("Expected safe=false")
	}
}
