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

// NavigateToOnboardingMsg signals the app to switch to the onboarding page.
type NavigateToOnboardingMsg struct{}

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

// deviceCodeMsg carries the result of a GitHub device code request.
type deviceCodeMsg struct {
	result *providerauth.DeviceCodeResult
	err    error
}

// modelsListMsg carries the result of a live model listing.
type modelsListMsg struct {
	models []providerauth.ModelInfo
	err    error
}

type onboardingStep int

const (
	stepSelectProvider onboardingStep = iota
	stepAnthropicAuthChoice
	stepBrowserAuth
	stepEnterAPIKey
	stepEnterBaseURL
	stepFetchModels
	stepSelectModel
	stepTestConnection
)

type authMethod string

const (
	authAPIKey  authMethod = "api_key"
	authBrowser authMethod = "browser" // Anthropic: browser OAuth, fallback to key paste
	authGHCLI   authMethod = "gh_cli"  // Copilot: gh token → device flow → key paste
	authNone    authMethod = "none"
)

type providerTypeInfo struct {
	name         string
	displayName  string
	auth         authMethod
	needsBaseURL bool
	defaultURL   string
}

var providerTypes = []providerTypeInfo{
	{
		name: "anthropic", displayName: "Anthropic (Claude)",
		auth: authBrowser,
	},
	{
		name: "copilot", displayName: "GitHub Copilot",
		auth: authGHCLI,
	},
	{
		name: "openai", displayName: "OpenAI (GPT)",
		auth: authAPIKey, needsBaseURL: true,
		defaultURL: "https://api.openai.com/v1",
	},
	{
		name: "ollama", displayName: "Ollama (Local)",
		auth: authNone, needsBaseURL: true,
		defaultURL: "http://localhost:11434",
	},
	{
		name: "gemini", displayName: "Google Gemini",
		auth: authAPIKey,
	},
}

// OnboardingModel is the multi-step provider setup wizard.
type OnboardingModel struct {
	client *client.Client
	step   onboardingStep

	// Provider selection
	cursor int

	// API key input (used for manual key entry and browser auth fallback)
	apiKeyInput textinput.Model

	// Base URL input
	baseURLInput textinput.Model

	// Browser/device auth state
	authing               bool   // browser/gh/device auth in progress
	authError             string
	authToken             string // token obtained from auth flow
	browserOpened         bool   // browser was opened for user
	authCancel            context.CancelFunc
	deviceUserCode        string // device flow: code to display to user
	deviceVerificationURI string // device flow: URL to open
	setupTokenMode        bool   // true when user chose "Paste setup-token" for Anthropic

	// Model listing
	fetchingModels bool
	fetchedModels  []providerauth.ModelInfo
	modelsError    string

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

// selectedModelID returns the ID of the currently selected model.
func (m OnboardingModel) selectedModelID() string {
	if m.modelCursor < len(m.fetchedModels) {
		return m.fetchedModels[m.modelCursor].ID
	}
	return ""
}

func (m OnboardingModel) Update(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	// Global Esc: cancel any in-progress auth and return to provider selection.
	// This fires regardless of the current step, so error screens and browser-wait
	// screens all dismiss properly. For the API key entry step, the step-specific
	// handler routes back to the Anthropic auth choice when appropriate.
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == "escape" {
		if m.step != stepSelectProvider && m.step != stepEnterAPIKey && m.step != stepAnthropicAuthChoice {
			if m.authCancel != nil {
				m.authCancel()
				m.authCancel = nil
			}
			m.authing = false
			m.authError = ""
			m.setupTokenMode = false
			m.step = stepSelectProvider
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case deviceCodeMsg:
		if msg.err != nil {
			// Device flow request failed — fall to API key paste
			m.authing = false
			m.authError = msg.err.Error()
			m.step = stepEnterAPIKey
			m.apiKeyInput.Placeholder = "ghp_..."
			return m, m.apiKeyInput.Focus()
		}
		// Show user code and start polling
		m.deviceUserCode = msg.result.UserCode
		m.deviceVerificationURI = msg.result.VerificationURI
		m.browserOpened = true
		go providerauth.OpenBrowserURL(msg.result.VerificationURI) //nolint:errcheck
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(msg.result.ExpiresIn)*time.Second)
		m.authCancel = cancel
		return m, m.pollDeviceFlow(ctx, msg.result.DeviceCode, msg.result.Interval)

	case browserAuthResultMsg:
		m.authing = false
		if msg.err != nil {
			// Auth failed — fall back to API key paste
			m.authError = msg.err.Error()
			m.step = stepEnterAPIKey
			p := m.selectedProvider()
			if p.auth == authGHCLI {
				m.apiKeyInput.Placeholder = "ghp_..."
			} else {
				m.apiKeyInput.Placeholder = "sk-ant-..."
			}
			return m, m.apiKeyInput.Focus()
		}
		m.authToken = msg.token
		return m.transitionToFetchModels()

	case modelsListMsg:
		m.fetchingModels = false
		if msg.err != nil {
			m.modelsError = msg.err.Error()
			m.step = stepSelectModel
			return m, nil
		}
		m.fetchedModels = msg.models
		m.modelCursor = 0
		m.step = stepSelectModel
		return m, nil

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
		if m.testing || m.authing || m.fetchingModels {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	switch m.step {
	case stepSelectProvider:
		return m.updateSelectProvider(msg)
	case stepAnthropicAuthChoice:
		return m.updateAnthropicAuthChoice(msg)
	case stepBrowserAuth:
		return m.updateBrowserAuth(msg)
	case stepEnterAPIKey:
		return m.updateEnterAPIKey(msg)
	case stepEnterBaseURL:
		return m.updateEnterBaseURL(msg)
	case stepFetchModels:
		return m.updateFetchModels(msg)
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
	// Reset state
	m.authToken = ""
	m.authError = ""
	m.browserOpened = false
	m.deviceUserCode = ""
	m.deviceVerificationURI = ""
	m.fetchedModels = nil
	m.modelsError = ""

	switch p.auth {
	case authBrowser:
		// Anthropic: let user choose between Claude subscription OAuth or API key
		m.step = stepAnthropicAuthChoice
		m.cursor = 0
		return m, nil

	case authGHCLI:
		// Copilot: always use explicit device flow (never auto-detect gh CLI token)
		m.step = stepBrowserAuth
		m.authing = true
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
		return m.transitionToFetchModels()
	}
	return m, nil
}

// transitionToFetchModels starts the async model listing step.
func (m OnboardingModel) transitionToFetchModels() (OnboardingModel, tea.Cmd) {
	m.step = stepFetchModels
	m.fetchingModels = true
	m.fetchedModels = nil
	m.modelsError = ""
	return m, tea.Batch(m.spinner.Tick, m.fetchModels())
}

func (m OnboardingModel) fetchModels() tea.Cmd {
	return func() tea.Msg {
		p := providerTypes[m.cursor]
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := providerauth.ListModels(ctx, p.name, m.authToken, m.baseURLInput.Value())
		return modelsListMsg{models: models, err: err}
	}
}

func (m OnboardingModel) startDeviceFlow() tea.Cmd {
	return func() tea.Msg {
		result, err := providerauth.StartGitHubDeviceFlow(context.Background(), providerauth.GithubCopilotClientID)
		return deviceCodeMsg{result: result, err: err}
	}
}

func (m OnboardingModel) pollDeviceFlow(ctx context.Context, deviceCode string, interval int) tea.Cmd {
	return func() tea.Msg {
		ch := providerauth.PollGitHubDeviceFlow(ctx, providerauth.GithubCopilotClientID, deviceCode, interval)
		result := <-ch
		return browserAuthResultMsg{token: result.Token, err: result.Err}
	}
}

func (m OnboardingModel) startAnthropicAuth(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		ch := providerauth.StartAnthropicOAuth(ctx)
		result := <-ch
		return browserAuthResultMsg{token: result.Token, err: result.Err}
	}
}

func (m OnboardingModel) updateAnthropicAuthChoice(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "escape":
			m.step = stepSelectProvider
			return m, nil
		case "j", "down":
			if m.cursor < 2 {
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
		case "enter", " ":
			switch m.cursor {
			case 0:
				// Claude subscription OAuth
				m.step = stepBrowserAuth
				m.authing = true
				m.browserOpened = true
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				m.authCancel = cancel
				return m, tea.Batch(m.spinner.Tick, m.startAnthropicAuth(ctx))
			case 1:
				// Setup-token paste (from `claude setup-token`)
				m.setupTokenMode = true
				m.step = stepEnterAPIKey
				m.apiKeyInput.Placeholder = "Paste setup-token here..."
				return m, m.apiKeyInput.Focus()
			case 2:
				// Manual API key
				m.setupTokenMode = false
				m.step = stepEnterAPIKey
				m.apiKeyInput.Placeholder = "sk-ant-..."
				return m, m.apiKeyInput.Focus()
			}
		}
	}
	return m, nil
}

func (m OnboardingModel) updateBrowserAuth(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "escape":
			if m.authCancel != nil {
				m.authCancel()
				m.authCancel = nil
			}
			m.authing = false
			m.authError = ""
			m.step = stepSelectProvider
			return m, nil
		}
	}
	return m, nil
}

func (m OnboardingModel) updateEnterAPIKey(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "escape":
			m.apiKeyInput.SetValue("")
			m.authError = ""
			m.setupTokenMode = false
			// Go back to Anthropic auth choice if this is an Anthropic provider
			p := m.selectedProvider()
			if p.auth == authBrowser {
				m.step = stepAnthropicAuthChoice
				m.cursor = 0
			} else {
				m.step = stepSelectProvider
			}
			return m, nil
		case "enter":
			if m.apiKeyInput.Value() == "" {
				return m, nil
			}
			m.authToken = m.apiKeyInput.Value()
			p := m.selectedProvider()
			if p.needsBaseURL {
				m.step = stepEnterBaseURL
				m.baseURLInput.SetValue(p.defaultURL)
				return m, m.baseURLInput.Focus()
			}
			return m.transitionToFetchModels()
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
			return m.transitionToFetchModels()
		}
	}

	var cmd tea.Cmd
	m.baseURLInput, cmd = m.baseURLInput.Update(msg)
	return m, cmd
}

func (m OnboardingModel) updateFetchModels(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if keyMsg.String() == "escape" {
			m.fetchingModels = false
			m.step = stepSelectProvider
			return m, nil
		}
	}
	return m, nil
}

func (m OnboardingModel) updateSelectModel(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	models := m.fetchedModels
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "escape":
			p := m.selectedProvider()
			if p.needsBaseURL {
				m.step = stepEnterBaseURL
				return m, m.baseURLInput.Focus()
			}
			// For browser/gh_cli auth, go back to provider selection
			m.step = stepSelectProvider
			return m, nil
		case "j", "down":
			if m.modelCursor < len(models)-1 {
				m.modelCursor++
			}
		case "k", "up":
			if m.modelCursor > 0 {
				m.modelCursor--
			}
		case "enter", " ":
			if len(models) > 0 {
				return m.startTest()
			}
		}
		// Number shortcuts (1-9)
		for i := range models {
			if i >= 9 {
				break
			}
			if keyMsg.String() == fmt.Sprintf("%d", i+1) {
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
	model := m.selectedModelID()

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
			ApiKey:    m.authToken,
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
				model := m.selectedModelID()
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

	case stepAnthropicAuthChoice:
		sb.WriteString("Sign in with Anthropic\n\n")
		choices := []string{
			"Sign in with Claude account (Pro/Team/Enterprise)",
			"Paste setup-token (from claude setup-token)",
			"Enter API key manually",
		}
		for i, label := range choices {
			cursor := "  "
			style := mutedStyle
			if i == m.cursor {
				cursor = "▶ "
				style = lipgloss.NewStyle().Foreground(t.Foreground).Bold(true)
			}
			sb.WriteString(style.Render(fmt.Sprintf("%s%d. %s", cursor, i+1, label)) + "\n")
		}
		sb.WriteString("\n" + mutedStyle.Render("↑/↓ or 1-3: select  Enter: confirm  Esc: back"))

	case stepBrowserAuth:
		p := m.selectedProvider()
		if m.authing {
			if p.auth == authGHCLI && m.deviceUserCode != "" {
				// Device flow: show user code
				sb.WriteString("Sign in with GitHub Copilot\n\n")
				codeStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
				sb.WriteString("Your code: " + codeStyle.Render(m.deviceUserCode) + "\n\n")
				sb.WriteString(m.spinner.View() + " Waiting for authorization...\n\n")
				sb.WriteString(mutedStyle.Render("Enter the code at "+m.deviceVerificationURI) + "\n")
				sb.WriteString(mutedStyle.Render("Esc: cancel"))
			} else if p.auth == authGHCLI {
				sb.WriteString(m.spinner.View() + " Checking for GitHub CLI auth...\n")
			} else {
				sb.WriteString("Sign in with " + p.displayName + "\n\n")
				sb.WriteString(m.spinner.View() + " Waiting for browser sign-in...\n\n")
				sb.WriteString(mutedStyle.Render("Complete sign-in in your browser.") + "\n")
				sb.WriteString(mutedStyle.Render("Esc: cancel and enter key manually"))
			}
		} else if m.authError != "" {
			sb.WriteString(errorStyle.Render("Authentication issue") + "\n\n")
			sb.WriteString(mutedStyle.Render(m.authError) + "\n\n")
			sb.WriteString(mutedStyle.Render("Falling back to manual key entry..."))
		}

	case stepEnterAPIKey:
		p := m.selectedProvider()
		if m.setupTokenMode {
			sb.WriteString("Paste your Claude setup-token:\n\n")
			sb.WriteString(mutedStyle.Render("Generate one by running:") + "\n")
			sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(t.Accent).Render("  claude setup-token") + "\n\n")
			sb.WriteString(mutedStyle.Render("This uses your Claude Pro/Team/Enterprise subscription.") + "\n\n")
		} else {
			switch p.auth {
			case authBrowser:
				sb.WriteString("Paste your Anthropic API key:\n\n")
				sb.WriteString(mutedStyle.Render("Get one at console.anthropic.com/settings/keys") + "\n\n")
			case authGHCLI:
				sb.WriteString("Paste your GitHub token:\n\n")
				sb.WriteString(mutedStyle.Render("Run: gh auth token") + "\n")
				sb.WriteString(mutedStyle.Render("Or create a PAT at github.com/settings/tokens") + "\n\n")
			default:
				fmt.Fprintf(&sb, "Enter your %s API key:\n\n", p.displayName)
			}
		}
		sb.WriteString("Key: " + m.apiKeyInput.View() + "\n\n")
		sb.WriteString(mutedStyle.Render("Your key is stored locally and never shared.") + "\n\n")
		sb.WriteString(mutedStyle.Render("Enter: continue  Esc: back"))

	case stepEnterBaseURL:
		p := m.selectedProvider()
		fmt.Fprintf(&sb, "Enter the %s server URL:\n\n", p.displayName)
		sb.WriteString("URL: " + m.baseURLInput.View() + "\n\n")
		sb.WriteString(mutedStyle.Render("Enter: continue  Esc: back"))

	case stepFetchModels:
		p := m.selectedProvider()
		sb.WriteString(successStyle.Render("Authenticated!") + "\n\n")
		sb.WriteString(m.spinner.View() + " Loading available models from " + p.displayName + "...\n\n")
		sb.WriteString(mutedStyle.Render("Esc: cancel"))

	case stepSelectModel:
		models := m.fetchedModels
		if m.modelsError != "" || len(models) == 0 {
			sb.WriteString(errorStyle.Render("Could not fetch models") + "\n\n")
			if m.modelsError != "" {
				sb.WriteString(mutedStyle.Render(m.modelsError) + "\n\n")
			}
			sb.WriteString(mutedStyle.Render("Esc: back"))
		} else {
			sb.WriteString("Select your default model:\n\n")
			// Show up to 15 models with scrolling
			maxVisible := 15
			start := 0
			if m.modelCursor >= maxVisible {
				start = m.modelCursor - maxVisible + 1
			}
			end := start + maxVisible
			if end > len(models) {
				end = len(models)
			}
			if start > 0 {
				sb.WriteString(mutedStyle.Render("  ... more above") + "\n")
			}
			for i := start; i < end; i++ {
				cursor := "  "
				style := mutedStyle
				if i == m.modelCursor {
					cursor = "▶ "
					style = lipgloss.NewStyle().Foreground(t.Foreground).Bold(true)
				}
				label := models[i].Name
				if models[i].Name != models[i].ID {
					label = fmt.Sprintf("%s (%s)", models[i].Name, models[i].ID)
				}
				sb.WriteString(style.Render(cursor+label) + "\n")
			}
			if end < len(models) {
				sb.WriteString(mutedStyle.Render("  ... more below") + "\n")
			}
			sb.WriteString("\n" + mutedStyle.Render("Other models can be used later."))
			sb.WriteString("\n" + mutedStyle.Render("↑/↓: select  Enter: confirm  Esc: back"))
		}

	case stepTestConnection:
		p := m.selectedProvider()
		if m.testing {
			sb.WriteString(m.spinner.View() + " Testing connection to " + p.displayName + "...\n")
		} else if m.testResult != nil && m.testResult.Success {
			sb.WriteString(successStyle.Render("Connection successful!") + "\n\n")
			sb.WriteString(successStyle.Render("✓") + " Provider: " + p.name + "\n")
			sb.WriteString(successStyle.Render("✓") + " Default model: " + m.selectedModelID() + "\n")
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
	if p.auth != authNone {
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
	case stepAnthropicAuthChoice:
		return 1
	case stepBrowserAuth, stepEnterAPIKey:
		return 1
	case stepEnterBaseURL:
		idx := 1
		if p.auth != authNone {
			idx++
		}
		return idx
	case stepFetchModels, stepSelectModel:
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
