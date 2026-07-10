package pages

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/GoCodeAlone/ratchet-cli/internal/client"
	pb "github.com/GoCodeAlone/ratchet-cli/internal/proto"
	providerauth "github.com/GoCodeAlone/ratchet-cli/internal/provider"
	"github.com/GoCodeAlone/ratchet-cli/internal/tui/theme"
	wfprovider "github.com/GoCodeAlone/workflow-plugin-agent/provider"
	"github.com/GoCodeAlone/workflow/secrets"
)

// OnboardingDoneMsg signals provider setup is complete.
type OnboardingDoneMsg struct {
	Provider *pb.Provider
}

// OnboardingCancelledMsg signals provider setup should return to its caller.
type OnboardingCancelledMsg struct{}

// NavigateToOnboardingMsg signals the app to switch to the onboarding page.
type NavigateToOnboardingMsg struct{}

type providerAddedMsg struct {
	provider *pb.Provider
	err      error
	flowID   uint64
}

type providerTestedMsg struct {
	result *pb.TestProviderResult
	err    error
	flowID uint64
}

type providerRemovedMsg struct {
	err    error
	flowID uint64
}

// browserAuthResultMsg carries the result of a browser-based auth flow.
type browserAuthResultMsg struct {
	token  string
	err    error
	flowID uint64
}

// deviceCodeMsg carries the result of a GitHub device code request.
type deviceCodeMsg struct {
	result *providerauth.DeviceCodeResult
	err    error
	flowID uint64
}

// modelsListMsg carries the result of a live model listing.
type modelsListMsg struct {
	models []providerauth.ModelInfo
	err    error
	flowID uint64
}

// pullProgressMsg carries a model pull progress update (0–100).
type pullProgressMsg struct {
	pct    float64
	flowID uint64
}

// pullDoneMsg signals a model pull completed or failed.
type pullDoneMsg struct {
	err    error
	flowID uint64
}

// ollamaSetupMsg carries the result of the async Ollama health/start check.
type ollamaSetupMsg struct {
	status string // "ready", "not_installed", "failed"
	err    error
	flowID uint64
}

type cliCheckMsg struct {
	path       string
	workingDir string
	err        error
	flowID     uint64
}

type onboardingStep int

const (
	stepSelectProvider onboardingStep = iota
	stepAnthropicAuthChoice
	stepBrowserAuth
	stepEnterAPIKey
	stepOllamaChoice // "I already have a local server" vs "Set up Ollama for me"
	stepOllamaSetup  // checking/starting Ollama before model pull
	stepEnterSettings
	stepEnterBaseURL
	stepFetchModels
	stepPullModel
	stepSelectModel
	stepCLISetup
	stepReview
	stepTestConnection
)

// anthropicAuthChoice identifies which Anthropic sign-in option was selected.
type anthropicAuthChoice int

const (
	anthropicChoiceAPIKey       anthropicAuthChoice = 0 // Enter API key directly
	anthropicChoiceConsoleOAuth anthropicAuthChoice = 1 // Console OAuth → creates API key
	anthropicChoiceMaxOAuth     anthropicAuthChoice = 2 // Max/Pro subscription OAuth
)

type onboardingDeps struct {
	listModels        func(context.Context, string, string, string, map[string]string) ([]providerauth.ModelInfo, error)
	addProvider       func(context.Context, *pb.AddProviderReq) (*pb.Provider, error)
	removeProvider    func(context.Context, string) error
	testProvider      func(context.Context, string) (*pb.TestProviderResult, error)
	startGitHubDevice func(context.Context) (*providerauth.DeviceCodeResult, error)
	pollGitHubDevice  func(context.Context, string, int) (string, error)
	startOpenAIDevice func(context.Context) (*providerauth.DeviceCodeResult, error)
	pollOpenAIDevice  func(context.Context, string, string, int) (string, error)
	startAnthropic    func(context.Context) (string, error)
	startAnthropicMax func(context.Context) (string, error)
	lookPath          func(string) (string, error)
	checkCLI          func(context.Context, string, string) error
	workingDir        func() (string, error)
	setupOllama       func(context.Context, string) (string, error)
}

// OnboardingModel is the multi-step provider setup wizard.
type OnboardingModel struct {
	deps   onboardingDeps
	step   onboardingStep
	flowID uint64

	// Provider selection
	// cursor is used for navigation within the current step (provider list,
	// auth choice list, model list, etc.). providerIdx is set once the user
	// confirms a provider and never changes until a new provider is selected.
	cursor      int
	providerIdx int
	providers   []providerauth.SetupEntry
	filterInput textinput.Model
	filtering   bool

	// API key input (used for manual key entry and browser auth fallback)
	apiKeyInput textinput.Model

	// Base URL input
	baseURLInput textinput.Model

	// Provider-specific non-secret settings.
	settingInput  textinput.Model
	settingIdx    int
	settingChoice int
	settings      map[string]string
	settingsError string
	settingsNext  onboardingStep

	// Browser/device auth state
	authing               bool // browser/gh/device auth in progress
	authError             string
	authToken             string // token obtained from auth flow
	authCancel            context.CancelFunc
	deviceUserCode        string              // device flow: code to display to user
	deviceVerificationURI string              // device flow: URL to open
	anthropicChoice       anthropicAuthChoice // which Anthropic auth option was selected

	// Model listing
	fetchingModels bool
	fetchedModels  []providerauth.ModelInfo
	modelsError    string

	// Ollama choice and setup (stepOllamaChoice, stepOllamaSetup)
	ollamaChoiceCursor int    // 0 = "I have a server", 1 = "Set up Ollama"
	ollamaSetupStatus  string // "checking", "not_installed", "starting", "ready", "failed"
	ollamaSetupError   string
	ollamaSetupCancel  context.CancelFunc

	// Model pull (stepPullModel)
	pullingModel       bool
	pullProgress       float64
	pullModelName      string
	pullCursor         int
	recommendedModels  []string
	pullError          string
	pullProgressCh     chan float64
	pullDoneCh         chan error
	pullCancel         context.CancelFunc
	pullCustomInput    textinput.Model
	pullEnteringCustom bool

	// Model selection
	modelCursor         int
	selectedModel       string
	manualModelInput    textinput.Model
	enteringManualModel bool

	// External CLI setup.
	cliCommandPath string
	cliWorkingDir  string
	cliError       string

	// Connection test
	spinner    spinner.Model
	testing    bool
	removing   bool
	testResult *pb.TestProviderResult
	testError  string
	added      bool // provider has been added to daemon

	width  int
	height int
}

func NewOnboarding(c *client.Client, t theme.Theme) OnboardingModel {
	return newOnboarding(c, t, defaultOnboardingDeps(c))
}

func defaultOnboardingDeps(c *client.Client) onboardingDeps {
	return onboardingDeps{
		listModels: providerauth.ListModelsWithSettings,
		addProvider: func(ctx context.Context, req *pb.AddProviderReq) (*pb.Provider, error) {
			return c.AddProvider(ctx, req)
		},
		removeProvider: func(ctx context.Context, alias string) error {
			return c.RemoveProvider(ctx, alias)
		},
		testProvider: func(ctx context.Context, alias string) (*pb.TestProviderResult, error) {
			return c.TestProvider(ctx, alias)
		},
		startGitHubDevice: func(ctx context.Context) (*providerauth.DeviceCodeResult, error) {
			return providerauth.StartGitHubDeviceFlow(ctx, providerauth.GithubCopilotClientID)
		},
		pollGitHubDevice: func(ctx context.Context, deviceCode string, interval int) (string, error) {
			result := <-providerauth.PollGitHubDeviceFlow(ctx, providerauth.GithubCopilotClientID, deviceCode, interval)
			return result.Token, result.Err
		},
		startOpenAIDevice: providerauth.StartOpenAIChatGPTDeviceFlow,
		pollOpenAIDevice: func(ctx context.Context, deviceCode, userCode string, interval int) (string, error) {
			result := <-providerauth.PollOpenAIChatGPTDeviceFlow(ctx, deviceCode, userCode, interval)
			return result.Token, result.Err
		},
		startAnthropic: func(ctx context.Context) (string, error) {
			result := <-providerauth.StartAnthropicOAuth(ctx)
			return result.Token, result.Err
		},
		startAnthropicMax: func(ctx context.Context) (string, error) {
			result := <-providerauth.StartAnthropicMaxOAuth(ctx)
			return result.Token, result.Err
		},
		lookPath: exec.LookPath,
		checkCLI: func(ctx context.Context, providerType, path string) error {
			_, err := exec.CommandContext(ctx, path, providerauth.CLIHealthCheckArgs(providerType)...).Output()
			return err
		},
		workingDir:  os.Getwd,
		setupOllama: setupOllama,
	}
}

func newOnboarding(_ *client.Client, t theme.Theme, deps onboardingDeps) OnboardingModel {
	apiKey := textinput.New()
	apiKey.Placeholder = "sk-..."
	apiKey.EchoMode = textinput.EchoPassword
	apiKey.EchoCharacter = '*'
	apiKey.Prompt = ""

	baseURL := textinput.New()
	baseURL.Placeholder = "http://localhost:11434"
	baseURL.Prompt = ""
	filter := textinput.New()
	filter.Placeholder = "provider name or type"
	filter.Prompt = "/ "
	setting := textinput.New()
	setting.Prompt = ""
	manualModel := textinput.New()
	manualModel.Placeholder = "provider model ID"
	manualModel.Prompt = ""

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(t.Primary)

	return OnboardingModel{
		deps:             deps,
		step:             stepSelectProvider,
		providers:        providerauth.Catalog(),
		filterInput:      filter,
		apiKeyInput:      apiKey,
		baseURLInput:     baseURL,
		settingInput:     setting,
		manualModelInput: manualModel,
		settings:         make(map[string]string),
		spinner:          sp,
	}
}

func (m OnboardingModel) Init() tea.Cmd {
	return nil
}

func (m OnboardingModel) selectedProvider() providerauth.SetupEntry {
	if len(m.providers) == 0 {
		return providerauth.SetupEntry{}
	}
	if m.providerIdx < 0 || m.providerIdx >= len(m.providers) {
		return m.providers[0]
	}
	return m.providers[m.providerIdx]
}

// selectedModelID returns the ID of the currently selected model.
func (m OnboardingModel) selectedModelID() string {
	if m.selectedModel != "" {
		return m.selectedModel
	}
	if m.modelCursor < len(m.fetchedModels) {
		return m.fetchedModels[m.modelCursor].ID
	}
	return ""
}

func (m OnboardingModel) Update(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case deviceCodeMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
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
		go providerauth.OpenBrowserURL(msg.result.VerificationURI) //nolint:errcheck
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(msg.result.ExpiresIn)*time.Second)
		m.authCancel = cancel
		return m, m.pollDeviceFlow(ctx, msg.result.DeviceCode, msg.result.UserCode, msg.result.Interval)

	case browserAuthResultMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
		m.authing = false
		if m.authCancel != nil {
			m.authCancel()
			m.authCancel = nil
		}
		if msg.err != nil {
			m.authError = msg.err.Error()
			p := m.selectedProvider()
			if p.Auth == providerauth.AuthOpenAIChatGPT {
				m.step = stepBrowserAuth
				return m, nil
			}
			m.step = stepEnterAPIKey
			if p.Auth == providerauth.AuthGitHubDevice {
				m.apiKeyInput.Placeholder = "ghp_..."
			} else {
				m.apiKeyInput.Placeholder = "sk-ant-..."
			}
			return m, m.apiKeyInput.Focus()
		}
		m.authToken = msg.token
		return m.advanceAfterCredential()

	case modelsListMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
		m.fetchingModels = false
		if msg.err != nil {
			m.modelsError = redactProviderError(msg.err, m.authToken)
		}
		m.fetchedModels = msg.models
		m.modelCursor = 0
		// When Ollama has no models installed, offer to pull one.
		// Only trigger for actual Ollama servers (default base URL or localhost:11434),
		// not for other OpenAI-compatible servers (LM Studio, vLLM) where the
		// Ollama pull API won't work.
		isOllamaServer := m.selectedProvider().Type == "ollama" &&
			(m.baseURLInput.Value() == "" || strings.Contains(m.baseURLInput.Value(), "localhost:11434") || strings.Contains(m.baseURLInput.Value(), "127.0.0.1:11434"))
		if len(msg.models) == 0 && msg.err == nil && isOllamaServer {
			m.step = stepPullModel
			m.pullCursor = 0
			m.pullError = ""
			m.pullingModel = false
			m.recommendedModels = recommendedOllamaModels()
			return m, nil
		}
		m.step = stepSelectModel
		if len(msg.models) == 0 && m.selectedProvider().AllowManualModel {
			m.enteringManualModel = true
			m.manualModelInput.SetValue("")
			return m, m.manualModelInput.Focus()
		}
		return m, nil

	case cliCheckMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
		if msg.err != nil {
			m.cliError = msg.err.Error()
			return m, nil
		}
		m.cliCommandPath = msg.path
		m.cliWorkingDir = msg.workingDir
		m.cliError = ""
		m.step = stepReview
		return m, nil

	case pullProgressMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
		m.pullProgress = msg.pct
		return m, m.readPullProgress()

	case pullDoneMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
		m.pullingModel = false
		if m.pullCancel != nil {
			m.pullCancel()
			m.pullCancel = nil
		}
		if msg.err != nil && msg.err != context.Canceled {
			m.pullError = msg.err.Error()
			return m, nil
		}
		if msg.err == context.Canceled {
			// User cancelled — reset pull state and return to provider selection.
			m.pullError = ""
			m.pullModelName = ""
			m.pullProgress = 0
			m.pullingModel = false
			m.flowID++
			m.step = stepSelectProvider
			m.cursor = m.providerIdx
			return m, nil
		}
		// Pull succeeded — re-fetch models and proceed to selection.
		return m.transitionToFetchModels()

	case providerAddedMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
		if msg.err != nil {
			m.testing = false
			m.testError = msg.err.Error()
			return m, nil
		}
		m.added = true
		return m, m.testProvider(msg.provider.Alias)

	case providerTestedMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
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

	case providerRemovedMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
		m.removing = false
		if msg.err != nil {
			m.testError = "remove failed: " + msg.err.Error()
			return m, nil
		}
		m.added = false
		m.step = stepReview
		m.testResult = nil
		m.testError = ""
		return m, nil

	case spinner.TickMsg:
		if m.testing || m.removing || m.authing || m.fetchingModels || m.pullingModel || (m.step == stepCLISetup && m.cliCommandPath == "" && m.cliError == "") {
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
	case stepOllamaChoice:
		return m.updateOllamaChoice(msg)
	case stepOllamaSetup:
		return m.updateOllamaSetup(msg)
	case stepEnterSettings:
		return m.updateEnterSettings(msg)
	case stepEnterBaseURL:
		return m.updateEnterBaseURL(msg)
	case stepFetchModels:
		return m.updateFetchModels(msg)
	case stepPullModel:
		return m.updatePullModel(msg)
	case stepSelectModel:
		return m.updateSelectModel(msg)
	case stepCLISetup:
		return m.updateCLISetup(msg)
	case stepReview:
		return m.updateReview(msg)
	case stepTestConnection:
		return m.updateTestConnection(msg)
	}

	return m, nil
}

func (m OnboardingModel) updateSelectProvider(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if m.filtering {
			switch keyMsg.String() {
			case "esc":
				m.filtering = false
				m.filterInput.SetValue("")
				m.cursor = 0
				return m, nil
			case "enter":
				m.filtering = false
				m.cursor = 0
				return m, nil
			}
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.cursor = 0
			return m, cmd
		}
		indices := m.filteredProviderIndices()
		switch keyMsg.String() {
		case "esc":
			if m.filterInput.Value() != "" {
				m.filterInput.SetValue("")
				m.cursor = 0
				return m, nil
			}
			return m, func() tea.Msg { return OnboardingCancelledMsg{} }
		case "/":
			m.filtering = true
			return m, m.filterInput.Focus()
		case "j", "down":
			if m.cursor < len(indices)-1 {
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
		for i := 0; i < len(indices) && i < 9; i++ {
			if keyMsg.String() == fmt.Sprintf("%d", i+1) {
				m.cursor = i
			}
		}
	}
	return m, nil
}

func (m OnboardingModel) filteredProviderIndices() []int {
	query := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	indices := make([]int, 0, len(m.providers))
	for i, entry := range m.providers {
		if query == "" || providerEntryMatches(entry, query) {
			indices = append(indices, i)
		}
	}
	return indices
}

func providerEntryMatches(entry providerauth.SetupEntry, query string) bool {
	values := []string{entry.Type, entry.DisplayName, string(entry.Category)}
	values = append(values, entry.Aliases...)
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), query) {
			return true
		}
	}
	return false
}

func (m OnboardingModel) advanceFromProvider() (OnboardingModel, tea.Cmd) {
	indices := m.filteredProviderIndices()
	if len(indices) == 0 || m.cursor < 0 || m.cursor >= len(indices) {
		return m, nil
	}
	m.providerIdx = indices[m.cursor]
	m.flowID++
	p := m.selectedProvider()
	m.filtering = false
	m.filterInput.SetValue("")
	m.cursor = m.providerIdx
	// Reset state
	m.authToken = ""
	m.authError = ""
	m.deviceUserCode = ""
	m.deviceVerificationURI = ""
	m.fetchedModels = nil
	m.modelsError = ""
	m.settings = make(map[string]string)
	m.settingsError = ""
	m.selectedModel = ""
	m.enteringManualModel = false
	m.cliCommandPath = ""
	m.cliWorkingDir = ""
	m.cliError = ""
	m.apiKeyInput.SetValue("")
	m.baseURLInput.SetValue("")

	if p.Setup == providerauth.SetupCLINative {
		m.step = stepCLISetup
		return m, tea.Batch(m.spinner.Tick, m.checkCLIProvider())
	}

	switch p.Auth {
	case providerauth.AuthAnthropic:
		// Anthropic: let user choose between Claude subscription OAuth or API key
		m.step = stepAnthropicAuthChoice
		m.cursor = 0
		return m, nil

	case providerauth.AuthGitHubDevice, providerauth.AuthOpenAIChatGPT:
		m.step = stepBrowserAuth
		m.authing = true
		return m, tea.Batch(m.spinner.Tick, m.startDeviceFlow())

	case providerauth.AuthAPIKey:
		m.step = stepEnterAPIKey
		m.apiKeyInput.Placeholder = p.CredentialLabel
		return m, m.apiKeyInput.Focus()

	case providerauth.AuthNone:
		if p.Type == "ollama" {
			m.step = stepOllamaChoice
			m.ollamaChoiceCursor = 0
			return m, nil
		}
		return m.advanceAfterCredential()
	}
	return m, nil
}

func (m OnboardingModel) advanceAfterCredential() (OnboardingModel, tea.Cmd) {
	p := m.selectedProvider()
	if len(p.Settings) > 0 {
		next := stepFetchModels
		if p.PromptBaseURL {
			next = stepEnterBaseURL
		}
		return m.beginSettings(next)
	}
	if p.PromptBaseURL {
		m.step = stepEnterBaseURL
		m.baseURLInput.SetValue(p.DefaultBaseURL)
		return m, m.baseURLInput.Focus()
	}
	return m.transitionToModelStrategy()
}

func (m OnboardingModel) beginSettings(next onboardingStep) (OnboardingModel, tea.Cmd) {
	m.step = stepEnterSettings
	m.settingsNext = next
	m.settingIdx = 0
	m.settingChoice = 0
	m.settingsError = ""
	m.prepareSettingInput()
	return m, m.settingInput.Focus()
}

func (m *OnboardingModel) prepareSettingInput() {
	p := m.selectedProvider()
	if m.settingIdx < 0 || m.settingIdx >= len(p.Settings) {
		return
	}
	field := p.Settings[m.settingIdx]
	m.settingInput.Placeholder = field.Placeholder
	value := m.settings[field.Key]
	if value == "" {
		value = field.Default
	}
	m.settingInput.SetValue(value)
	m.settingChoice = 0
	if len(field.Choices) > 0 {
		for i, choice := range field.Choices {
			if choice == value {
				m.settingChoice = i
				break
			}
		}
	}
}

func (m OnboardingModel) updateEnterSettings(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	p := m.selectedProvider()
	if m.settingIdx < 0 || m.settingIdx >= len(p.Settings) {
		return m.transitionToModelStrategy()
	}
	field := p.Settings[m.settingIdx]
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.settingsError = ""
			if m.settingIdx > 0 {
				m.settingIdx--
				m.prepareSettingInput()
				return m, m.settingInput.Focus()
			}
			if p.Type == "ollama" {
				m.step = stepOllamaChoice
				return m, nil
			}
			if p.Auth == providerauth.AuthAnthropic {
				m.step = stepAnthropicAuthChoice
				m.cursor = 0
				return m, nil
			}
			if p.Auth == providerauth.AuthAPIKey {
				m.step = stepEnterAPIKey
				return m, m.apiKeyInput.Focus()
			}
			m.step = stepSelectProvider
			m.cursor = 0
			return m, nil
		case "j", "down":
			if len(field.Choices) > 0 && m.settingChoice < len(field.Choices)-1 {
				m.settingChoice++
			}
		case "k", "up":
			if len(field.Choices) > 0 && m.settingChoice > 0 {
				m.settingChoice--
			}
		case "enter":
			value := strings.TrimSpace(m.settingInput.Value())
			if len(field.Choices) > 0 {
				value = field.Choices[m.settingChoice]
			}
			if field.Required && value == "" {
				m.settingsError = field.Label + " is required"
				return m, nil
			}
			m.settingsError = ""
			if value == "" {
				delete(m.settings, field.Key)
			} else {
				m.settings[field.Key] = value
			}
			m.settingIdx++
			if m.settingIdx < len(p.Settings) {
				m.prepareSettingInput()
				return m, m.settingInput.Focus()
			}
			switch m.settingsNext {
			case stepEnterBaseURL:
				m.step = stepEnterBaseURL
				m.baseURLInput.SetValue(p.DefaultBaseURL)
				return m, m.baseURLInput.Focus()
			case stepOllamaSetup:
				return m.beginOllamaSetup()
			default:
				return m.transitionToModelStrategy()
			}
		}
		if len(field.Choices) > 0 {
			for i := 0; i < len(field.Choices) && i < 9; i++ {
				if keyMsg.String() == fmt.Sprintf("%d", i+1) {
					m.settingChoice = i
				}
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.settingInput, cmd = m.settingInput.Update(msg)
	return m, cmd
}

func (m OnboardingModel) transitionToModelStrategy() (OnboardingModel, tea.Cmd) {
	p := m.selectedProvider()
	switch p.Model {
	case providerauth.ModelExternal:
		m.step = stepReview
		return m, nil
	case providerauth.ModelManual:
		m.step = stepSelectModel
		m.enteringManualModel = true
		m.manualModelInput.SetValue("")
		return m, m.manualModelInput.Focus()
	case providerauth.ModelDynamic, providerauth.ModelOllama:
		return m.transitionToFetchModels()
	default:
		m.modelsError = fmt.Sprintf("unsupported model strategy %q", p.Model)
		m.step = stepSelectModel
		return m, nil
	}
}

func (m OnboardingModel) checkCLIProvider() tea.Cmd {
	return func() tea.Msg {
		entry := m.selectedProvider()
		path, err := m.deps.lookPath(entry.CLICommand)
		if err != nil {
			return cliCheckMsg{err: err, flowID: m.flowID}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := m.deps.checkCLI(ctx, entry.Type, path); err != nil {
			return cliCheckMsg{err: fmt.Errorf("health check %s: %w", entry.DisplayName, err), flowID: m.flowID}
		}
		workingDir, err := m.deps.workingDir()
		return cliCheckMsg{path: path, workingDir: workingDir, err: err, flowID: m.flowID}
	}
}

func (m OnboardingModel) updateCLISetup(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == "esc" {
		m.flowID++
		m.step = stepSelectProvider
		m.cursor = 0
		m.cliError = ""
		return m, nil
	}
	return m, nil
}

func (m OnboardingModel) updateReview(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "enter", " ":
			return m.startTest()
		case "b", "esc":
			if m.selectedProvider().Model == providerauth.ModelExternal {
				m.step = stepSelectProvider
				m.cursor = 0
				return m, nil
			}
			m.step = stepSelectModel
			return m, nil
		}
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
		p := m.selectedProvider()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		models, err := m.deps.listModels(ctx, p.Type, m.authToken, m.baseURLInput.Value(), m.settings)
		return modelsListMsg{models: models, err: err, flowID: m.flowID}
	}
}

func (m OnboardingModel) startDeviceFlow() tea.Cmd {
	return func() tea.Msg {
		var result *providerauth.DeviceCodeResult
		var err error
		if m.selectedProvider().Auth == providerauth.AuthOpenAIChatGPT {
			result, err = m.deps.startOpenAIDevice(context.Background())
		} else {
			result, err = m.deps.startGitHubDevice(context.Background())
		}
		return deviceCodeMsg{result: result, err: err, flowID: m.flowID}
	}
}

func (m OnboardingModel) pollDeviceFlow(ctx context.Context, deviceCode, userCode string, interval int) tea.Cmd {
	return func() tea.Msg {
		var token string
		var err error
		if m.selectedProvider().Auth == providerauth.AuthOpenAIChatGPT {
			token, err = m.deps.pollOpenAIDevice(ctx, deviceCode, userCode, interval)
		} else {
			token, err = m.deps.pollGitHubDevice(ctx, deviceCode, interval)
		}
		return browserAuthResultMsg{token: token, err: err, flowID: m.flowID}
	}
}

func (m OnboardingModel) startAnthropicAuth(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		token, err := m.deps.startAnthropic(ctx)
		return browserAuthResultMsg{token: token, err: err, flowID: m.flowID}
	}
}

func (m OnboardingModel) startAnthropicMaxAuth(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		token, err := m.deps.startAnthropicMax(ctx)
		return browserAuthResultMsg{token: token, err: err, flowID: m.flowID}
	}
}

func (m OnboardingModel) updateAnthropicAuthChoice(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.flowID++
			m.step = stepSelectProvider
			m.cursor = m.providerIdx
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
			m.anthropicChoice = anthropicAuthChoice(m.cursor)
			switch m.cursor {
			case 0:
				// Direct API key entry (recommended)
				m.step = stepEnterAPIKey
				m.apiKeyInput.Placeholder = "sk-ant-api03-..."
				return m, m.apiKeyInput.Focus()
			case 1:
				// Console OAuth — creates permanent API key via browser
				m.step = stepBrowserAuth
				m.authing = true
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				m.authCancel = cancel
				return m, tea.Batch(m.spinner.Tick, m.startAnthropicAuth(ctx))
			case 2:
				// Max/Pro subscription OAuth — token used as Bearer directly
				m.step = stepBrowserAuth
				m.authing = true
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				m.authCancel = cancel
				return m, tea.Batch(m.spinner.Tick, m.startAnthropicMaxAuth(ctx))
			}
		}
	}
	return m, nil
}

func (m OnboardingModel) updateBrowserAuth(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			if m.authCancel != nil {
				m.authCancel()
				m.authCancel = nil
			}
			m.authing = false
			m.authError = ""
			m.flowID++
			m.step = stepSelectProvider
			m.cursor = m.providerIdx
			return m, nil
		}
	}
	return m, nil
}

func (m OnboardingModel) updateEnterAPIKey(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.apiKeyInput.SetValue("")
			m.authError = ""
			// Go back to Anthropic auth choice if this is an Anthropic provider
			p := m.selectedProvider()
			if p.Auth == providerauth.AuthAnthropic {
				m.step = stepAnthropicAuthChoice
				m.cursor = 0 // reset auth choice cursor
			} else {
				m.step = stepSelectProvider
				m.cursor = m.providerIdx // restore provider list cursor
			}
			return m, nil
		case "enter":
			p := m.selectedProvider()
			if p.CredentialRequired && m.apiKeyInput.Value() == "" {
				return m, nil
			}
			m.authToken = m.apiKeyInput.Value()
			return m.advanceAfterCredential()
		}
	}

	var cmd tea.Cmd
	m.apiKeyInput, cmd = m.apiKeyInput.Update(msg)
	return m, cmd
}

func (m OnboardingModel) updateOllamaChoice(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			m.flowID++
			m.step = stepSelectProvider
			m.cursor = m.providerIdx
			return m, nil
		case "j", "down":
			if m.ollamaChoiceCursor < 1 {
				m.ollamaChoiceCursor++
			}
		case "k", "up":
			if m.ollamaChoiceCursor > 0 {
				m.ollamaChoiceCursor--
			}
		case "enter":
			p := m.selectedProvider()
			if m.ollamaChoiceCursor == 0 {
				m.baseURLInput.SetValue(p.DefaultBaseURL)
				if len(p.Settings) > 0 {
					return m.beginSettings(stepEnterBaseURL)
				}
				m.step = stepEnterBaseURL
				return m, m.baseURLInput.Focus()
			}
			m.baseURLInput.SetValue(p.DefaultBaseURL)
			if len(p.Settings) > 0 {
				return m.beginSettings(stepOllamaSetup)
			}
			return m.beginOllamaSetup()
		}
	}
	return m, nil
}

func (m OnboardingModel) beginOllamaSetup() (OnboardingModel, tea.Cmd) {
	ctx, cancel := context.WithCancel(context.Background())
	m.ollamaSetupCancel = cancel
	m.step = stepOllamaSetup
	m.ollamaSetupStatus = "checking"
	m.ollamaSetupError = ""
	return m, tea.Batch(m.spinner.Tick, m.checkOllama(ctx))
}

func (m OnboardingModel) checkOllama(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		status, err := m.deps.setupOllama(ctx, m.baseURLInput.Value())
		return ollamaSetupMsg{status: status, err: err, flowID: m.flowID}
	}
}

// setupOllama checks reachability, installs Ollama if missing, and starts it.
func setupOllama(ctx context.Context, baseURL string) (string, error) {
	c := wfprovider.NewOllamaClient(baseURL)
	healthCtx, healthCancel := context.WithTimeout(ctx, 3*time.Second)
	err := c.Health(healthCtx)
	healthCancel()
	if err == nil {
		return "ready", nil
	}

	if _, lookErr := exec.LookPath("ollama"); lookErr != nil {
		if installErr := installOllamaQuiet(ctx); installErr != nil {
			return "failed", fmt.Errorf("install ollama: %w", installErr)
		}
		if _, lookErr = exec.LookPath("ollama"); lookErr != nil {
			return "failed", fmt.Errorf("ollama not found after install")
		}
	}
	if err := ctx.Err(); err != nil {
		return "failed", err
	}

	cmd := exec.Command("ollama", "serve")
	cmd.Stdout = nil
	cmd.Stderr = nil
	if startErr := cmd.Start(); startErr != nil {
		return "failed", fmt.Errorf("start ollama: %w", startErr)
	}

	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	poll := time.NewTicker(500 * time.Millisecond)
	defer poll.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return "failed", ctx.Err()
		case <-deadline.C:
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return "failed", fmt.Errorf("ollama did not become ready within 15s")
		case <-poll.C:
			pollCtx, pollCancel := context.WithTimeout(ctx, 2*time.Second)
			pollErr := c.Health(pollCtx)
			pollCancel()
			if pollErr == nil {
				return "ready", nil
			}
		}
	}
}

// installOllamaQuiet installs Ollama silently (output suppressed for TUI).
func installOllamaQuiet(ctx context.Context) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.CommandContext(ctx, "brew", "install", "ollama")
	case "linux":
		script := `set -e; t=$(mktemp); curl -fsSL https://ollama.com/install.sh -o "$t"; sh "$t"; rm -f "$t"`
		cmd = exec.CommandContext(ctx, "sh", "-c", script)
	default:
		return fmt.Errorf("automatic install not supported on %s — install from https://ollama.com/download", runtime.GOOS)
	}
	// Suppress output for TUI context.
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// recommendedOllamaModels returns a list of recommended models for new users.
// Currently static since Ollama has no public library API. Centralized here
// so it's easy to update. The "Custom" option in the TUI lets users enter any name.
func recommendedOllamaModels() []string {
	return []string{"qwen3:8b", "llama3.1:8b", "gemma3:4b"}
}

func (m OnboardingModel) updateOllamaSetup(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	switch msg := msg.(type) {
	case ollamaSetupMsg:
		if msg.flowID != m.flowID {
			return m, nil
		}
		if m.ollamaSetupCancel != nil {
			m.ollamaSetupCancel()
			m.ollamaSetupCancel = nil
		}
		m.ollamaSetupStatus = msg.status
		if msg.err != nil {
			m.ollamaSetupError = msg.err.Error()
		}
		if msg.status == "ready" {
			// Ollama is running → go to model pull
			m.step = stepPullModel
			m.pullCursor = 0
			m.pullError = ""
			m.pullingModel = false
			m.recommendedModels = recommendedOllamaModels()
			return m, nil
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "esc" {
			if m.ollamaSetupCancel != nil {
				m.ollamaSetupCancel()
				m.ollamaSetupCancel = nil
			}
			m.flowID++
			m.step = stepOllamaChoice
			m.ollamaChoiceCursor = 0
			return m, nil
		}
	}
	return m, nil
}

func (m OnboardingModel) updateEnterBaseURL(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		switch keyMsg.String() {
		case "esc":
			p := m.selectedProvider()
			if len(p.Settings) > 0 {
				m.settingIdx = len(p.Settings) - 1
				m.settingsNext = stepEnterBaseURL
				m.step = stepEnterSettings
				m.prepareSettingInput()
				return m, m.settingInput.Focus()
			}
			if p.Auth == providerauth.AuthAPIKey || p.Auth == providerauth.AuthAnthropic || p.Auth == providerauth.AuthGitHubDevice {
				m.step = stepEnterAPIKey
				return m, m.apiKeyInput.Focus()
			}
			if p.Type == "ollama" {
				m.step = stepOllamaChoice
				return m, nil
			}
			m.step = stepSelectProvider
			m.cursor = m.providerIdx
			return m, nil
		case "enter":
			if m.selectedProvider().BaseURLRequired && m.baseURLInput.Value() == "" {
				return m, nil
			}
			return m.transitionToModelStrategy()
		}
	}

	var cmd tea.Cmd
	m.baseURLInput, cmd = m.baseURLInput.Update(msg)
	return m, cmd
}

func (m OnboardingModel) updateFetchModels(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if keyMsg.String() == "esc" {
			m.fetchingModels = false
			m.flowID++
			m.step = stepSelectProvider
			m.cursor = m.providerIdx
			return m, nil
		}
	}
	return m, nil
}

func (m OnboardingModel) updatePullModel(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	// Allow Esc to cancel an in-progress pull.
	if m.pullingModel {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok && keyMsg.String() == "esc" {
			if m.pullCancel != nil {
				m.pullCancel()
				m.pullCancel = nil
			}
			m.pullingModel = false
			m.pullError = ""
			m.flowID++
			m.step = stepSelectProvider
			m.cursor = m.providerIdx
		}
		return m, nil
	}

	// Custom model name entry mode.
	if m.pullEnteringCustom {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch keyMsg.String() {
			case "esc":
				m.pullEnteringCustom = false
				m.pullCustomInput.SetValue("")
				return m, nil
			case "enter":
				name := strings.TrimSpace(m.pullCustomInput.Value())
				if name == "" {
					return m, nil
				}
				m.pullEnteringCustom = false
				m.pullModelName = name
				return m.startPull()
			}
		}
		var cmd tea.Cmd
		m.pullCustomInput, cmd = m.pullCustomInput.Update(msg)
		return m, cmd
	}

	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		customIdx := len(m.recommendedModels) // index for the "Custom" row
		switch keyMsg.String() {
		case "esc":
			m.flowID++
			m.step = stepSelectProvider
			m.cursor = m.providerIdx
			return m, nil
		case "j", "down":
			if m.pullCursor < customIdx {
				m.pullCursor++
			}
		case "k", "up":
			if m.pullCursor > 0 {
				m.pullCursor--
			}
		case "enter", " ":
			if m.pullCursor == customIdx {
				// Enter custom model name
				m.pullEnteringCustom = true
				m.pullCustomInput = textinput.New()
				m.pullCustomInput.Placeholder = "e.g. mistral:7b"
				m.pullCustomInput.Prompt = ""
				return m, m.pullCustomInput.Focus()
			}
			m.pullModelName = m.recommendedModels[m.pullCursor]
			return m.startPull()
		}
		// Number shortcuts (1 = recommended[0], ..., N+1 = Custom)
		for i := range m.recommendedModels {
			if keyMsg.String() == fmt.Sprintf("%d", i+1) {
				m.pullCursor = i
			}
		}
		if keyMsg.String() == fmt.Sprintf("%d", customIdx+1) {
			m.pullCursor = customIdx
		}
	}
	return m, nil
}

// startPull initiates the async model pull and returns the updated model and initial tea.Cmd.
func (m OnboardingModel) startPull() (OnboardingModel, tea.Cmd) {
	m.pullingModel = true
	m.pullProgress = 0
	m.pullError = ""
	progressCh := make(chan float64, 50)
	doneCh := make(chan error, 1)
	m.pullProgressCh = progressCh
	m.pullDoneCh = doneCh
	ctx, cancel := context.WithCancel(context.Background())
	m.pullCancel = cancel
	baseURL := m.baseURLInput.Value()
	pullName := m.pullModelName
	go func() {
		defer close(progressCh) // signal readPullProgress that no more updates are coming
		c := wfprovider.NewOllamaClient(baseURL)
		err := c.Pull(ctx, pullName, func(pct float64) {
			select {
			case <-ctx.Done():
				return
			default:
			}
			// Non-blocking send: drop progress updates if UI can't keep up
			// to avoid backpressuring the download.
			select {
			case progressCh <- pct:
			default:
			}
		})
		doneCh <- err
	}()
	return m, tea.Batch(m.spinner.Tick, m.readPullProgress())
}

// readPullProgress returns a tea.Cmd that waits for the next pull progress update.
// Prioritizes doneCh so completion isn't delayed by buffered progress updates.
func (m OnboardingModel) readPullProgress() tea.Cmd {
	return func() tea.Msg {
		// Check doneCh first (non-blocking) to prioritize completion.
		select {
		case err := <-m.pullDoneCh:
			return pullDoneMsg{err: err, flowID: m.flowID}
		default:
		}
		// Wait for either progress or done.
		select {
		case pct, ok := <-m.pullProgressCh:
			if ok {
				return pullProgressMsg{pct: pct, flowID: m.flowID}
			}
			// progressCh closed — pull goroutine finished, read result.
			err := <-m.pullDoneCh
			return pullDoneMsg{err: err, flowID: m.flowID}
		case err := <-m.pullDoneCh:
			return pullDoneMsg{err: err, flowID: m.flowID}
		}
	}
}

func (m OnboardingModel) updateSelectModel(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	models := m.fetchedModels
	if m.enteringManualModel {
		if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
			switch keyMsg.String() {
			case "esc":
				if len(models) > 0 {
					m.enteringManualModel = false
					m.manualModelInput.SetValue("")
					return m, nil
				}
				m.step = stepSelectProvider
				m.cursor = 0
				return m, nil
			case "enter":
				model := strings.TrimSpace(m.manualModelInput.Value())
				if model == "" {
					return m, nil
				}
				m.selectedModel = model
				m.enteringManualModel = false
				m.step = stepReview
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.manualModelInput, cmd = m.manualModelInput.Update(msg)
		return m, cmd
	}
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		customIndex := len(models)
		switch keyMsg.String() {
		case "esc":
			p := m.selectedProvider()
			if p.PromptBaseURL {
				m.step = stepEnterBaseURL
				return m, m.baseURLInput.Focus()
			}
			m.step = stepSelectProvider
			m.cursor = 0
			return m, nil
		case "j", "down":
			maxCursor := len(models) - 1
			if m.selectedProvider().AllowManualModel {
				maxCursor = customIndex
			}
			if m.modelCursor < maxCursor {
				m.modelCursor++
			}
		case "k", "up":
			if m.modelCursor > 0 {
				m.modelCursor--
			}
		case "enter", " ":
			if m.selectedProvider().AllowManualModel && m.modelCursor == customIndex {
				m.enteringManualModel = true
				m.manualModelInput.SetValue("")
				return m, m.manualModelInput.Focus()
			}
			if m.modelCursor >= 0 && m.modelCursor < len(models) {
				m.selectedModel = models[m.modelCursor].ID
				m.step = stepReview
				return m, nil
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

func (m OnboardingModel) addProvider(p providerauth.SetupEntry, model string) tea.Cmd {
	return func() tea.Msg {
		settingsJSON := ""
		if len(m.settings) > 0 {
			data, err := json.Marshal(m.settings)
			if err != nil {
				return providerAddedMsg{err: fmt.Errorf("encode provider settings: %w", err), flowID: m.flowID}
			}
			settingsJSON = string(data)
		}
		baseURL := m.baseURLInput.Value()
		if p.Model == providerauth.ModelExternal {
			baseURL = m.cliWorkingDir
		}
		req := &pb.AddProviderReq{
			Alias:     providerAlias(p),
			Type:      p.Type,
			Model:     model,
			ApiKey:    m.authToken,
			BaseUrl:   baseURL,
			Settings:  settingsJSON,
			IsDefault: true,
		}
		provider, err := m.deps.addProvider(context.Background(), req)
		return providerAddedMsg{provider: provider, err: err, flowID: m.flowID}
	}
}

func (m OnboardingModel) testProvider(alias string) tea.Cmd {
	return func() tea.Msg {
		result, err := m.deps.testProvider(context.Background(), alias)
		return providerTestedMsg{result: result, err: err, flowID: m.flowID}
	}
}

func (m OnboardingModel) removeProvider(alias string) tea.Cmd {
	return func() tea.Msg {
		return providerRemovedMsg{err: m.deps.removeProvider(context.Background(), alias), flowID: m.flowID}
	}
}

func providerAlias(entry providerauth.SetupEntry) string {
	if entry.DefaultAlias != "" {
		return entry.DefaultAlias
	}
	return entry.Type
}

func (m OnboardingModel) updateTestConnection(msg tea.Msg) (OnboardingModel, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if m.testing || m.removing {
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
							Alias:     providerAlias(p),
							Type:      p.Type,
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
				return m, tea.Batch(m.spinner.Tick, m.testProvider(providerAlias(p)))
			}
			return m.startTest()
		case "b", "esc":
			if m.added {
				p := m.selectedProvider()
				m.removing = true
				return m, tea.Batch(m.spinner.Tick, m.removeProvider(providerAlias(p)))
			}
			m.step = stepReview
			m.testResult = nil
			m.testError = ""
			return m, nil
		}
	}
	return m, nil
}

func (m OnboardingModel) View(t theme.Theme, width, height int) string {
	w, h := width, height
	if m.width > 0 {
		w = m.width
	}
	if m.height > 0 {
		h = m.height
	}

	cardWidth := 62
	if w > 0 && cardWidth > w-6 {
		cardWidth = w - 6
	}
	if cardWidth < 24 {
		cardWidth = 24
	}
	contentWidth := cardWidth - 6

	var sb strings.Builder
	titleStyle := lipgloss.NewStyle().Foreground(t.Primary).Bold(true)
	selectedStyle := lipgloss.NewStyle().Foreground(t.Foreground).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(t.Muted)
	successStyle := lipgloss.NewStyle().Foreground(t.Success)
	errorStyle := lipgloss.NewStyle().Foreground(t.Error)
	wrapMuted := mutedStyle.Width(contentWidth)

	sb.WriteString(titleStyle.Render("Welcome to ratchet") + "\n\n")
	var dots []string
	for i := 0; i < m.stepCount(); i++ {
		style := mutedStyle
		dot := "○"
		if i <= m.currentStepIndex() {
			style = lipgloss.NewStyle().Foreground(t.Primary)
			dot = "●"
		}
		dots = append(dots, style.Render(dot))
	}
	sb.WriteString(strings.Join(dots, " ") + "\n\n")

	switch m.step {
	case stepSelectProvider:
		sb.WriteString("Select your AI provider:\n\n")
		if m.filtering {
			sb.WriteString("Filter: " + m.filterInput.View() + "\n\n")
		}
		indices := m.filteredProviderIndices()
		if len(indices) == 0 {
			sb.WriteString(errorStyle.Render("No providers match this filter.") + "\n")
		} else {
			cursor := m.cursor
			if cursor >= len(indices) {
				cursor = len(indices) - 1
			}
			maxVisible := 3
			if h >= 36 {
				maxVisible = 12
			} else if m.filtering {
				maxVisible = 1
			}
			start := max(0, cursor-maxVisible+1)
			end := min(len(indices), start+maxVisible)
			if start > 0 {
				sb.WriteString(mutedStyle.Render("  ... more above") + "\n")
			}
			var lastCategory providerauth.Category
			for visibleIndex := start; visibleIndex < end; visibleIndex++ {
				entry := m.providers[indices[visibleIndex]]
				if entry.Category != lastCategory {
					sb.WriteString(titleStyle.Render(categoryLabel(entry.Category)) + "\n")
					lastCategory = entry.Category
				}
				prefix := "  "
				style := mutedStyle
				if visibleIndex == cursor {
					prefix = "▶ "
					style = selectedStyle
				}
				shortcut := "  "
				if visibleIndex < 9 {
					shortcut = fmt.Sprintf("%d.", visibleIndex+1)
				}
				sb.WriteString(style.Render(fmt.Sprintf("%s%-2s %s", prefix, shortcut, entry.DisplayName)) + "\n")
			}
			if end < len(indices) {
				sb.WriteString(mutedStyle.Render("  ... more below") + "\n")
			}
			selected := m.providers[indices[cursor]]
			sb.WriteString("\n" + wrapMuted.Render(selected.Description) + "\n")
		}
		sb.WriteString("\n" + mutedStyle.Render("↑/↓: select  /: filter  Enter: confirm  Esc: clear filter"))

	case stepAnthropicAuthChoice:
		sb.WriteString("Sign in with Anthropic\n\n")
		choices := []string{
			"Enter API key (recommended)",
			"Console OAuth (creates an API key)",
			"Max/Pro subscription OAuth (experimental)",
		}
		writeChoiceList(&sb, choices, m.cursor, selectedStyle, mutedStyle)
		sb.WriteString("\n" + mutedStyle.Render("↑/↓ or 1-3: select  Enter: confirm  Esc: back"))

	case stepBrowserAuth:
		p := m.selectedProvider()
		if m.authing && m.deviceUserCode != "" {
			sb.WriteString("Sign in with " + p.DisplayName + "\n\n")
			codeStyle := lipgloss.NewStyle().Bold(true).Foreground(t.Accent)
			sb.WriteString("Your code: " + codeStyle.Render(m.deviceUserCode) + "\n\n")
			sb.WriteString(m.spinner.View() + " Waiting for authorization...\n\n")
			sb.WriteString(wrapMuted.Render("Enter the code at "+m.deviceVerificationURI) + "\n")
			sb.WriteString(mutedStyle.Render("Esc: cancel"))
		} else if m.authing {
			sb.WriteString("Sign in with " + p.DisplayName + "\n\n")
			sb.WriteString(m.spinner.View() + " Waiting for browser sign-in...\n\n")
			if m.anthropicChoice == anthropicChoiceMaxOAuth {
				sb.WriteString(wrapMuted.Render("Experimental subscription access may have API restrictions.") + "\n")
			}
			sb.WriteString(mutedStyle.Render("Esc: cancel"))
		} else if m.authError != "" {
			sb.WriteString(errorStyle.Render("Authentication issue") + "\n\n")
			sb.WriteString(wrapMuted.Render(m.authError) + "\n\n")
			sb.WriteString(mutedStyle.Render("Esc: back"))
		}

	case stepEnterAPIKey:
		p := m.selectedProvider()
		sb.WriteString("Configure " + p.DisplayName + "\n\n")
		if p.AuthHint != "" {
			sb.WriteString(wrapMuted.Render(p.AuthHint) + "\n\n")
		}
		sb.WriteString(p.CredentialLabel + ": " + m.apiKeyInput.View() + "\n\n")
		sb.WriteString(wrapMuted.Render("The credential is stored through the daemon secrets provider and is not shown again.") + "\n")
		sb.WriteString(mutedStyle.Render("Enter: continue  Esc: back"))

	case stepOllamaChoice:
		sb.WriteString("How would you like to set up Ollama?\n\n")
		choices := []string{
			"Use an existing local model server",
			"Install/start Ollama and download a model",
		}
		writeChoiceList(&sb, choices, m.ollamaChoiceCursor, selectedStyle, mutedStyle)
		sb.WriteString("\n" + mutedStyle.Render("↑/↓: select  Enter: confirm  Esc: back"))

	case stepOllamaSetup:
		if m.ollamaSetupStatus == "failed" {
			sb.WriteString(errorStyle.Render("Ollama setup failed") + "\n\n")
			sb.WriteString(wrapMuted.Render(m.ollamaSetupError) + "\n\n")
			sb.WriteString(mutedStyle.Render("Esc: back"))
		} else {
			sb.WriteString(m.spinner.View() + " Setting up Ollama...\n\n")
			sb.WriteString(wrapMuted.Render("Checking the installation and starting the local server.") + "\n\n")
			sb.WriteString(mutedStyle.Render("Esc: cancel"))
		}

	case stepEnterSettings:
		p := m.selectedProvider()
		if m.settingIdx >= 0 && m.settingIdx < len(p.Settings) {
			field := p.Settings[m.settingIdx]
			fmt.Fprintf(&sb, "Configure %s\n\n%s (%d/%d)\n\n", p.DisplayName, field.Label, m.settingIdx+1, len(p.Settings))
			if len(field.Choices) > 0 {
				writeChoiceList(&sb, field.Choices, m.settingChoice, selectedStyle, mutedStyle)
			} else {
				sb.WriteString(m.settingInput.View() + "\n")
			}
			if m.settingsError != "" {
				sb.WriteString("\n" + errorStyle.Render(m.settingsError) + "\n")
			}
			sb.WriteString("\n" + mutedStyle.Render("↑/↓: select  Enter: continue  Esc: back"))
		}

	case stepEnterBaseURL:
		p := m.selectedProvider()
		sb.WriteString("Configure " + p.DisplayName + " endpoint\n\n")
		sb.WriteString("URL: " + m.baseURLInput.View() + "\n\n")
		if p.Type == "ollama" {
			sb.WriteString(wrapMuted.Render("The default works for Ollama; compatible local servers are also supported.") + "\n\n")
		}
		sb.WriteString(mutedStyle.Render("Enter: continue  Esc: back"))

	case stepFetchModels:
		p := m.selectedProvider()
		sb.WriteString(m.spinner.View() + " Loading models from " + p.DisplayName + "...\n\n")
		sb.WriteString(mutedStyle.Render("Esc: cancel"))

	case stepPullModel:
		if m.pullEnteringCustom {
			sb.WriteString("Enter model name to pull:\n\nModel: " + m.pullCustomInput.View() + "\n\n")
			sb.WriteString(mutedStyle.Render("Enter: pull  Esc: back"))
			break
		}
		sb.WriteString("No models are installed. Pull one to continue:\n\n")
		choices := append(slices.Clone(m.recommendedModels), "Custom model name")
		writeChoiceList(&sb, choices, m.pullCursor, selectedStyle, mutedStyle)
		if m.pullingModel {
			status := fitText(fmt.Sprintf("Pulling %s... %.0f%%", m.pullModelName, m.pullProgress), contentWidth-2)
			fmt.Fprintf(&sb, "\n%s %s\n", m.spinner.View(), status)
		} else if m.pullError != "" {
			sb.WriteString("\n" + errorStyle.Render(fitText("Pull failed: "+m.pullError, contentWidth)) + "\n")
		}
		sb.WriteString("\n" + mutedStyle.Render("↑/↓: select  Enter: pull  Esc: back"))

	case stepSelectModel:
		if m.enteringManualModel {
			sb.WriteString("Enter the provider model ID:\n\nModel: " + m.manualModelInput.View() + "\n\n")
			if m.modelsError != "" {
				sb.WriteString(wrapMuted.Render("Automatic discovery was unavailable: "+m.modelsError) + "\n\n")
			}
			sb.WriteString(mutedStyle.Render("Enter: continue  Esc: back"))
			break
		}
		sb.WriteString("Select your default model:\n\n")
		models := m.fetchedModels
		maxVisible := 10
		start := max(0, m.modelCursor-maxVisible+1)
		end := min(len(models), start+maxVisible)
		for i := start; i < end; i++ {
			prefix, style := "  ", mutedStyle
			if i == m.modelCursor {
				prefix, style = "▶ ", selectedStyle
			}
			label := models[i].Name
			if label == "" {
				label = models[i].ID
			}
			sb.WriteString(style.Render(fitText(prefix+label, contentWidth)) + "\n")
		}
		if m.selectedProvider().AllowManualModel {
			prefix, style := "  ", mutedStyle
			if m.modelCursor == len(models) {
				prefix, style = "▶ ", selectedStyle
			}
			sb.WriteString(style.Render(prefix+"Enter model ID manually") + "\n")
		}
		sb.WriteString("\n" + mutedStyle.Render("↑/↓: select  Enter: confirm  Esc: back"))

	case stepCLISetup:
		p := m.selectedProvider()
		sb.WriteString("Checking " + p.DisplayName + "\n\n")
		if m.cliError == "" {
			sb.WriteString(m.spinner.View() + " Looking for " + p.CLICommand + " on PATH...\n\n")
			sb.WriteString(mutedStyle.Render("Esc: cancel"))
		} else {
			sb.WriteString(errorStyle.Render("CLI setup check failed") + "\n\n")
			sb.WriteString(wrapMuted.Render(m.cliError) + "\n\n")
			sb.WriteString(wrapMuted.Render(p.InstallHint) + "\n\n")
			sb.WriteString(mutedStyle.Render("Esc: back"))
		}

	case stepReview:
		p := m.selectedProvider()
		sb.WriteString("Review provider setup\n\n")
		sb.WriteString("Provider: " + p.DisplayName + "\n")
		if model := m.selectedModelID(); model != "" {
			sb.WriteString(fitReviewValue("Model", model, contentWidth) + "\n")
		}
		if m.cliCommandPath != "" {
			sb.WriteString(fitReviewValue("Command", m.cliCommandPath, contentWidth) + "\n")
		}
		if m.cliWorkingDir != "" {
			sb.WriteString(fitReviewValue("Working directory", m.cliWorkingDir, contentWidth) + "\n")
		}
		if baseURL := strings.TrimSpace(m.baseURLInput.Value()); baseURL != "" {
			sb.WriteString(fitReviewValue("Endpoint", baseURL, contentWidth) + "\n")
		}
		if m.authToken != "" {
			sb.WriteString(successStyle.Render("Credential configured") + "\n")
		}
		keys := make([]string, 0, len(m.settings))
		for key := range m.settings {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			sb.WriteString(fitReviewValue(settingLabel(p, key), m.settings[key], contentWidth) + "\n")
		}
		sb.WriteString("\n" + mutedStyle.Render("Enter: save and test  b/Esc: back"))

	case stepTestConnection:
		p := m.selectedProvider()
		if m.removing {
			sb.WriteString(m.spinner.View() + " Removing the failed provider configuration...\n")
		} else if m.testing {
			sb.WriteString(m.spinner.View() + " Testing connection to " + p.DisplayName + "...\n")
		} else if m.testResult != nil && m.testResult.Success {
			sb.WriteString(successStyle.Render("Connection successful") + "\n\n")
			sb.WriteString("Provider: " + p.Type + "\n")
			if model := m.selectedModelID(); model != "" {
				sb.WriteString(fitReviewValue("Default model", model, contentWidth) + "\n")
			}
			fmt.Fprintf(&sb, "Response time: %dms\n", m.testResult.LatencyMs)
			sb.WriteString("\n" + mutedStyle.Render("Enter: start chatting"))
		} else {
			sb.WriteString(errorStyle.Render("Connection failed") + "\n\n")
			sb.WriteString(wrapMuted.Render(m.testError) + "\n\n")
			sb.WriteString(mutedStyle.Render("r: retry  b/Esc: back"))
		}
	}

	card := t.OnboardingCard.Width(cardWidth).Render(strings.TrimRight(sb.String(), "\n"))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, card)
}

func writeChoiceList(sb *strings.Builder, choices []string, cursor int, selectedStyle, mutedStyle lipgloss.Style) {
	for i, label := range choices {
		prefix, style := "  ", mutedStyle
		if i == cursor {
			prefix, style = "▶ ", selectedStyle
		}
		sb.WriteString(style.Render(fmt.Sprintf("%s%d. %s", prefix, i+1, label)) + "\n")
	}
}

func categoryLabel(category providerauth.Category) string {
	switch category {
	case providerauth.CategoryAPI:
		return "API providers"
	case providerauth.CategoryCompatible:
		return "Compatible endpoints"
	case providerauth.CategorySubscription:
		return "Subscriptions"
	case providerauth.CategoryCloud:
		return "Cloud platforms"
	case providerauth.CategoryLocal:
		return "Local runtimes"
	case providerauth.CategoryCLI:
		return "External CLI agents"
	default:
		return string(category)
	}
}

func settingLabel(entry providerauth.SetupEntry, key string) string {
	for _, field := range entry.Settings {
		if field.Key == key {
			return field.Label
		}
	}
	return key
}

func fitReviewValue(label, value string, width int) string {
	return fitText(label+": "+value, width)
}

func fitText(text string, width int) string {
	if width <= 3 || lipgloss.Width(text) <= width {
		return text
	}
	tail := "..."
	limit := width - lipgloss.Width(tail)
	var b strings.Builder
	currentWidth := 0
	for _, r := range text {
		runeWidth := lipgloss.Width(string(r))
		if currentWidth+runeWidth > limit {
			break
		}
		b.WriteRune(r)
		currentWidth += runeWidth
	}
	return b.String() + tail
}

func redactProviderError(err error, credential string) string {
	if err == nil {
		return ""
	}
	redactor := secrets.NewRedactor()
	redactor.AddValue("provider credential", credential)
	return redactor.Redact(err.Error())
}

func (m OnboardingModel) stepCount() int {
	return 5
}

func (m OnboardingModel) currentStepIndex() int {
	switch m.step {
	case stepSelectProvider:
		return 0
	case stepAnthropicAuthChoice, stepBrowserAuth, stepEnterAPIKey, stepOllamaChoice, stepOllamaSetup, stepEnterSettings, stepEnterBaseURL, stepCLISetup:
		return 1
	case stepFetchModels, stepPullModel, stepSelectModel:
		return 2
	case stepReview:
		return 3
	case stepTestConnection:
		return 4
	default:
		return 0
	}
}
