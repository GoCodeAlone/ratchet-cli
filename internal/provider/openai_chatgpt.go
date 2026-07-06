package providerauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	OpenAIChatGPTClientID = "app_EMoamEEZ73f0CkXaXp7hrann"
	openAIChatGPTIssuer   = "https://auth.openai.com"
)

type OpenAIChatGPTTokenBundle struct {
	IDToken                 string `json:"id_token,omitempty"`
	AccessToken             string `json:"access_token"`
	RefreshToken            string `json:"refresh_token"`
	AccountID               string `json:"account_id,omitempty"`
	ChatGPTAccountID        string `json:"chatgpt_account_id,omitempty"`
	ChatGPTAccountIsFedRAMP bool   `json:"chatgpt_account_is_fedramp,omitempty"`
	ChatGPTUserID           string `json:"chatgpt_user_id,omitempty"`
	PlanType                string `json:"chatgpt_plan_type,omitempty"`
}

type openAIChatGPTDeviceClient struct {
	httpClient *http.Client
	issuer     string
}

func newOpenAIChatGPTDeviceClient(httpClient *http.Client, issuer string) openAIChatGPTDeviceClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if issuer == "" {
		issuer = openAIChatGPTIssuer
	}
	return openAIChatGPTDeviceClient{
		httpClient: httpClient,
		issuer:     strings.TrimRight(issuer, "/"),
	}
}

// StartOpenAIChatGPTDeviceFlow starts the official Codex ChatGPT device-code flow.
func StartOpenAIChatGPTDeviceFlow(ctx context.Context) (*DeviceCodeResult, error) {
	c := newOpenAIChatGPTDeviceClient(http.DefaultClient, openAIChatGPTIssuer)
	return c.start(ctx)
}

// PollOpenAIChatGPTDeviceFlow polls until the device code is approved and returns
// a JSON token bundle suitable for the workflow-plugin-agent openai_chatgpt provider.
func PollOpenAIChatGPTDeviceFlow(ctx context.Context, deviceAuthID, userCode string, interval int) <-chan OAuthResult {
	c := newOpenAIChatGPTDeviceClient(http.DefaultClient, openAIChatGPTIssuer)
	return c.poll(ctx, deviceAuthID, userCode, interval)
}

// LoadCodexAuth imports a Codex CLI auth.json file and returns a compact token
// bundle JSON string for ratchet secret storage.
func LoadCodexAuth(path string) (string, error) {
	if path == "" {
		path = DefaultCodexAuthPath()
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Codex auth file: %w", err)
	}
	var wrapper struct {
		Tokens OpenAIChatGPTTokenBundle `json:"tokens"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return "", fmt.Errorf("parse Codex auth file: %w", err)
	}
	if wrapper.Tokens.AccessToken == "" && wrapper.Tokens.RefreshToken == "" {
		return "", fmt.Errorf("Codex auth file does not contain ChatGPT tokens")
	}
	return marshalOpenAIChatGPTTokenBundle(wrapper.Tokens)
}

func DefaultCodexAuthPath() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".codex", "auth.json")
	}
	return filepath.Join(".codex", "auth.json")
}

func (c openAIChatGPTDeviceClient) start(ctx context.Context) (*DeviceCodeResult, error) {
	payload, err := json.Marshal(map[string]string{"client_id": OpenAIChatGPTClientID})
	if err != nil {
		return nil, fmt.Errorf("marshal device code request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer+"/api/accounts/deviceauth/usercode", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create device code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request device code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("device code request failed (%d): %s", resp.StatusCode, truncateOpenAIChatGPTBody(body))
	}

	var result struct {
		DeviceAuthID string      `json:"device_auth_id"`
		UserCode     string      `json:"user_code"`
		UserCodeAlt  string      `json:"usercode"`
		Interval     flexibleInt `json:"interval"`
		ExpiresIn    flexibleInt `json:"expires_in"`
		Error        string      `json:"error"`
		ErrorDesc    string      `json:"error_description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse device code response: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("%s: %s", result.Error, result.ErrorDesc)
	}
	userCode := result.UserCode
	if userCode == "" {
		userCode = result.UserCodeAlt
	}
	if result.DeviceAuthID == "" || userCode == "" {
		return nil, fmt.Errorf("device code response missing device_auth_id or user_code")
	}
	interval := int(result.Interval)
	if interval <= 0 {
		interval = 5
	}
	expires := int(result.ExpiresIn)
	if expires <= 0 {
		expires = 15 * 60
	}
	return &DeviceCodeResult{
		DeviceCode:      result.DeviceAuthID,
		UserCode:        userCode,
		VerificationURI: c.issuer + "/codex/device",
		ExpiresIn:       expires,
		Interval:        interval,
	}, nil
}

func (c openAIChatGPTDeviceClient) poll(ctx context.Context, deviceAuthID, userCode string, interval int) <-chan OAuthResult {
	ch := make(chan OAuthResult, 1)
	if interval <= 0 {
		interval = 5
	}
	go func() {
		defer close(ch)
		ticker := time.NewTicker(time.Duration(interval) * time.Second)
		defer ticker.Stop()
		for {
			token, done, err := c.pollOnce(ctx, deviceAuthID, userCode)
			if err != nil {
				ch <- OAuthResult{Err: err}
				return
			}
			if done {
				ch <- OAuthResult{Token: token}
				return
			}
			select {
			case <-ctx.Done():
				ch <- OAuthResult{Err: ctx.Err()}
				return
			case <-ticker.C:
			}
		}
	}()
	return ch
}

func (c openAIChatGPTDeviceClient) pollOnce(ctx context.Context, deviceAuthID, userCode string) (token string, done bool, err error) {
	payload, err := json.Marshal(map[string]string{
		"device_auth_id": deviceAuthID,
		"user_code":      userCode,
	})
	if err != nil {
		return "", false, fmt.Errorf("marshal device token request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer+"/api/accounts/deviceauth/token", bytes.NewReader(payload))
	if err != nil {
		return "", false, fmt.Errorf("create device token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("poll device token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return "", false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", false, fmt.Errorf("device token request failed (%d): %s", resp.StatusCode, truncateOpenAIChatGPTBody(body))
	}

	var result struct {
		AuthorizationCode string `json:"authorization_code"`
		CodeChallenge     string `json:"code_challenge"`
		CodeVerifier      string `json:"code_verifier"`
		Error             string `json:"error"`
		ErrorDesc         string `json:"error_description"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return "", false, fmt.Errorf("parse device token response: %w", err)
	}
	if result.Error != "" {
		if result.Error == "authorization_pending" || result.Error == "slow_down" {
			return "", false, nil
		}
		return "", false, fmt.Errorf("%s: %s", result.Error, result.ErrorDesc)
	}
	if result.AuthorizationCode == "" || result.CodeVerifier == "" {
		return "", false, fmt.Errorf("device token response missing authorization_code or code_verifier")
	}
	bundle, err := c.exchangeAuthorizationCode(ctx, result.AuthorizationCode, result.CodeVerifier)
	if err != nil {
		return "", false, err
	}
	return bundle, true, nil
}

func (c openAIChatGPTDeviceClient) exchangeAuthorizationCode(ctx context.Context, code, verifier string) (string, error) {
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {c.issuer + "/deviceauth/callback"},
		"client_id":     {OpenAIChatGPTClientID},
		"code_verifier": {verifier},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.issuer+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchange authorization code: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("authorization code exchange failed (%d): %s", resp.StatusCode, truncateOpenAIChatGPTBody(body))
	}

	var tokens OpenAIChatGPTTokenBundle
	if err := json.Unmarshal(body, &tokens); err != nil {
		return "", fmt.Errorf("parse token exchange response: %w", err)
	}
	if tokens.AccessToken == "" || tokens.RefreshToken == "" {
		return "", fmt.Errorf("token exchange response missing access_token or refresh_token")
	}
	return marshalOpenAIChatGPTTokenBundle(tokens)
}

func marshalOpenAIChatGPTTokenBundle(tokens OpenAIChatGPTTokenBundle) (string, error) {
	raw, err := json.Marshal(tokens)
	if err != nil {
		return "", fmt.Errorf("marshal ChatGPT token bundle: %w", err)
	}
	return string(raw), nil
}

type flexibleInt int

func (f *flexibleInt) UnmarshalJSON(raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		*f = 0
		return nil
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return err
		}
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return err
		}
		*f = flexibleInt(n)
		return nil
	}
	var n int
	if err := json.Unmarshal(raw, &n); err != nil {
		return err
	}
	*f = flexibleInt(n)
	return nil
}

func truncateOpenAIChatGPTBody(body []byte) string {
	const max = 300
	msg := strings.TrimSpace(string(body))
	if len(msg) <= max {
		return msg
	}
	return msg[:max] + "..."
}
