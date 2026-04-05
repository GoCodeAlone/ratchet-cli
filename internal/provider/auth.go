package providerauth

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/term"
)

// PromptAPIKey prompts the user for an API key (hidden input).
func PromptAPIKey(providerType string) (string, error) {
	fmt.Printf("Enter %s API key: ", providerType)
	key, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(key)), nil
}

// PromptBaseURL prompts for a base URL with a default value.
func PromptBaseURL(defaultURL string) (string, error) {
	fmt.Printf("Base URL [%s]: ", defaultURL)
	reader := bufio.NewReader(os.Stdin)
	url, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	url = strings.TrimSpace(url)
	if url == "" {
		return defaultURL, nil
	}
	return url, nil
}

// DeviceFlow implements GitHub's device flow for Copilot auth.
func DeviceFlow(ctx context.Context) (string, error) {
	deviceResp, err := StartGitHubDeviceFlow(ctx, GithubCopilotClientID)
	if err != nil {
		return "", fmt.Errorf("start device flow: %w", err)
	}
	fmt.Printf("Open %s and enter code: %s\n", deviceResp.VerificationURI, deviceResp.UserCode)
	result := <-PollGitHubDeviceFlow(ctx, GithubCopilotClientID, deviceResp.DeviceCode, deviceResp.Interval)
	if result.Err != nil {
		return "", fmt.Errorf("poll device flow: %w", result.Err)
	}
	return result.Token, nil
}
