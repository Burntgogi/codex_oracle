package browser

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

var ErrLoginRequired = errors.New("ChatGPT login required in the private browser profile")

type ChatGPTAutomation interface {
	ProbeAuth(ctx context.Context) (AuthStatus, error)
	ConfigureModel(ctx context.Context, selection ModelSelection) (ModelEvidence, error)
	UploadFiles(ctx context.Context, paths []string) error
	SubmitPrompt(ctx context.Context, prompt string) error
	CaptureLatestAssistantText(ctx context.Context) (string, error)
}

type CompletionProbe interface {
	ProbeCompletion(ctx context.Context) (CompletionStatus, error)
}

type WaitOptions struct {
	PollInterval  time.Duration
	StableSamples int
}

type CDPRunner struct {
	Env          map[string]string
	Timeout      time.Duration
	PollInterval time.Duration
}

func NewCDPRunner(env map[string]string) *CDPRunner {
	return &CDPRunner{
		Env:          copyStringMap(env),
		Timeout:      10 * time.Minute,
		PollInterval: 2 * time.Second,
	}
}

func (r *CDPRunner) Run(ctx context.Context, request Request) (Result, error) {
	env := r.Env
	if env == nil {
		env = map[string]string{}
	}
	chromePath, err := FindChromePath(env)
	if err != nil {
		return Result{}, err
	}
	profileDir := strings.TrimSpace(request.ProfileDir)
	if profileDir == "" {
		profileDir = strings.TrimSpace(env["ORACLE_BROWSER_PROFILE_DIR"])
	}
	if profileDir == "" {
		profileDir = filepath.Join(defaultOracleHome(env), "browser-profile")
	}
	chatgptURL := strings.TrimSpace(request.ChatGPTURL)
	if chatgptURL == "" {
		chatgptURL = strings.TrimSpace(env["ORACLE_CHATGPT_URL"])
	}
	if chatgptURL == "" {
		chatgptURL = "https://chatgpt.com/"
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	process, endpoint, err := LaunchChromeAndWait(runCtx, chromePath, LaunchConfig{
		ProfileDir:  profileDir,
		ChatGPTURL:  chatgptURL,
		KeepBrowser: request.KeepBrowser,
	}, 100*time.Millisecond)
	if err != nil {
		return Result{}, err
	}
	if !request.KeepBrowser {
		defer process.Kill()
	}
	cdp, err := DialCDP(runCtx, endpoint)
	if err != nil {
		return Result{}, err
	}
	defer cdp.Close()
	page, err := OpenPage(runCtx, cdp, chatgptURL)
	if err != nil {
		return Result{}, err
	}
	client := NewChatGPTClient(page)
	auth, err := client.ProbeAuth(runCtx)
	if err != nil {
		return Result{}, err
	}
	if !auth.Authenticated || !auth.ComposerVisible {
		return Result{}, fmt.Errorf("%w: run login_setup and sign into ChatGPT at %s", ErrLoginRequired, profileDir)
	}
	answer, err := PrepareSubmitAndCapture(runCtx, client, request, WaitOptions{
		PollInterval:  r.PollInterval,
		StableSamples: 2,
	})
	if err != nil {
		return Result{}, err
	}
	conversationURL := chatgptURL
	if currentURL, err := page.CurrentURL(runCtx); err == nil && strings.TrimSpace(currentURL) != "" {
		conversationURL = strings.TrimSpace(currentURL)
	}
	return Result{
		Answer:     answer,
		Transcript: answer,
		Runtime: runtimeMetadata(runtimeMetadataInput{
			PID:             process.PID(),
			Endpoint:        endpoint,
			TargetID:        page.TargetID,
			ConversationURL: conversationURL,
			ProfileDir:      profileDir,
			ChatGPTURL:      chatgptURL,
		}),
	}, nil
}

func WaitForStableAnswer(ctx context.Context, client ChatGPTAutomation, options WaitOptions) (string, error) {
	pollInterval := options.PollInterval
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	stableSamples := options.StableSamples
	if stableSamples <= 0 {
		stableSamples = 2
	}
	var last string
	stable := 0
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		inProgress := false
		if probe, ok := client.(CompletionProbe); ok {
			if status, err := probe.ProbeCompletion(ctx); err == nil {
				inProgress = status.InProgress
			}
		}
		text, err := client.CaptureLatestAssistantText(ctx)
		if err == nil && strings.TrimSpace(text) != "" {
			if text == last {
				stable++
			} else {
				last = text
				stable = 1
			}
			if stable >= stableSamples && !inProgress {
				return text, nil
			}
		}
		select {
		case <-ctx.Done():
			if last != "" {
				return "", fmt.Errorf("assistant response did not stabilize before timeout; last capture length=%d: %w", len(last), ctx.Err())
			}
			return "", fmt.Errorf("assistant response was not captured before timeout: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

func PrepareSubmitAndCapture(ctx context.Context, client ChatGPTAutomation, request Request, waitOptions WaitOptions) (string, error) {
	selection := ModelSelection{
		Model:        request.Model,
		DesiredLabel: request.DesiredModelLabel,
		Strategy:     request.BrowserModelStrategy,
		ThinkingTime: request.BrowserThinkingTime,
	}
	evidence, err := client.ConfigureModel(ctx, selection)
	if err != nil {
		return "", err
	}
	if err := VerifyModelEvidence(selection, evidence); err != nil {
		return "", err
	}
	if len(request.AttachmentPaths) > 0 {
		if err := client.UploadFiles(ctx, request.AttachmentPaths); err != nil {
			return "", err
		}
	}
	submitText := request.BundleText
	if strings.TrimSpace(submitText) == "" {
		submitText = request.Prompt
	}
	if err := client.SubmitPrompt(ctx, submitText); err != nil {
		return "", err
	}
	answer, err := WaitForStableAnswer(ctx, client, waitOptions)
	if err != nil {
		return "", err
	}
	for _, followUp := range normalizedFollowUps(request.FollowUps) {
		if err := client.SubmitPrompt(ctx, followUp); err != nil {
			return "", err
		}
		answer, err = WaitForStableAnswer(ctx, client, waitOptions)
		if err != nil {
			return "", err
		}
	}
	return answer, nil
}

func normalizedFollowUps(followUps []string) []string {
	normalized := make([]string, 0, len(followUps))
	for _, followUp := range followUps {
		trimmed := strings.TrimSpace(followUp)
		if trimmed != "" {
			normalized = append(normalized, trimmed)
		}
	}
	return normalized
}

func defaultOracleHome(env map[string]string) string {
	if value := strings.TrimSpace(env["ORACLE_HOME_DIR"]); value != "" {
		return value
	}
	return filepath.Join(".", ".codex_oracle")
}

func copyStringMap(input map[string]string) map[string]string {
	output := map[string]string{}
	for key, value := range input {
		output[key] = value
	}
	return output
}
