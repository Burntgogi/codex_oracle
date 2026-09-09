package browser_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"codex_oracle/internal/browser"
)

type sequenceChatGPT struct {
	auth        browser.AuthStatus
	evidence    browser.ModelEvidence
	capture     []string
	prompts     []string
	submitCalls int
	uploadFiles []string
	actions     []string
}

type completionAwareChatGPT struct {
	sequenceChatGPT
	completion []browser.CompletionStatus
}

func (s *sequenceChatGPT) ProbeAuth(context.Context) (browser.AuthStatus, error) {
	return s.auth, nil
}

func (s *sequenceChatGPT) ConfigureModel(context.Context, browser.ModelSelection) (browser.ModelEvidence, error) {
	return s.evidence, nil
}

func (s *sequenceChatGPT) SubmitPrompt(_ context.Context, prompt string) error {
	s.submitCalls++
	s.prompts = append(s.prompts, prompt)
	s.actions = append(s.actions, "submit")
	return nil
}

func (s *sequenceChatGPT) UploadFiles(_ context.Context, paths []string) error {
	s.uploadFiles = append([]string{}, paths...)
	s.actions = append(s.actions, "upload")
	return nil
}

func (s *sequenceChatGPT) CaptureLatestAssistantText(context.Context) (string, error) {
	if len(s.capture) == 0 {
		return "", errors.New("no assistant text found")
	}
	value := s.capture[0]
	s.capture = s.capture[1:]
	if value == "" {
		return "", errors.New("no assistant text found")
	}
	return value, nil
}

func (s *completionAwareChatGPT) ProbeCompletion(context.Context) (browser.CompletionStatus, error) {
	if len(s.completion) == 0 {
		return browser.CompletionStatus{InProgress: false, Reason: "default-complete"}, nil
	}
	value := s.completion[0]
	s.completion = s.completion[1:]
	return value, nil
}

func TestWaitForStableAnswerRequiresRepeatedSameText(t *testing.T) {
	client := &sequenceChatGPT{
		auth:    browser.AuthStatus{Authenticated: true, ComposerVisible: true},
		capture: []string{"draft", "final", "final"},
	}

	text, err := browser.WaitForStableAnswer(context.Background(), client, browser.WaitOptions{
		PollInterval:  time.Millisecond,
		StableSamples: 2,
	})
	if err != nil {
		t.Fatalf("WaitForStableAnswer returned error: %v", err)
	}
	if text != "final" {
		t.Fatalf("text = %q, want final", text)
	}
}

func TestWaitForStableAnswerTimesOutWithoutText(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	client := &sequenceChatGPT{
		auth:    browser.AuthStatus{Authenticated: true, ComposerVisible: true},
		capture: []string{"", "", ""},
	}

	_, err := browser.WaitForStableAnswer(ctx, client, browser.WaitOptions{
		PollInterval:  time.Millisecond,
		StableSamples: 2,
	})
	if err == nil {
		t.Fatal("WaitForStableAnswer returned nil error, want timeout")
	}
}

func TestWaitForStableAnswerWaitsForCompletionSignal(t *testing.T) {
	client := &completionAwareChatGPT{
		sequenceChatGPT: sequenceChatGPT{
			auth:    browser.AuthStatus{Authenticated: true, ComposerVisible: true},
			capture: []string{"draft", "draft", "final", "final"},
		},
		completion: []browser.CompletionStatus{
			{InProgress: true, Reason: "stop-button-visible"},
			{InProgress: true, Reason: "stop-button-visible"},
			{InProgress: false, Ready: true, Reason: "send-button-ready"},
			{InProgress: false, Ready: true, Reason: "send-button-ready"},
		},
	}

	text, err := browser.WaitForStableAnswer(context.Background(), client, browser.WaitOptions{
		PollInterval:  time.Millisecond,
		StableSamples: 2,
	})
	if err != nil {
		t.Fatalf("WaitForStableAnswer returned error: %v", err)
	}
	if text != "final" {
		t.Fatalf("text = %q, want final", text)
	}
}

func TestPrepareAndSubmitFailsBeforePromptWhenProExtendedUnconfirmed(t *testing.T) {
	client := &sequenceChatGPT{
		auth: browser.AuthStatus{Authenticated: true, ComposerVisible: true},
		evidence: browser.ModelEvidence{
			Status:        "current",
			ObservedLabel: "GPT-5.5 Pro",
			ThinkingTime:  "standard",
			Confirmed:     true,
		},
		capture: []string{"should not be used"},
	}

	_, err := browser.PrepareSubmitAndCapture(context.Background(), client, browser.Request{
		Prompt:               "review",
		BundleText:           "bundle",
		Model:                "gpt-5.5-pro",
		DesiredModelLabel:    "GPT-5.5 Pro",
		BrowserThinkingTime:  "extended",
		BrowserModelStrategy: "select",
	}, browser.WaitOptions{PollInterval: time.Millisecond, StableSamples: 1})
	if err == nil {
		t.Fatal("PrepareSubmitAndCapture returned nil error, want model verification failure")
	}
	if client.submitCalls != 0 {
		t.Fatalf("submitCalls = %d, want 0", client.submitCalls)
	}
}

func TestPrepareAndSubmitUploadsAttachmentsBeforePrompt(t *testing.T) {
	client := &sequenceChatGPT{
		auth: browser.AuthStatus{Authenticated: true, ComposerVisible: true},
		evidence: browser.ModelEvidence{
			Status:        "selected",
			ObservedLabel: "GPT-6 Astra",
			ThinkingTime:  "extended",
			ProConfirmed:  true,
			Confirmed:     true,
		},
		capture: []string{"final", "final"},
	}

	text, err := browser.PrepareSubmitAndCapture(context.Background(), client, browser.Request{
		Prompt:               "review attached context",
		Model:                "gpt-5.5-pro",
		DesiredModelLabel:    "GPT-6 Astra",
		BrowserThinkingTime:  "extended",
		BrowserModelStrategy: "select",
		AttachmentPaths:      []string{"C:\\tmp\\context.zip"},
	}, browser.WaitOptions{PollInterval: time.Millisecond, StableSamples: 2})
	if err != nil {
		t.Fatalf("PrepareSubmitAndCapture returned error: %v", err)
	}
	if text != "final" {
		t.Fatalf("text = %q, want final", text)
	}
	if len(client.uploadFiles) != 1 || client.uploadFiles[0] != "C:\\tmp\\context.zip" {
		t.Fatalf("uploadFiles = %#v, want context.zip", client.uploadFiles)
	}
	if len(client.actions) < 2 || client.actions[0] != "upload" || client.actions[1] != "submit" {
		t.Fatalf("actions = %#v, want upload before submit", client.actions)
	}
}

func TestPrepareAndSubmitSendsFollowUpsAfterInitialAnswer(t *testing.T) {
	client := &sequenceChatGPT{
		auth: browser.AuthStatus{Authenticated: true, ComposerVisible: true},
		evidence: browser.ModelEvidence{
			Status:        "selected",
			ObservedLabel: "GPT-6 Astra",
			ThinkingTime:  "extended",
			ProConfirmed:  true,
			Confirmed:     true,
		},
		capture: []string{"initial answer", "follow-up answer"},
	}

	text, err := browser.PrepareSubmitAndCapture(context.Background(), client, browser.Request{
		Prompt:               "review",
		BundleText:           "bundle",
		FollowUps:            []string{"summarize the risk", "   "},
		Model:                "gpt-5.5-pro",
		DesiredModelLabel:    "GPT-6 Astra",
		BrowserThinkingTime:  "extended",
		BrowserModelStrategy: "select",
	}, browser.WaitOptions{PollInterval: time.Millisecond, StableSamples: 1})
	if err != nil {
		t.Fatalf("PrepareSubmitAndCapture returned error: %v", err)
	}
	if text != "follow-up answer" {
		t.Fatalf("text = %q, want final follow-up answer", text)
	}
	wantPrompts := []string{"bundle", "summarize the risk"}
	if len(client.prompts) != len(wantPrompts) {
		t.Fatalf("prompts = %#v, want %#v", client.prompts, wantPrompts)
	}
	for i := range wantPrompts {
		if client.prompts[i] != wantPrompts[i] {
			t.Fatalf("prompts = %#v, want %#v", client.prompts, wantPrompts)
		}
	}
}
