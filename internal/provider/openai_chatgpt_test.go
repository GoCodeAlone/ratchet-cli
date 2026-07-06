package providerauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCodexAuth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(`{
		"auth_mode": "chatgpt",
		"tokens": {
			"id_token": "id-token",
			"access_token": "access-token",
			"refresh_token": "refresh-token",
			"account_id": "acct_123"
		}
	}`), 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}

	got, err := LoadCodexAuth(path)
	if err != nil {
		t.Fatalf("LoadCodexAuth: %v", err)
	}
	var tokens OpenAIChatGPTTokenBundle
	if err := json.Unmarshal([]byte(got), &tokens); err != nil {
		t.Fatalf("unmarshal token bundle: %v", err)
	}
	if tokens.AccessToken != "access-token" || tokens.RefreshToken != "refresh-token" || tokens.AccountID != "acct_123" {
		t.Fatalf("tokens = %+v", tokens)
	}
}

func TestOpenAIChatGPTDeviceClientFlow(t *testing.T) {
	var sawUserCodeReq, sawPollReq, sawExchangeReq bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/accounts/deviceauth/usercode":
			sawUserCodeReq = true
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode usercode request: %v", err)
			}
			if req["client_id"] != OpenAIChatGPTClientID {
				t.Fatalf("client_id = %q", req["client_id"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"device_auth_id":"dev-123","usercode":"USER-CODE","interval":"1"}`))
		case "/api/accounts/deviceauth/token":
			sawPollReq = true
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode poll request: %v", err)
			}
			if req["device_auth_id"] != "dev-123" || req["user_code"] != "USER-CODE" {
				t.Fatalf("poll request = %#v", req)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"authorization_code":"auth-code","code_challenge":"challenge","code_verifier":"verifier"}`))
		case "/oauth/token":
			sawExchangeReq = true
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse form: %v", err)
			}
			want := url.Values{
				"grant_type":    {"authorization_code"},
				"code":          {"auth-code"},
				"redirect_uri":  {serverURL(r) + "/deviceauth/callback"},
				"client_id":     {OpenAIChatGPTClientID},
				"code_verifier": {"verifier"},
			}
			for key, values := range want {
				if got := r.Form.Get(key); got != values[0] {
					t.Fatalf("form[%s] = %q, want %q", key, got, values[0])
				}
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id_token":"id-token","access_token":"access-token","refresh_token":"refresh-token"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := newOpenAIChatGPTDeviceClient(server.Client(), server.URL)
	device, err := client.start(context.Background())
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if device.DeviceCode != "dev-123" || device.UserCode != "USER-CODE" || device.Interval != 1 {
		t.Fatalf("device = %+v", device)
	}
	if device.VerificationURI != server.URL+"/codex/device" {
		t.Fatalf("VerificationURI = %q", device.VerificationURI)
	}

	token, done, err := client.pollOnce(context.Background(), device.DeviceCode, device.UserCode)
	if err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if !done {
		t.Fatal("pollOnce did not complete")
	}
	var tokens OpenAIChatGPTTokenBundle
	if err := json.Unmarshal([]byte(token), &tokens); err != nil {
		t.Fatalf("unmarshal token: %v", err)
	}
	if tokens.AccessToken != "access-token" || tokens.RefreshToken != "refresh-token" {
		t.Fatalf("tokens = %+v", tokens)
	}
	if !sawUserCodeReq || !sawPollReq || !sawExchangeReq {
		t.Fatalf("requests seen: usercode=%v poll=%v exchange=%v", sawUserCodeReq, sawPollReq, sawExchangeReq)
	}
}

func TestOpenAIChatGPTDeviceClientPollPending(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "pending", http.StatusForbidden)
	}))
	defer server.Close()

	client := newOpenAIChatGPTDeviceClient(server.Client(), server.URL)
	token, done, err := client.pollOnce(context.Background(), "dev", "code")
	if err != nil {
		t.Fatalf("pollOnce: %v", err)
	}
	if done || token != "" {
		t.Fatalf("pollOnce token=%q done=%v, want pending", token, done)
	}
}

func TestFlexibleInt(t *testing.T) {
	var got struct {
		A flexibleInt `json:"a"`
		B flexibleInt `json:"b"`
	}
	if err := json.Unmarshal([]byte(`{"a":"7","b":3}`), &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.A != 7 || got.B != 3 {
		t.Fatalf("got %+v", got)
	}
}

func serverURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
