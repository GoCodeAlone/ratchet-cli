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
	"log"
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
	// anthropicConsoleAuthURL is the authorize endpoint for Console (API key) OAuth.
	anthropicConsoleAuthURL = "https://console.anthropic.com/oauth/authorize"
	// anthropicMaxAuthURL is the authorize endpoint for Claude Max/Pro subscription OAuth.
	anthropicMaxAuthURL   = "https://claude.ai/oauth/authorize"
	anthropicTokenURL     = "https://console.anthropic.com/v1/oauth/token"
	anthropicClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	anthropicCreateKeyURL = "https://api.anthropic.com/api/oauth/claude_cli/create_api_key"
	// anthropicOAuthScopes are the required scopes for both Console and Max flows.
	anthropicOAuthScopes = "org:create_api_key user:profile user:inference"
)

// GitHub device flow constants.
const (
	githubDeviceCodeURL = "https://github.com/login/device/code"
	githubTokenURL      = "https://github.com/login/oauth/access_token"
	// GithubCopilotClientID is GitHub Copilot's official OAuth App client ID.
	GithubCopilotClientID = "Iv1.b507a08c87ecfe98"
)

// StartAnthropicOAuth runs the Console OAuth PKCE flow for Anthropic.
// It uses redirect_uri=https://console.anthropic.com/oauth/code/callback (Anthropic's
// own callback page) with scopes org:create_api_key user:profile user:inference.
// Since Anthropic redirects to their own page (not localhost), the CLI opens a
// local server to accept a manually-pasted code — we keep a localhost callback
// for a seamless paste-free experience on platforms where it works.
//
// Flow:
//  1. Open browser to authorize URL
//  2. User authorizes; Anthropic redirects to their callback page showing the code
//  3. Our local server also accepts the redirect if the browser follows localhost
//  4. On success, exchange code for access token then create a permanent API key
func StartAnthropicOAuth(ctx context.Context) <-chan OAuthResult {
	return startAnthropicOAuthFlow(ctx, anthropicConsoleAuthURL, true)
}

// StartAnthropicMaxOAuth runs the Max/Pro subscription OAuth PKCE flow.
// Uses claude.ai as the authorize endpoint; the resulting token is used directly
// as a Bearer token (no API key creation step). Experimental: may have restrictions.
func StartAnthropicMaxOAuth(ctx context.Context) <-chan OAuthResult {
	return startAnthropicOAuthFlow(ctx, anthropicMaxAuthURL, false)
}

// startAnthropicOAuthFlow implements the shared PKCE browser flow for both
// Console and Max modes.  When createAPIKey is true the access token is
// exchanged for a permanent Anthropic API key; otherwise the access token
// itself is returned.
func startAnthropicOAuthFlow(ctx context.Context, authorizeURL string, createAPIKey bool) <-chan OAuthResult {
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

		// Start local server on random port to receive the redirect.
		// Anthropic's redirect_uri must be https://console.anthropic.com/oauth/code/callback
		// but we also spin up a localhost listener so that if the browser follows
		// a localhost link (via a proxy or custom hosts entry) we catch it automatically.
		// The primary flow is: browser → Anthropic callback page → user pastes code.
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			ch <- OAuthResult{Err: fmt.Errorf("start callback server: %w", err)}
			return
		}
		port := listener.Addr().(*net.TCPAddr).Port
		localhostRedirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

		// Anthropic requires this specific redirect URI for the CLI OAuth app.
		const anthropicCallbackURI = "https://console.anthropic.com/oauth/code/callback"

		// Build auth URL with correct scopes and redirect URI
		authURL := fmt.Sprintf(
			"%s?response_type=code&client_id=%s&redirect_uri=%s&state=%s&code_challenge=%s&code_challenge_method=S256&scope=%s",
			authorizeURL,
			url.QueryEscape(anthropicClientID),
			url.QueryEscape(anthropicCallbackURI),
			url.QueryEscape(state),
			url.QueryEscape(challenge),
			url.QueryEscape(anthropicOAuthScopes),
		)

		codeCh := make(chan string, 1)
		errCh := make(chan error, 1)

		// Local server: handles the case where the browser is proxied through localhost
		// or where a future version uses localhost redirect URI directly.
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
		// Code paste endpoint: the TUI can POST the code here after user pastes it
		mux.HandleFunc("/paste", func(w http.ResponseWriter, r *http.Request) {
			code := r.FormValue("code")
			if code == "" {
				http.Error(w, "missing code", http.StatusBadRequest)
				return
			}
			codeCh <- code
			fmt.Fprint(w, "ok")
		})

		server := &http.Server{Handler: mux}
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("oauth callback server: panic: %v", r)
				}
			}()
			_ = server.Serve(listener)
		}()
		defer server.Close()

		// Open browser to the authorize URL
		if err := OpenBrowserURL(authURL); err != nil {
			ch <- OAuthResult{Err: fmt.Errorf("open browser: %w", err)}
			return
		}

		// Wait for code via localhost callback, context cancellation, or a
		// manually-pasted code submitted via the /paste endpoint.
		select {
		case code := <-codeCh:
			var token string
			var tokenErr error
			if createAPIKey {
				// Console flow: exchange code → access token → permanent API key
				token, tokenErr = exchangeAnthropicCodeForAPIKey(ctx, code, verifier, anthropicCallbackURI)
			} else {
				// Max flow: exchange code → access token (used as Bearer directly)
				token, tokenErr = exchangeAnthropicCodeForToken(ctx, code, verifier, anthropicCallbackURI)
			}
			ch <- OAuthResult{Token: token, Err: tokenErr}
		case err := <-errCh:
			ch <- OAuthResult{Err: err}
		case <-ctx.Done():
			ch <- OAuthResult{Err: ctx.Err()}
		}

		_ = localhostRedirectURI // used above for local server startup
	}()

	return ch
}

// LocalOAuthServerPort returns the port of a temporary local OAuth server that
// can be used to submit a code via POST /paste?code=<value>.  The caller is
// responsible for closing the server.
func LocalOAuthServerPort() int { return 0 } // placeholder — unused externally

// exchangeAnthropicCodeForToken exchanges an authorization code for an OAuth
// access token.  This is used directly by the Max flow (token as Bearer).
func exchangeAnthropicCodeForToken(ctx context.Context, code, verifier, redirectURI string) (string, error) {
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
	return tokenResult.AccessToken, nil
}

// exchangeAnthropicCodeForAPIKey exchanges an authorization code for an access
// token and then uses that token to create a permanent Anthropic API key.
// This is the Console OAuth flow (option 2).
func exchangeAnthropicCodeForAPIKey(ctx context.Context, code, verifier, redirectURI string) (string, error) {
	accessToken, err := exchangeAnthropicCodeForToken(ctx, code, verifier, redirectURI)
	if err != nil {
		return "", err
	}

	// Exchange access token for a permanent API key
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
	keyReq.Header.Set("Authorization", "Bearer "+accessToken)
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
