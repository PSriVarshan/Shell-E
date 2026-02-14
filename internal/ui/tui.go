package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"shell-e/internal/executor"
	"shell-e/internal/memory"
	"shell-e/internal/planner"
	"shell-e/internal/safety"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FF9F")).
			Padding(0, 1)

	statusStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#888")).
			Italic(true)

	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#7DF9FF")).
			Bold(true)

	botStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#00FF9F"))

	cmdStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFD700")).
			Italic(true)

	resultStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#CCC"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FF4444")).
			Bold(true)

	confirmStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#FFA500")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#555"))
)

// Messages for async operations
type inferDoneMsg struct {
	plan *planner.CommandPlan
	err  error
}

type execDoneMsg struct {
	result *executor.Result
	plan   *planner.CommandPlan
}

// Model is the BubbleTea model
type Model struct {
	viewport viewport.Model
	textarea textarea.Model
	spinner  spinner.Model
	planner  *planner.Planner
	executor *executor.Executor
	safety   *safety.Checker
	mem      *memory.Memory

	messages       []string
	status         string
	ready          bool
	processing     bool
	pendingConfirm *planner.CommandPlan
	width          int
	height         int
}

func NewModel(p *planner.Planner, exec *executor.Executor, s *safety.Checker, mem *memory.Memory) Model {
	ta := textarea.New()
	ta.Placeholder = "Type your request... (e.g., 'create a folder called test')"
	ta.Focus()
	ta.CharLimit = 500
	ta.SetHeight(2)
	ta.ShowLineNumbers = false

	vp := viewport.New(80, 20)

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return Model{
		viewport: vp,
		textarea: ta,
		planner:  p,
		executor: exec,
		safety:   s,
		mem:      mem,
		spinner:  sp,
		status:   "Ready",
		messages: []string{
			"🐚 Shell-E — Your local AI OS assistant",
			"Type natural language commands. I'll plan and execute them safely.",
			"Commands: /clear (reset chat) • /history (show history) • /exit (quit)",
			"",
		},
	}
}

func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC:
			return m, tea.Quit
		case tea.KeyEnter:
			if m.processing {
				return m, nil
			}

			input := strings.TrimSpace(m.textarea.Value())
			if input == "" {
				return m, nil
			}

			m.textarea.Reset()

			// Handle confirmation response
			if m.pendingConfirm != nil {
				return m.handleConfirmation(input)
			}

			// Handle slash commands
			if strings.HasPrefix(input, "/") {
				return m.handleSlashCommand(input)
			}

			// Normal input — send to planner
			m.addMessage(userStyle.Render("You: ") + input)
			m.status = "🧠 Thinking..."
			m.processing = true
			m.updateViewport()

			return m, tea.Batch(m.spinner.Tick, m.runInference(input))
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Adaptive layout: allocate space for header, input, help
		headerHeight := 1
		helpHeight := 1
		inputHeight := 4 // textarea + padding
		vpHeight := m.height - headerHeight - helpHeight - inputHeight
		if vpHeight < 3 {
			vpHeight = 3
		}

		vpWidth := m.width
		if vpWidth < 20 {
			vpWidth = 20
		}

		m.viewport.Width = vpWidth
		m.viewport.Height = vpHeight
		m.textarea.SetWidth(vpWidth)
		m.ready = true
		m.updateViewport()
		return m, nil

	case inferDoneMsg:
		if msg.err != nil {
			m.addMessage(errorStyle.Render("Error: ") + msg.err.Error())
			m.status = "Ready"
			m.processing = false
			m.updateViewport()
			return m, nil
		}
		return m.handlePlan(msg.plan)

	case spinner.TickMsg:
		if m.processing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil

	case execDoneMsg:
		return m.handleExecResult(msg.result, msg.plan)
	}

	// Update sub-components
	var cmd tea.Cmd
	m.textarea, cmd = m.textarea.Update(msg)
	cmds = append(cmds, cmd)
	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m *Model) handleSlashCommand(input string) (tea.Model, tea.Cmd) {
	switch strings.ToLower(input) {
	case "/clear":
		m.messages = m.messages[:4] // Keep header
		m.mem.Clear()
		m.addMessage(statusStyle.Render("💫 Chat cleared"))
		m.updateViewport()
	case "/history":
		history := m.mem.GetHistory()
		if len(history) == 0 {
			m.addMessage(statusStyle.Render("No history yet"))
		} else {
			m.addMessage(statusStyle.Render("📜 History:"))
			for _, ex := range history {
				m.addMessage(fmt.Sprintf("  [%s] %s → %s",
					ex.Timestamp.Format("15:04"), ex.UserInput, ex.Response))
			}
		}
		m.updateViewport()
	case "/exit":
		return m, tea.Quit
	default:
		m.addMessage(statusStyle.Render("Unknown command: " + input))
		m.updateViewport()
	}
	return m, nil
}

func (m *Model) handleConfirmation(input string) (tea.Model, tea.Cmd) {
	plan := m.pendingConfirm
	m.pendingConfirm = nil

	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "y" || lower == "yes" {
		m.addMessage(statusStyle.Render("✓ Confirmed — executing..."))
		m.status = "⚡ Executing..."
		m.updateViewport()
		return m, m.runExecution(plan)
	}

	m.addMessage(statusStyle.Render("✗ Cancelled"))
	m.status = "Ready"
	m.processing = false
	m.updateViewport()
	return m, nil
}

func (m *Model) handlePlan(plan *planner.CommandPlan) (tea.Model, tea.Cmd) {
	if plan.Command == nil || *plan.Command == "" {
		// Chat-only response
		m.addMessage(botStyle.Render("Shell-E: ") + plan.Response)
		m.mem.RecordExchange(m.getLastUserInput(), "", "", plan.Response)
		m.mem.Save()
		m.status = "Ready"
		m.processing = false
		m.updateViewport()
		return m, nil
	}

	// Has a command — show it and check safety
	cmd := *plan.Command
	m.addMessage(botStyle.Render("Shell-E: ") + plan.Response)
	m.addMessage(cmdStyle.Render("  → " + cmd))

	assessment := m.safety.Check(cmd)

	switch assessment.Level {
	case safety.Blocked:
		m.addMessage(errorStyle.Render(assessment.Reason))
		m.mem.RecordExchange(m.getLastUserInput(), cmd, "BLOCKED", assessment.Reason)
		m.mem.Save()
		m.status = "Ready"
		m.processing = false
		m.updateViewport()
		return m, nil

	case safety.NeedsConfirm:
		m.addMessage(confirmStyle.Render(assessment.Reason))
		m.pendingConfirm = plan
		m.status = "Awaiting confirmation..."
		m.processing = false
		m.updateViewport()
		return m, nil

	default: // Safe
		m.status = "⚡ Executing..."
		m.updateViewport()
		return m, tea.Batch(m.spinner.Tick, m.runExecution(plan))
	}
}

func (m *Model) handleExecResult(result *executor.Result, plan *planner.CommandPlan) (tea.Model, tea.Cmd) {
	cmd := ""
	if plan.Command != nil {
		cmd = *plan.Command
	}

	if result.Success {
		if result.Output != "" {
			output := result.Output
			lines := strings.Split(output, "\n")
			if len(lines) > 30 {
				output = strings.Join(lines[:30], "\n") + fmt.Sprintf("\n... (%d more lines)", len(lines)-30)
			}
			m.addMessage(resultStyle.Render(output))
		}
		m.addMessage(
			statusStyle.Render(fmt.Sprintf("  ✓ Done (%.1fs)", result.Duration.Seconds())))
	} else {
		errMsg := result.Error
		if result.Output != "" {
			errMsg = result.Output + "\n" + errMsg
		}
		m.addMessage(errorStyle.Render("  ✗ " + errMsg))
	}

	m.mem.RecordExchange(m.getLastUserInput(), cmd, result.Output, plan.Response)

	// Sync memory with Executor's actual state (handles cd AND fallback)
	if result.CurrentWorkDir != "" && result.CurrentWorkDir != m.mem.WorkingDir {
		m.mem.WorkingDir = result.CurrentWorkDir
	}

	m.mem.Save()

	m.status = "Ready"
	m.processing = false
	m.addMessage("")
	m.updateViewport()
	return m, nil
}

func (m *Model) runInference(input string) tea.Cmd {
	return func() tea.Msg {
		plan, err := m.planner.Plan(input)
		return inferDoneMsg{plan: plan, err: err}
	}
}

func (m *Model) runExecution(plan *planner.CommandPlan) tea.Cmd {
	return func() tea.Msg {
		cmd := ""
		shell := "powershell"
		if plan.Command != nil {
			cmd = *plan.Command
		}
		if plan.Shell != "" {
			shell = plan.Shell
		}
		result := m.executor.Execute(cmd, shell)
		return execDoneMsg{result: result, plan: plan}
	}
}

// addMessage adds a message with word wrapping to fit the viewport width
func (m *Model) addMessage(msg string) {
	if m.width > 4 {
		msg = wrapText(msg, m.width-2)
	}
	m.messages = append(m.messages, msg)
}

func (m *Model) getLastUserInput() string {
	for i := len(m.messages) - 1; i >= 0; i-- {
		if strings.Contains(m.messages[i], "You: ") {
			parts := strings.SplitN(m.messages[i], "You: ", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(stripAnsi(parts[1]))
			}
		}
	}
	return ""
}

func (m *Model) updateViewport() {
	content := strings.Join(m.messages, "\n")
	m.viewport.SetContent(content)
	m.viewport.GotoBottom()
}

func (m Model) View() string {
	if !m.ready {
		return "Loading Shell-E..."
	}

	header := ""
	if m.processing {
		header = fmt.Sprintf("%s %s %s", titleStyle.Render("🐚 Shell-E"), m.spinner.View(), statusStyle.Render(m.status))
	} else {
		header = titleStyle.Render("🐚 Shell-E") + "  " + statusStyle.Render(m.status)
	}

	chatArea := m.viewport.View()

	input := m.textarea.View()

	help := helpStyle.Render(" Enter: send • /clear: reset • /exit: quit • Ctrl+C: force quit")

	return fmt.Sprintf("%s\n%s\n%s\n%s", header, chatArea, input, help)
}

// wrapText soft-wraps a string at maxWidth, respecting word boundaries
func wrapText(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return s
	}

	var result strings.Builder
	lines := strings.Split(s, "\n")

	for i, line := range lines {
		if i > 0 {
			result.WriteByte('\n')
		}

		// Get the visible length (without ANSI codes)
		plain := stripAnsi(line)
		if len(plain) <= maxWidth {
			result.WriteString(line)
			continue
		}

		// Need to wrap — work on the plain text, then re-wrap
		words := strings.Fields(plain)
		currentLen := 0
		for j, word := range words {
			wordLen := len(word)
			if currentLen+wordLen+1 > maxWidth && currentLen > 0 {
				result.WriteByte('\n')
				currentLen = 0
			}
			if currentLen > 0 {
				result.WriteByte(' ')
				currentLen++
			} else if j > 0 {
				// continuation line — no extra indent
			}
			result.WriteString(word)
			currentLen += wordLen
		}
	}

	return result.String()
}

// stripAnsi removes ANSI escape codes for clean text measurement
func stripAnsi(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++ // skip the 'm'
			continue
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}
