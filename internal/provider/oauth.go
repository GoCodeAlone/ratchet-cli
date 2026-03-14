package providerauth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// OAuthResult is returned when a browser-based auth flow completes.
type OAuthResult struct {
	Token string
	Err   error
}

// DeviceCodeResult holds the response from a device code request.
type DeviceCodeResult struct {
	DeviceCode      string
	UserCode        string
	VerificationURI string
	ExpiresIn       int
	Interval        int
}

// Anthropic OAuth constants.
const (
	anthropicAuthURL      = "https://console.anthropic.com/oauth/authorize"
	anthropicTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	anthropicClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicCreateKeyURL = "https://api.anthropic.com/api/oauth/claude_cli/create_api_key"
)

// GitHub device flow constants.
const (
	githubDeviceCodeURL = "https://github.com/login/device/code"
	githubTokenURL      = "https://github.com/login/oauth/access_token"
	// GithubCopilotClientID is GitHub Copilot's official OAuth App client ID.
	GithubCopilotClientID = "Iv1.b507a08c87ecfe98"
)

// StartAnthropicOAuth runs the OAuth PKCE flow for Anthropic/Claude.
// It starts a local HTTP server, opens the browser to the auth URL,
// and waits for the callback with the authorization code.
func StartAnthropicOAuth(ctx context.Context) <-chan OAuthResult {
	ch := make(chan OAuthResult, 1)

	go func() {
		defer close(ch)

		// Generate PKCE code verifier + challenge
		verifier, challenge, err := generatePKCE()
		if err != nil {
			ch <- OAuthResult{Err: fmt.Errorf("generate PKCE: %w", err)}
			return
		}

		state, err := randomHex(16)
		if err != nil {
			ch <- OAuthResult{Err: fmt.Errorf("generate state: %w", err)}
			return
		}

		// Start local server on random port
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			ch <- OAuthResult{Err: fmt.Errorf("start callback server: %w", err)}
			return
		}
		port := listener.Addr().(*net.TCPAddr).Port
		redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

		// Build auth URL
		authURL := fmt.Sprintf("%s?response_type=code&client_id=%s&redirect_uri=%s&state=%s&code_challenge=%s&code_challenge_method=S256&scope=api",
			anthropicAuthURL,
			url.QueryEscape(anthropicClientID),
			url.QueryEscape(redirectURI),
			url.QueryEscape(state),
			url.QueryEscape(challenge),
		)

		codeCh := make(chan string, 1)
		errCh := make(chan error, 1)

		mux := http.NewServeMux()
		mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("state") != state {
				errCh <- fmt.Errorf("state mismatch")
				http.Error(w, "State mismatch", http.StatusBadRequest)
				return
			}
			if errMsg := r.URL.Query().Get("error"); errMsg != "" {
				errCh <- fmt.Errorf("auth error: %s — %s", errMsg, r.URL.Query().Get("error_description"))
				fmt.Fprintf(w, "<html><body><h2>Authentication failed</h2><p>%s</p><p>You can close this tab.</p></body></html>", html.EscapeString(errMsg))
				return
			}
			code := r.URL.Query().Get("code")
			if code == "" {
				errCh <- fmt.Errorf("no authorization code in callback")
				http.Error(w, "No code", http.StatusBadRequest)
				return
			}
			codeCh <- code
			fmt.Fprintf(w, "<html><body><h2>Authenticated!</h2><p>You can close this tab and return to ratchet.</p></body></html>")
		})

		server := &http.Server{Handler: mux}
		go server.Serve(listener) //nolint:errcheck
		defer server.Close()

		// Open browser
		if err := OpenBrowserURL(authURL); err != nil {
			ch <- OAuthResult{Err: fmt.Errorf("open browser: %w", err)}
			return
		}

		// Wait for callback or context cancellation
		select {
		case code := <-codeCh:
			// Exchange code for token
			token, err := exchangeAnthropicCode(ctx, code, verifier, redirectURI)
			ch <- OAuthResult{Token: token, Err: err}
		case err := <-errCh:
			ch <- OAuthResult{Err: err}
		case <-ctx.Done():
			ch <- OAuthResult{Err: ctx.Err()}
		}
	}()

	return ch
}

func exchangeAnthropicCode(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	// Step 1: Exchange authorization code for access token (JSON body, not form-encoded)
	tokenReqBody, err := json.Marshal(map[string]any{
		"grant_type":    "authorization_code",
		"client_id":     anthropicClientID,
		"code":          code,
		"redirect_uri":  redirectURI,
		"code_verifier": verifier,
	})
	if err != nil {
		return "", fmt.Errorf("marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", anthropicTokenURL, bytes.NewReader(tokenReqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed (%d): %s", resp.StatusCode, string(body))
	}

	var tokenResult struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tokenResult); err != nil {
		return "", fmt.Errorf("parse token response: %w", err)
	}
	if tokenResult.Error != "" {
		return "", fmt.Errorf("token error: %s", tokenResult.Error)
	}
	if tokenResult.AccessToken == "" {
		return "", fmt.Errorf("no access_token in response")
	}

	// Step 2: Exchange access token for a permanent API key
	keyReqBody, err := json.Marshal(map[string]any{
		"name": "ratchet-cli",
	})
	if err != nil {
		return "", fmt.Errorf("marshal key request: %w", err)
	}

	keyReq, err := http.NewRequestWithContext(ctx, "POST", anthropicCreateKeyURL, bytes.NewReader(keyReqBody))
	if err != nil {
		return "", err
	}
	keyReq.Header.Set("Authorization", "Bearer "+tokenResult.AccessToken)
	keyReq.Header.Set("Content-Type", "application/json")
	keyReq.Header.Set("Accept", "application/json")

	keyResp, err := http.DefaultClient.Do(keyReq)
	if err != nil {
		return "", fmt.Errorf("create api key: %w", err)
	}
	defer keyResp.Body.Close()

	keyBody, _ := io.ReadAll(keyResp.Body)
	if keyResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create api key failed (%d): %s", keyResp.StatusCode, string(keyBody))
	}

	var keyResult struct {
		RawKey string `json:"raw_key"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(keyBody, &keyResult); err != nil {
		return "", fmt.Errorf("parse api key response: %w", err)
	}
	if keyResult.Error != "" {
		return "", fmt.Errorf("api key error: %s", keyResult.Error)
	}
	if keyResult.RawKey == "" {
		return "", fmt.Errorf("no raw_key in api key response")
	}
	return keyResult.RawKey, nil
}

// StartGitHubDeviceFlow initiates the GitHub device code flow.
// Returns the device code result so the TUI can display the user code,
// then call PollGitHubDeviceFlow to wait for authorization.
func StartGitHubDeviceFlow(ctx context.Context, clientID string) (*DeviceCodeResult, error) {
	data := url.Values{
		"client_id": {clientID},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", githubDeviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device code request failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		DeviceCode      string `json:"device_code"`
		UserCode        string `json:"user_code"`
		VerificationURI string `json:"verification_uri"`
		ExpiresIn       int    `json:"expires_in"`
		Interval        int    `json:"interval"`
		Error           string `json:"error"`
		ErrorDesc       string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s: %s", result.Error, result.ErrorDesc)
	}

	interval := result.Interval
	if interval < 5 {
		interval = 5
	}

	return &DeviceCodeResult{
		DeviceCode:      result.DeviceCode,
		UserCode:        result.UserCode,
		VerificationURI: result.VerificationURI,
		ExpiresIn:       result.ExpiresIn,
		Interval:        interval,
	}, nil
}

// PollGitHubDeviceFlow polls for the device flow token until authorized or timeout.
func PollGitHubDeviceFlow(ctx context.Context, clientID, deviceCode string, interval int) <-chan OAuthResult {
	ch := make(chan OAuthResult, 1)

	go func() {
		defer close(ch)

		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				ch <- OAuthResult{Err: ctx.Err()}
				return
			case <-ticker.C:
				token, done, err := pollGitHubToken(ctx, clientID, deviceCode)
				if err != nil {
					ch <- OAuthResult{Err: err}
					return
				}
				if done {
					ch <- OAuthResult{Token: token}
					return
				}
			}
		}
	}()

	return ch
}

func pollGitHubToken(ctx context.Context, clientID, deviceCode string) (token string, done bool, err error) {
	data := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", githubTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("poll token: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		ErrorDesc   string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", false, fmt.Errorf("parse poll response: %w", err)
	}

	switch result.Error {
	case "":
		if result.AccessToken == "" {
			return "", false, fmt.Errorf("empty access token in response")
		}
		return result.AccessToken, true, nil
	case "authorization_pending":
		return "", false, nil
	case "slow_down":
		return "", false, nil
	case "expired_token":
		return "", false, fmt.Errorf("device code expired, please try again")
	case "access_denied":
		return "", false, fmt.Errorf("authorization denied by user")
	default:
		return "", false, fmt.Errorf("%s: %s", result.Error, result.ErrorDesc)
	}
}

// OpenBrowserURL opens a URL in the default browser.
func OpenBrowserURL(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}

func generatePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h[:])
	return verifier, challenge, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
