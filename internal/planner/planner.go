package planner

import (
	"encoding/json"
	"fmt"
	"strings"

	"shell-e/internal/llm"
	"shell-e/internal/memory"
)

// CommandPlan is the structured output from the LLM
type CommandPlan struct {
	Command   *string `json:"command"`   // Shell command to run (null if chat-only)
	Shell     string  `json:"shell"`     // "powershell" or "cmd"
	Response  string  `json:"response"`  // Chat response to show user
	Reasoning string  `json:"reasoning"` // Brief explanation of what/why
	Safe      bool    `json:"safe"`      // LLM's self-assessment (we verify independently)
}

// Planner converts user intent into executable command plans
type Planner struct {
	llm   llm.LLM
	mem   *memory.Memory
	shell string // default shell
}

func NewPlanner(l llm.LLM, mem *memory.Memory, defaultShell string) *Planner {
	return &Planner{
		llm:   l,
		mem:   mem,
		shell: defaultShell,
	}
}

// Plan takes user input and returns a CommandPlan
func (p *Planner) Plan(userInput string) (*CommandPlan, error) {
	messages := p.buildMessages(userInput)

	var rawResponse string
	var err error

	if server, ok := p.llm.(*llm.LlamaServer); ok {
		rawResponse, err = server.InferWithHistory(messages, nil)
	} else {
		// Fallback for MockLLM — just send the user prompt
		rawResponse, err = p.llm.Infer(userInput, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("LLM inference failed: %w", err)
	}

	plan, err := p.ParseResponse(rawResponse)
	if err != nil {
		return &CommandPlan{
			Command:   nil,
			Response:  rawResponse,
			Reasoning: "Could not parse structured output, returning as chat",
		}, nil
	}

	if plan.Shell == "" {
		plan.Shell = p.shell
	}

	// Sanitize: strip CWD prefix from commands (model sometimes uses absolute paths)
	if plan.Command != nil && p.mem != nil {
		sanitized := sanitizeCommand(*plan.Command, p.mem.WorkingDir)
		plan.Command = &sanitized
	}

	return plan, nil
}

// historyPlan is used to safely marshal previous exchanges as JSON
// for the assistant turn in conversation history.
type historyPlan struct {
	Command   *string `json:"command"`
	Shell     string  `json:"shell"`
	Response  string  `json:"response"`
	Reasoning string  `json:"reasoning"`
	Safe      bool    `json:"safe"`
}

// buildMessages creates the ChatML conversation history.
// Only the last 2 exchanges are included to keep the 3B model focused.
// Previous exchanges are proper user/assistant turns so the model has
// context for follow-up requests like "use it" or "do that again".
func (p *Planner) buildMessages(userInput string) []llm.ChatMessage {
	var messages []llm.ChatMessage

	if p.mem != nil {
		ctx := p.mem.GetContext()

		// Only use last 2 exchanges — more confuses the 3B model
		exchanges := ctx.RecentExchanges
		if len(exchanges) > 2 {
			exchanges = exchanges[len(exchanges)-2:]
		}

		for _, ex := range exchanges {
			// Previous user message (just the text, no CWD — keep it clean)
			messages = append(messages, llm.ChatMessage{
				Role:    "user",
				Content: ex.UserInput,
			})

			// Previous assistant response — use json.Marshal for safe serialization
			var hp historyPlan
			hp.Shell = "powershell"
			hp.Response = ex.Response
			hp.Reasoning = "executed"
			hp.Safe = true
			if ex.Command != "" {
				hp.Command = &ex.Command
			}

			jsonBytes, err := json.Marshal(hp)
			if err != nil {
				// Fallback: just send the response text
				jsonBytes = []byte(fmt.Sprintf(`{"command":null,"response":"%s"}`, ex.Response))
			}

			messages = append(messages, llm.ChatMessage{
				Role:    "assistant",
				Content: string(jsonBytes),
			})
		}

		// Current user message with CWD
		// IMPORTANT: convert backslashes to forward slashes — the 3B model
		// corrupts paths like C:\Files\Projects when embedding them in JSON
		// because \F, \P etc. are invalid JSON escapes. Forward slashes
		// work fine in PowerShell and avoid this corruption.
		cwd := strings.ReplaceAll(ctx.WorkingDirectory, "\\", "/")
		userMsg := fmt.Sprintf("%s\n\n[CWD: %s]", userInput, cwd)
		messages = append(messages, llm.ChatMessage{
			Role:    "user",
			Content: userMsg,
		})
	} else {
		messages = append(messages, llm.ChatMessage{
			Role:    "user",
			Content: userInput,
		})
	}

	return messages
}

// ParseResponse extracts JSON from the LLM output
func (p *Planner) ParseResponse(raw string) (*CommandPlan, error) {
	raw = strings.TrimSpace(raw)

	jsonStr := ExtractJSON(raw)
	if jsonStr == "" {
		jsonStr = raw
	}

	var plan CommandPlan

	// First try: parse as-is
	err := json.Unmarshal([]byte(jsonStr), &plan)
	if err != nil {
		// Second try: fix invalid JSON escape sequences
		// (e.g., model writes \F instead of \\F in paths)
		sanitized := sanitizeJSON(jsonStr)
		err = json.Unmarshal([]byte(sanitized), &plan)
		if err != nil {
			return nil, fmt.Errorf("JSON parse error: %w", err)
		}
	}

	// Normalize: small models sometimes return "null" (string) instead of null (JSON null)
	if plan.Command != nil && (strings.TrimSpace(*plan.Command) == "null" || strings.TrimSpace(*plan.Command) == "") {
		plan.Command = nil
	}

	return &plan, nil
}

// sanitizeJSON fixes common JSON issues from small models:
// - Invalid escape sequences like \F, \P, \S (from Windows paths)
// - These get converted to proper \\F, \\P, \\S
func sanitizeJSON(s string) string {
	validEscapes := map[byte]bool{
		'"': true, '\\': true, '/': true,
		'n': true, 'r': true, 't': true, 'u': true,
		// 'b': false, 'f': false - treat these as invalid (path corruption candidates)
	}

	var result strings.Builder
	inString := false

	for i := 0; i < len(s); i++ {
		ch := s[i]

		if ch == '"' && (i == 0 || s[i-1] != '\\') {
			inString = !inString
			result.WriteByte(ch)
			continue
		}

		if inString && ch == '\\' && i+1 < len(s) {
			next := s[i+1]
			if !validEscapes[next] {
				// Invalid escape like \F — convert to \\F (escaped backslash + F)
				result.WriteString("\\\\")
				continue
			}
		}

		result.WriteByte(ch)
	}

	return result.String()
}

// sanitizeCommand strips the CWD prefix from commands when the model
// embeds absolute paths. This handles normal, forward-slash, and
// corrupted (no-separator) path variants.
func sanitizeCommand(command, workingDir string) string {
	if workingDir == "" {
		return command
	}

	// Build variants of the CWD that the model might embed:
	// 1. Original:   C:\Files\Projects\Shell-E
	// 2. Fwd slash:  C:/Files/Projects/Shell-E
	// 3. Corrupted:  C:FilesProjectsShell-E (backslashes stripped by JSON)
	variants := []string{
		workingDir + "\\",
		workingDir + "/",
		strings.ReplaceAll(workingDir, "\\", "/") + "/",
		strings.ReplaceAll(workingDir, "/", "\\") + "\\",
	}

	// Build corrupted variant (strip all slashes)
	corrupted := strings.ReplaceAll(workingDir, "\\", "")
	corrupted = strings.ReplaceAll(corrupted, "/", "")
	variants = append(variants, corrupted)

	// Try stripping each variant from the command
	result := command
	for _, prefix := range variants {
		result = strings.ReplaceAll(result, prefix, "")
	}

	// Also try case-insensitive replacement for Windows paths
	lower := strings.ToLower(result)
	for _, prefix := range variants {
		lowerPrefix := strings.ToLower(prefix)
		if idx := strings.Index(lower, lowerPrefix); idx >= 0 {
			result = result[:idx] + result[idx+len(prefix):]
			lower = strings.ToLower(result)
		}
	}

	return result
}

// ExtractJSON finds and returns the first JSON object in a string
func ExtractJSON(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}

	return ""
}
