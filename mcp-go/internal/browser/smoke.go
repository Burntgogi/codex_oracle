package browser

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type CDPSmokeChecker struct{}

func NewCDPSmokeChecker() *CDPSmokeChecker {
	return &CDPSmokeChecker{}
}

func (c *CDPSmokeChecker) Check(ctx context.Context, request SmokeRequest) (SmokeResult, error) {
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	result := SmokeResult{
		ProfileDir: strings.TrimSpace(request.ProfileDir),
		ChatGPTURL: strings.TrimSpace(request.ChatGPTURL),
	}
	if result.ChatGPTURL == "" {
		result.ChatGPTURL = "https://chatgpt.com/"
	}
	endpoint := strings.TrimSpace(request.Endpoint)
	if endpoint == "" {
		if result.ProfileDir == "" {
			result.Checks = append(result.Checks, SmokeCheck{Name: "devtools-endpoint", Status: "fail", Message: "profileDir or endpoint is required"})
			return result, fmt.Errorf("profileDir or endpoint is required")
		}
		var err error
		endpoint, err = ReadDevToolsEndpoint(result.ProfileDir)
		if err != nil {
			result.Checks = append(result.Checks, SmokeCheck{Name: "devtools-endpoint", Status: "fail", Message: err.Error()})
			return result, err
		}
	}
	result.Endpoint = endpoint
	result.Checks = append(result.Checks, SmokeCheck{Name: "devtools-endpoint", Status: "ok"})

	cdp, err := DialCDP(runCtx, endpoint)
	if err != nil {
		result.Checks = append(result.Checks, SmokeCheck{Name: "cdp", Status: "fail", Message: err.Error()})
		return result, err
	}
	defer cdp.Close()
	result.Checks = append(result.Checks, SmokeCheck{Name: "cdp", Status: "ok"})

	page, err := OpenPage(runCtx, cdp, result.ChatGPTURL)
	if err != nil {
		result.Checks = append(result.Checks, SmokeCheck{Name: "chatgpt-page", Status: "fail", Message: err.Error()})
		return result, err
	}
	result.Checks = append(result.Checks, SmokeCheck{Name: "chatgpt-page", Status: "ok"})

	auth, err := NewChatGPTClient(page).ProbeAuth(runCtx)
	if err != nil {
		result.Checks = append(result.Checks, SmokeCheck{Name: "auth", Status: "fail", Message: err.Error()})
		return result, err
	}
	result.Auth = auth
	authStatus := "login-required"
	if auth.Authenticated && auth.ComposerVisible {
		authStatus = "ok"
	}
	result.Checks = append(result.Checks, SmokeCheck{Name: "auth", Status: authStatus, Message: auth.Reason})
	return result, nil
}
