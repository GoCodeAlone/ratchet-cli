package pages

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
)

// OnboardingDoneMsg signals provider setup is complete.
type OnboardingDoneMsg struct {
	Provider *pb.Provider
}

type providerAddedMsg struct {
	provider *pb.Provider
	err      error
}

type providerTestedMsg struct {
	result *pb.TestProviderResult
	err    error
}

type onboardingStep int

const (
	stepSelectProvider onboardingStep = iota
	stepEnterAPIKey
	stepEnterBaseURL
	stepSelectModel
	stepTestConnection
)

type providerTypeInfo struct {
	name          string
	displayName   string
	needsAPIKey   bool
	needsBaseURL  bool
	defaultURL    string
	defaultModels []string
}

var providerTypes = []providerTypeInfo{
	{
		name: "anthropic", displayName: "Anthropic (Claude)",
		needsAPIKey: true, needsBaseURL: false,
		defaultModels: []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-4-20250514"},
	},
	{
		name: "openai", displayName: "OpenAI (GPT)",
		needsAPIKey: true, needsBaseURL: true,
		defaultURL:    "https://api.openai.com/v1",
		defaultModels: []string{"gpt-4o", "gpt-4o-mini", "o3-mini"},
	},
	{
		name: "ollama", displayName: "Ollama (Local)",
		needsAPIKey: false, needsBaseURL: true,
		defaultURL:    "http://localhost:11434",
		defaultModels: []string{"llama3.3", "codellama", "mistral"},
	},
	{
		name: "gemini", displayName: "Google Gemini",
		needsAPIKey: true, needsBaseURL: false,
		defaultModels: []string{"gemini-2.5-pro", "gemini-2.5-flash"},
	},
}

// OnboardingModel is the multi-step provider setup wizard.
type OnboardingModel struct {
	client *client.Client
	step   onboardingStep

	// Provider selection
	cursor int

	// API key input
	apiKeyInput textinput.Model

	// Base URL input
	baseURLInput textinput.Model

	// Model selection
	modelCursor int

	// Connection test
	spinner    spinner.Model
	testing    bool
	testResult *pb.TestProviderResult
	testError  string
	added      bool // provider has been added to daemon

	width  int
	height int
}

func NewOnboarding(c *client.Client, t theme.Theme) OnboardingModel {
	apiKey := textinput.New()
	apiKey.Placeholder = "sk-..."
	apiKey.EchoMode = textinput.EchoPassword
	apiKey.EchoCharacter = '*'
	apiKey.Prompt = ""

	baseURL := textinput.New()
	baseURL.Placeholder = "http://localhost:11434"
	baseURL.Prompt = ""

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(t.Primary)

	return OnboardingModel{
		client:   c,
		step:     stepSelectProvider,
		apiKeyInput:  apiKey,
		baseURLInput: baseURL,
		spinner:  sp,
	}
}

func (m OnboardingModel) Init() tea.Cmd {
	return nil
}

func (m OnboardingModel) selectedProvider() providerTypeInfo {
	return providerTypes[m.cursor]
}

func (m OnboardingModel) Update(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case providerAddedMsg:
		if msg.err != nil {
			m.testing = false
			m.testError = msg.err.Error()
			return m, nil
		}
		m.added = true
		// Now test the connection
		return m, m.testProvider(msg.provider.Alias)

	case providerTestedMsg:
		m.testing = false
		if msg.err != nil {
			m.testError = msg.err.Error()
			return m, nil
		}
		m.testResult = msg.result
		if !msg.result.Success {
			m.testError = msg.result.Message
		}
		return m, nil

	case spinner.TickMsg:
		if m.testing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.step {
	case stepSelectProvider:
		return m.updateSelectProvider(msg)
	case stepEnterAPIKey:
		return m.updateEnterAPIKey(msg)
	case stepEnterBaseURL:
		return m.updateEnterBaseURL(msg)
	case stepSelectModel:
		return m.updateSelectModel(msg)
	case stepTestConnection:
		return m.updateTestConnection(msg)
	}

	return m, nil
}

func (m OnboardingModel) updateSelectProvider(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "j", "down":
			if m.cursor < len(providerTypes)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "1":
			m.cursor = 0
		case "2":
			m.cursor = 1
		case "3":
			m.cursor = 2
		case "4":
			m.cursor = 3
		case "enter", " ":
			return m.advanceFromProvider()
		}
	}
	return m, nil
}

func (m OnboardingModel) advanceFromProvider() (OnboardingModel, tea.Cmd) {
	p := m.selectedProvider()
	if p.needsAPIKey {
		m.step = stepEnterAPIKey
		return m, m.apiKeyInput.Focus()
	}
	if p.needsBaseURL {
		m.step = stepEnterBaseURL
		m.baseURLInput.SetValue(p.defaultURL)
		return m, m.baseURLInput.Focus()
	}
	m.step = stepSelectModel
	m.modelCursor = 0
	return m, nil
}

func (m OnboardingModel) updateEnterAPIKey(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "escape":
			m.step = stepSelectProvider
			m.apiKeyInput.SetValue("")
			return m, nil
		case "enter":
			if m.apiKeyInput.Value() == "" {
				return m, nil
			}
			p := m.selectedProvider()
			if p.needsBaseURL {
				m.step = stepEnterBaseURL
				m.baseURLInput.SetValue(p.defaultURL)
				return m, m.baseURLInput.Focus()
			}
			m.step = stepSelectModel
			m.modelCursor = 0
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	return m, cmd
}

func (m OnboardingModel) updateEnterBaseURL(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "escape":
			p := m.selectedProvider()
			if p.needsAPIKey {
				m.step = stepEnterAPIKey
				return m, m.apiKeyInput.Focus()
			}
			m.step = stepSelectProvider
			return m, nil
		case "enter":
			if m.baseURLInput.Value() == "" {
				return m, nil
			}
			m.step = stepSelectModel
			m.modelCursor = 0
			return m, nil
		}
	}

	var cmd tea.Cmd
	m.baseURLInput, cmd = m.baseURLInput.Update(msg)
	return m, cmd
}

func (m OnboardingModel) updateSelectModel(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	p := m.selectedProvider()
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "escape":
			// Go back
			if p.needsBaseURL {
				m.step = stepEnterBaseURL
				return m, m.baseURLInput.Focus()
			}
			if p.needsAPIKey {
				m.step = stepEnterAPIKey
				return m, m.apiKeyInput.Focus()
			}
			m.step = stepSelectProvider
			return m, nil
		case "j", "down":
			if m.modelCursor < len(p.defaultModels)-1 {
				m.modelCursor++
			}
		case "k", "up":
			if m.modelCursor > 0 {
				m.modelCursor--
			}
		case "enter", " ":
			return m.startTest()
		}
		// Number shortcuts
		for i := range p.defaultModels {
			if keyMsg.String() == fmt.Sprintf("%d", i+1) && i < len(p.defaultModels) {
				m.modelCursor = i
			}
		}
	}
	return m, nil
}

func (m OnboardingModel) startTest() (OnboardingModel, tea.Cmd) {
	m.step = stepTestConnection
	m.testing = true
	m.testResult = nil
	m.testError = ""
	m.added = false

	p := m.selectedProvider()
	model := p.defaultModels[m.modelCursor]

	return m, tea.Batch(
		m.spinner.Tick,
		m.addProvider(p, model),
	)
}

func (m OnboardingModel) addProvider(p providerTypeInfo, model string) tea.Cmd {
	return func() tea.Msg {
		req := &pb.AddProviderReq{
			Alias:     p.name,
			Type:      p.name,
			Model:     model,
			ApiKey:    m.apiKeyInput.Value(),
			BaseUrl:   m.baseURLInput.Value(),
			IsDefault: true,
		}
		provider, err := m.client.AddProvider(context.Background(), req)
		return providerAddedMsg{provider: provider, err: err}
	}
}

func (m OnboardingModel) testProvider(alias string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.client.TestProvider(context.Background(), alias)
		return providerTestedMsg{result: result, err: err}
	}
}

func (m OnboardingModel) updateTestConnection(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		// Only handle keys when test is done
		if m.testing {
			return m, nil
		}

		if m.testResult != nil && m.testResult.Success {
			// Success — any key proceeds
			switch keyMsg.String() {
			case "enter", " ":
				p := m.selectedProvider()
				model := p.defaultModels[m.modelCursor]
				return m, func() tea.Msg {
					return OnboardingDoneMsg{
						Provider: &pb.Provider{
							Alias:     p.name,
							Type:      p.name,
							Model:     model,
							IsDefault: true,
						},
					}
				}
			}
			return m, nil
		}

		// Failure — offer retry or back
		switch keyMsg.String() {
		case "r":
			// Retry: re-test (provider already added)
			if m.added {
				m.testing = true
				m.testError = ""
				p := m.selectedProvider()
				return m, tea.Batch(m.spinner.Tick, m.testProvider(p.name))
			}
			return m.startTest()
		case "b", "escape":
			// Remove the broken provider and go back
			if m.added {
				p := m.selectedProvider()
				go m.client.RemoveProvider(context.Background(), p.name) //nolint:errcheck
				m.added = false
			}
			m.step = stepSelectModel
			m.testResult = nil
			m.testError = ""
			return m, nil
		}
	}
	return m, nil
}

func (m OnboardingModel) View(t theme.Theme, width, height int) string {
	w := width
	h := height
	if m.width > 0 {
		w = m.width
	}
	if m.height > 0 {
		h = m.height
	}

	var sb strings.Builder
	titleStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(t.Muted)
	successStyle := lipgloss.NewStyle().Foreground(t.Success)
	errorStyle := lipgloss.NewStyle().Foreground(t.Error)

	sb.WriteString(titleStyle.Render("Welcome to ratchet") + "\n\n")

	// Step progress dots
	steps := m.stepCount()
	current := m.currentStepIndex()
	var dots []string
	for i := 0; i < steps; i++ {
		if i <= current {
			dots = append(dots, lipgloss.NewStyle().Foreground(t.Primary).Render("●"))
		} else {
			dots = append(dots, mutedStyle.Render("○"))
		}
	}
	sb.WriteString(strings.Join(dots, " ") + "\n\n")

	switch m.step {
	case stepSelectProvider:
		sb.WriteString("Select your AI provider:\n\n")
		for i, p := range providerTypes {
			cursor := "  "
			style := mutedStyle
			if i == m.cursor {
				cursor = "▶ "
				style = lipgloss.NewStyle().Foreground(t.Foreground).Bold(true)
			}
			label := fmt.Sprintf("%s%d. %s", cursor, i+1, p.displayName)
			sb.WriteString(style.Render(label) + "\n")
		}
		sb.WriteString("\n" + mutedStyle.Render("↑/↓ or 1-4: select  Enter: confirm"))

	case stepEnterAPIKey:
		p := m.selectedProvider()
		sb.WriteString(fmt.Sprintf("Enter your %s API key:\n\n", p.displayName))
		sb.WriteString("API Key: " + m.apiKeyInput.View() + "\n\n")
		sb.WriteString(mutedStyle.Render("Your key is stored locally and never shared.") + "\n\n")
		sb.WriteString(mutedStyle.Render("Enter: continue  Esc: back"))

	case stepEnterBaseURL:
		p := m.selectedProvider()
		sb.WriteString(fmt.Sprintf("Enter the %s server URL:\n\n", p.displayName))
		sb.WriteString("URL: " + m.baseURLInput.View() + "\n\n")
		sb.WriteString(mutedStyle.Render("Enter: continue  Esc: back"))

	case stepSelectModel:
		p := m.selectedProvider()
		sb.WriteString("Select a model:\n\n")
		for i, model := range p.defaultModels {
			cursor := "  "
			style := mutedStyle
			if i == m.modelCursor {
				cursor = "▶ "
				style = lipgloss.NewStyle().Foreground(t.Foreground).Bold(true)
			}
			label := fmt.Sprintf("%s%d. %s", cursor, i+1, model)
			sb.WriteString(style.Render(label) + "\n")
		}
		sb.WriteString("\n" + mutedStyle.Render("↑/↓: select  Enter: confirm  Esc: back"))

	case stepTestConnection:
		p := m.selectedProvider()
		if m.testing {
			sb.WriteString(m.spinner.View() + " Testing connection to " + p.displayName + "...\n")
		} else if m.testResult != nil && m.testResult.Success {
			sb.WriteString(successStyle.Render("Connection successful!") + "\n\n")
			sb.WriteString(successStyle.Render("✓") + " Provider: " + p.name + "\n")
			sb.WriteString(successStyle.Render("✓") + " Model: " + p.defaultModels[m.modelCursor] + "\n")
			sb.WriteString(successStyle.Render("✓") + fmt.Sprintf(" Response time: %dms", m.testResult.LatencyMs) + "\n")
			sb.WriteString("\n" + mutedStyle.Render("Press Enter to start chatting"))
		} else {
			sb.WriteString(errorStyle.Render("Connection failed") + "\n\n")
			sb.WriteString(errorStyle.Render("✗") + " " + m.testError + "\n")
			sb.WriteString("\n" + mutedStyle.Render("r: retry  b: back  Esc: back"))
		}
	}

	cardWidth := 56
	if w > 0 && cardWidth > w-6 {
		cardWidth = w - 6
	}

	card := t.OnboardingCard.Width(cardWidth).Render(strings.TrimRight(sb.String(), "\n"))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, card)
}

func (m OnboardingModel) stepCount() int {
	p := m.selectedProvider()
	count := 3 // provider + model + test
	if p.needsAPIKey {
		count++
	}
	if p.needsBaseURL {
		count++
	}
	return count
}

func (m OnboardingModel) currentStepIndex() int {
	p := m.selectedProvider()
	switch m.step {
	case stepSelectProvider:
		return 0
	case stepEnterAPIKey:
		return 1
	case stepEnterBaseURL:
		if p.needsAPIKey {
			return 2
		}
		return 1
	case stepSelectModel:
		idx := 1
		if p.needsAPIKey {
			idx++
		}
		if p.needsBaseURL {
			idx++
		}
		return idx
	case stepTestConnection:
		return m.stepCount() - 1
	}
	return 0
}
