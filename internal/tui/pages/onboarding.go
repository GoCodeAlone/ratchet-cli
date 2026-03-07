package pages

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	providerauth "github.com/GoCodeAlone/ratchet-cli/internal/provider"
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

// browserAuthResultMsg carries the result of a browser-based auth flow.
type browserAuthResultMsg struct {
	token string
	err   error
}

// deviceCodeMsg carries the device code for display in the TUI.
type deviceCodeMsg struct {
	result *providerauth.DeviceCodeResult
	err    error
}

type onboardingStep int

const (
	stepSelectProvider onboardingStep = iota
	stepBrowserAuth
	stepEnterAPIKey
	stepEnterBaseURL
	stepSelectModel
	stepTestConnection
)

type authMethod string

const (
	authAPIKey     authMethod = "api_key"
	authBrowser    authMethod = "browser"
	authDeviceFlow authMethod = "device_flow"
	authNone       authMethod = "none"
)

type providerTypeInfo struct {
	name          string
	displayName   string
	auth          authMethod
	needsBaseURL  bool
	defaultURL    string
	defaultModels []string
}

var providerTypes = []providerTypeInfo{
	{
		name: "anthropic", displayName: "Anthropic (Claude)",
		auth: authBrowser,
		defaultModels: []string{"claude-sonnet-4-20250514", "claude-opus-4-20250514", "claude-haiku-4-20250514"},
	},
	{
		name: "copilot", displayName: "GitHub Copilot",
		auth: authDeviceFlow,
		defaultModels: []string{"gpt-4o", "claude-3.5-sonnet", "o3-mini"},
	},
	{
		name: "openai", displayName: "OpenAI (GPT)",
		auth: authAPIKey, needsBaseURL: true,
		defaultURL:    "https://api.openai.com/v1",
		defaultModels: []string{"gpt-4o", "gpt-4o-mini", "o3-mini"},
	},
	{
		name: "ollama", displayName: "Ollama (Local)",
		auth: authNone, needsBaseURL: true,
		defaultURL:    "http://localhost:11434",
		defaultModels: []string{"llama3.3", "codellama", "mistral"},
	},
	{
		name: "gemini", displayName: "Google Gemini",
		auth: authAPIKey,
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

	// Browser/device flow auth
	authToken   string // token obtained from browser/device auth
	authError   string
	authing     bool   // browser/device auth in progress
	deviceCode  *providerauth.DeviceCodeResult
	authCancel  context.CancelFunc

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
		client:       c,
		step:         stepSelectProvider,
		apiKeyInput:  apiKey,
		baseURLInput: baseURL,
		spinner:      sp,
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

	case browserAuthResultMsg:
		m.authing = false
		if msg.err != nil {
			m.authError = msg.err.Error()
			return m, nil
		}
		m.authToken = msg.token
		m.authError = ""
		// Auth succeeded, advance to model selection
		m.step = stepSelectModel
		m.modelCursor = 0
		return m, nil

	case deviceCodeMsg:
		if msg.err != nil {
			m.authing = false
			m.authError = msg.err.Error()
			return m, nil
		}
		m.deviceCode = msg.result
		// Open browser to verification URI
		go providerauth.OpenBrowserURL(msg.result.VerificationURI) //nolint:errcheck
		// Start polling for token
		ctx, cancel := context.WithCancel(context.Background())
		m.authCancel = cancel
		return m, m.pollDeviceFlow(ctx, msg.result.DeviceCode, msg.result.Interval)

	case providerAddedMsg:
		if msg.err != nil {
			m.testing = false
			m.testError = msg.err.Error()
			return m, nil
		}
		m.added = true
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
		if m.testing || m.authing {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.step {
	case stepSelectProvider:
		return m.updateSelectProvider(msg)
	case stepBrowserAuth:
		return m.updateBrowserAuth(msg)
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
		case "enter", " ":
			return m.advanceFromProvider()
		}
		// Number shortcuts
		for i := range providerTypes {
			if keyMsg.String() == fmt.Sprintf("%d", i+1) && i < len(providerTypes) {
				m.cursor = i
			}
		}
	}
	return m, nil
}

func (m OnboardingModel) advanceFromProvider() (OnboardingModel, tea.Cmd) {
	p := m.selectedProvider()
	switch p.auth {
	case authBrowser:
		m.step = stepBrowserAuth
		m.authing = true
		m.authError = ""
		m.authToken = ""
		return m, tea.Batch(m.spinner.Tick, m.startBrowserAuth())
	case authDeviceFlow:
		m.step = stepBrowserAuth
		m.authing = true
		m.authError = ""
		m.authToken = ""
		m.deviceCode = nil
		return m, tea.Batch(m.spinner.Tick, m.startDeviceFlow())
	case authAPIKey:
		m.step = stepEnterAPIKey
		return m, m.apiKeyInput.Focus()
	case authNone:
		if p.needsBaseURL {
			m.step = stepEnterBaseURL
			m.baseURLInput.SetValue(p.defaultURL)
			return m, m.baseURLInput.Focus()
		}
		m.step = stepSelectModel
		m.modelCursor = 0
		return m, nil
	}
	return m, nil
}

func (m OnboardingModel) startBrowserAuth() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		_ = cancel // cancel stored separately if needed
		ch := providerauth.StartAnthropicOAuth(ctx)
		result := <-ch
		return browserAuthResultMsg{token: result.Token, err: result.Err}
	}
}

func (m OnboardingModel) startDeviceFlow() tea.Cmd {
	return func() tea.Msg {
		result, err := providerauth.StartGitHubDeviceFlow(context.Background())
		return deviceCodeMsg{result: result, err: err}
	}
}

func (m OnboardingModel) pollDeviceFlow(ctx context.Context, deviceCode string, interval int) tea.Cmd {
	return func() tea.Msg {
		ch := providerauth.PollGitHubDeviceFlow(ctx, deviceCode, interval)
		result := <-ch
		return browserAuthResultMsg{token: result.Token, err: result.Err}
	}
}

func (m OnboardingModel) updateBrowserAuth(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "escape":
			// Cancel auth and go back
			if m.authCancel != nil {
				m.authCancel()
				m.authCancel = nil
			}
			m.authing = false
			m.authError = ""
			m.deviceCode = nil
			m.step = stepSelectProvider
			return m, nil
		case "r":
			if !m.authing && m.authError != "" {
				// Retry
				return m.advanceFromProvider()
			}
		}
	}
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
			if p.auth == authAPIKey {
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
			// Go back — skip browser auth steps (can't go back to those)
			if p.auth == authBrowser || p.auth == authDeviceFlow {
				m.step = stepSelectProvider
				return m, nil
			}
			if p.needsBaseURL {
				m.step = stepEnterBaseURL
				return m, m.baseURLInput.Focus()
			}
			if p.auth == authAPIKey {
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

func (m OnboardingModel) resolveAPIKey() string {
	// Browser/device flow auth stores token in authToken
	if m.authToken != "" {
		return m.authToken
	}
	return m.apiKeyInput.Value()
}

func (m OnboardingModel) addProvider(p providerTypeInfo, model string) tea.Cmd {
	return func() tea.Msg {
		req := &pb.AddProviderReq{
			Alias:     p.name,
			Type:      p.name,
			Model:     model,
			ApiKey:    m.resolveAPIKey(),
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
		if m.testing {
			return m, nil
		}

		if m.testResult != nil && m.testResult.Success {
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

		switch keyMsg.String() {
		case "r":
			if m.added {
				m.testing = true
				m.testError = ""
				p := m.selectedProvider()
				return m, tea.Batch(m.spinner.Tick, m.testProvider(p.name))
			}
			return m.startTest()
		case "b", "escape":
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
		sb.WriteString("\n" + mutedStyle.Render(fmt.Sprintf("↑/↓ or 1-%d: select  Enter: confirm", len(providerTypes))))

	case stepBrowserAuth:
		p := m.selectedProvider()
		if m.authing {
			if p.auth == authDeviceFlow && m.deviceCode != nil {
				// Device flow: show user code
				codeStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
				sb.WriteString("Sign in with GitHub\n\n")
				sb.WriteString("Enter this code at " + mutedStyle.Render(m.deviceCode.VerificationURI) + ":\n\n")
				sb.WriteString("  " + codeStyle.Render(m.deviceCode.UserCode) + "\n\n")
				sb.WriteString(m.spinner.View() + " Waiting for authorization...\n\n")
				sb.WriteString(mutedStyle.Render("A browser window should have opened.\nEsc: cancel"))
			} else if p.auth == authDeviceFlow {
				sb.WriteString(m.spinner.View() + " Requesting device code...\n")
			} else {
				// Browser OAuth: waiting for callback
				sb.WriteString("Sign in with " + p.displayName + "\n\n")
				sb.WriteString(m.spinner.View() + " Opening browser...\n\n")
				sb.WriteString(mutedStyle.Render("Complete sign-in in your browser.\nEsc: cancel"))
			}
		} else if m.authError != "" {
			sb.WriteString(errorStyle.Render("Authentication failed") + "\n\n")
			sb.WriteString(errorStyle.Render("✗") + " " + m.authError + "\n\n")
			sb.WriteString(mutedStyle.Render("r: retry  Esc: back"))
		} else {
			sb.WriteString(successStyle.Render("✓ Authenticated!") + "\n")
		}

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
	if p.auth == authBrowser || p.auth == authDeviceFlow {
		count++ // browser auth step
	}
	if p.auth == authAPIKey {
		count++ // api key step
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
	case stepBrowserAuth:
		return 1
	case stepEnterAPIKey:
		return 1
	case stepEnterBaseURL:
		idx := 1
		if p.auth == authAPIKey || p.auth == authBrowser || p.auth == authDeviceFlow {
			idx++
		}
		return idx
	case stepSelectModel:
		idx := 1
		if p.auth != authNone {
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
