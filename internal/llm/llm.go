package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// LLM defines the interface for interacting with the language model
type LLM interface {
	Start() error
	Stop() error
	Infer(prompt string, onToken func(string)) (string, error)
	IsRunning() bool
}

// ChatMessage represents a message in the OpenAI chat format
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the request body for /v1/chat/completions
type ChatRequest struct {
	Messages       []ChatMessage          `json:"messages"`
	Temperature    float64                `json:"temperature"`
	MaxTokens      int                    `json:"max_tokens,omitempty"`
	ResponseFormat map[string]interface{} `json:"response_format,omitempty"`
}

// ChatResponse is the response body from /v1/chat/completions
type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// LlamaServer implements LLM using llama-server HTTP API
type LlamaServer struct {
	BinPath      string
	ModelPath    string
	ContextSize  int
	Port         int
	SystemPrompt string // System prompt sent with every request

	cmd     *exec.Cmd
	running bool
	mu      sync.Mutex
	baseURL string
}

func NewLlamaServer(binPath, modelPath string, contextSize, port int) *LlamaServer {
	return &LlamaServer{
		BinPath:     binPath,
		ModelPath:   modelPath,
		ContextSize: contextSize,
		Port:        port,
		baseURL:     fmt.Sprintf("http://127.0.0.1:%d", port),
	}
}

// IsPortOpen checks if a port is already in use (server already running)
func IsPortOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (s *LlamaServer) Start() error {
	s.mu.Lock()

	if s.running {
		s.mu.Unlock()
		return nil
	}

	// Check if server is already running on this port (from a previous session)
	if IsPortOpen(s.Port) {
		fmt.Printf("   ✅ llama-server already running on port %d. Connecting...\n", s.Port)
		s.running = true
		s.mu.Unlock()
		return nil
	}

	bin := s.BinPath
	if bin == "" {
		bin = "llama-server"
	}

	args := []string{
		"-m", s.ModelPath,
		"-c", fmt.Sprintf("%d", s.ContextSize),
		"--host", "127.0.0.1",
		"--port", fmt.Sprintf("%d", s.Port),
	}

	s.cmd = exec.Command(bin, args...)
	// No stdout/stderr piping — just let it run detached
	// (piping can cause the process to die if buffers fill up)

	if err := s.cmd.Start(); err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to start llama-server: %w", err)
	}

	s.running = true
	fmt.Printf("   ✅ llama-server started (PID: %d)\n", s.cmd.Process.Pid)
	s.mu.Unlock()

	// Monitor for unexpected exit
	go func() {
		s.cmd.Wait()
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	// Wait for server to finish loading model and become ready
	if err := s.waitForReady(180 * time.Second); err != nil {
		s.Stop()
		return err
	}

	return nil
}

// waitForReady polls /health until the server reports "ok"
func (s *LlamaServer) waitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := s.baseURL + "/health"
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			bodyStr := string(body)

			if resp.StatusCode == 200 && strings.Contains(bodyStr, "ok") {
				return nil
			}

			if strings.Contains(bodyStr, "loading") {
				time.Sleep(1 * time.Second)
				continue
			}

			if resp.StatusCode == 200 {
				return nil
			}
		}

		s.mu.Lock()
		alive := s.running
		s.mu.Unlock()
		if !alive {
			return fmt.Errorf("llama-server process died during startup — check model path: %s", s.ModelPath)
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("llama-server startup timed out after %v — model may be too large for available RAM", timeout)
}

func (s *LlamaServer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if s.cmd != nil && s.cmd.Process != nil {
		fmt.Printf("   🛑 Stopping llama-server (PID: %d)...\n", s.cmd.Process.Pid)
		_ = s.cmd.Process.Kill()
		s.cmd.Wait()
	}

	s.running = false
	return nil
}

func (s *LlamaServer) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Infer sends a single user prompt (backward compatible — wraps InferWithHistory)
func (s *LlamaServer) Infer(prompt string, onToken func(string)) (string, error) {
	return s.InferWithHistory([]ChatMessage{{Role: "user", Content: prompt}}, onToken)
}

// InferWithHistory sends a chat completion request with full conversation history.
// The messages should be alternating user/assistant turns. The system prompt is
// automatically prepended.
func (s *LlamaServer) InferWithHistory(history []ChatMessage, onToken func(string)) (string, error) {
	s.mu.Lock()
	running := s.running
	s.mu.Unlock()

	if !running {
		return "", fmt.Errorf("llama-server not running")
	}

	// Build full messages: system prompt + conversation history
	messages := []ChatMessage{}

	if s.SystemPrompt != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: s.SystemPrompt,
		})
	}

	// Append all conversation history (user/assistant turns)
	messages = append(messages, history...)

	reqBody := ChatRequest{
		Messages:    messages,
		Temperature: 0.1,
		MaxTokens:   512,
		ResponseFormat: map[string]interface{}{
			"type": "json_object",
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 120 * time.Second}
	url := s.baseURL + "/v1/chat/completions"

	resp, err := client.Post(url, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return "", fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response choices returned")
	}

	content := strings.TrimSpace(chatResp.Choices[0].Message.Content)

	if onToken != nil {
		onToken(content)
	}

	return content, nil
}

// CouldBePartialEnd is kept for backward compatibility with existing tests
func CouldBePartialEnd(text string) bool {
	suffixes := []string{"\n> ", "\r\n> "}
	for _, pattern := range suffixes {
		for i := 1; i <= len(pattern) && i <= len(text); i++ {
			if text[len(text)-i:] == pattern[:i] {
				return true
			}
		}
	}
	return false
}
