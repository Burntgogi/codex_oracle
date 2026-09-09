package oracle

import (
	"strings"
	"testing"
)

func TestBuildChromeHandoffUsesResolvedBrowserSettings(t *testing.T) {
	resolved := DryRunResolved{
		ResolvedEngine: "browser",
		Model:          DefaultBrowserModel,
		Browser: &BrowserResolved{
			ChatGPTURL:   "https://chatgpt.com/",
			ThinkingTime: "extended",
		},
	}

	handoff, err := BuildChromeHandoff(resolved, "# Bundle\n\nReview this plan.", []string{" summarize risk ", "  ", "recommend next step"})
	if err != nil {
		t.Fatalf("BuildChromeHandoff returned error: %v", err)
	}

	if handoff.ChatGPTURL != DefaultChatGPTURL {
		t.Fatalf("ChatGPTURL = %q, want %q", handoff.ChatGPTURL, DefaultChatGPTURL)
	}
	if handoff.Model != DefaultBrowserModel {
		t.Fatalf("Model = %q, want %q", handoff.Model, DefaultBrowserModel)
	}
	if handoff.ModelLabel != "GPT-6 Astra" {
		t.Fatalf("ModelLabel = %q, want GPT-6 Astra", handoff.ModelLabel)
	}
	if handoff.Thinking != "extended" {
		t.Fatalf("Thinking = %q, want extended", handoff.Thinking)
	}
	if handoff.ThinkingLabel != "Pro" {
		t.Fatalf("ThinkingLabel = %q, want Pro", handoff.ThinkingLabel)
	}
	if handoff.HandoffDigest == "" {
		t.Fatal("HandoffDigest is empty")
	}
	if handoff.HandoffNonce == "" {
		t.Fatal("HandoffNonce is empty")
	}
	if handoff.Privacy.TargetHost != "chatgpt.com" {
		t.Fatalf("Privacy.TargetHost = %q, want chatgpt.com", handoff.Privacy.TargetHost)
	}
	if handoff.Privacy.AllowBrowserHistory {
		t.Fatal("AllowBrowserHistory = true, want false")
	}
	if handoff.Privacy.AllowUnrelatedTabs {
		t.Fatal("AllowUnrelatedTabs = true, want false")
	}
	if handoff.Privacy.RequiresUserApprovalForExternalSubmission {
		t.Fatal("RequiresUserApprovalForExternalSubmission = true, want false because plugin invocation authorizes submission")
	}
	if !handoff.Privacy.ExternalSubmissionAuthorizedByInvocation {
		t.Fatal("ExternalSubmissionAuthorizedByInvocation = false, want true")
	}
	if !strings.Contains(handoff.Prompt, "Review this plan.") {
		t.Fatalf("Prompt = %q, want rendered bundle text", handoff.Prompt)
	}
	wantFollowUps := []string{"summarize risk", "recommend next step"}
	if len(handoff.FollowUps) != len(wantFollowUps) {
		t.Fatalf("FollowUps = %#v, want %#v", handoff.FollowUps, wantFollowUps)
	}
	for i := range wantFollowUps {
		if handoff.FollowUps[i] != wantFollowUps[i] {
			t.Fatalf("FollowUps = %#v, want %#v", handoff.FollowUps, wantFollowUps)
		}
	}
	if !handoff.Capture.RequireFreshConversation {
		t.Fatal("RequireFreshConversation = false, want true")
	}
	if !handoff.Capture.PreferMarkdownCopy {
		t.Fatal("PreferMarkdownCopy = false, want true")
	}
}

func TestBuildChromeHandoffCanonicalizesLegacyModelBeforeDigest(t *testing.T) {
	const prompt = "# Bundle\n\nReview this plan."
	resolved := DryRunResolved{
		ResolvedEngine: "browser",
		Model:          LegacyBrowserModel,
		Browser: &BrowserResolved{
			ChatGPTURL:    DefaultChatGPTURL,
			DesiredModel:  "GPT-5.5",
			ThinkingLabel: "Pro 확장",
			ThinkingTime:  "extended",
		},
	}

	handoff, err := BuildChromeHandoff(resolved, prompt, nil)
	if err != nil {
		t.Fatalf("BuildChromeHandoff returned error: %v", err)
	}
	if handoff.Model != DefaultBrowserModel {
		t.Fatalf("Model = %q, want %q", handoff.Model, DefaultBrowserModel)
	}
	if handoff.ModelLabel != DefaultBrowserModelLabel {
		t.Fatalf("ModelLabel = %q, want %q", handoff.ModelLabel, DefaultBrowserModelLabel)
	}
	if handoff.ThinkingLabel != DefaultBrowserThinkingLabel {
		t.Fatalf("ThinkingLabel = %q, want %q", handoff.ThinkingLabel, DefaultBrowserThinkingLabel)
	}
	wantDigest := handoffDigest(DefaultBrowserModel, "extended", prompt)
	if handoff.HandoffDigest != wantDigest {
		t.Fatalf("HandoffDigest = %q, want canonical digest %q", handoff.HandoffDigest, wantDigest)
	}
}

func TestBuildChromeHandoffCanonicalizesGPT55BeforeDigest(t *testing.T) {
	const prompt = "# Bundle\n\nReview this plan."
	resolved := DryRunResolved{
		ResolvedEngine: "browser",
		Model:          " GPT-5.5 ",
		Browser: &BrowserResolved{
			ChatGPTURL:    DefaultChatGPTURL,
			DesiredModel:  "GPT-5.5",
			ThinkingLabel: "Pro 확장",
			ThinkingTime:  "extended",
		},
	}

	handoff, err := BuildChromeHandoff(resolved, prompt, nil)
	if err != nil {
		t.Fatalf("BuildChromeHandoff returned error: %v", err)
	}
	if handoff.Model != DefaultBrowserModel || handoff.ModelLabel != DefaultBrowserModelLabel || handoff.ThinkingLabel != DefaultBrowserThinkingLabel {
		t.Fatalf("handoff = %#v, want canonical GPT-6 Astra + Pro", handoff)
	}
	if handoff.HandoffDigest != handoffDigest(DefaultBrowserModel, "extended", prompt) {
		t.Fatalf("HandoffDigest = %q, want canonical digest", handoff.HandoffDigest)
	}
}

func TestBrowserModelLabelCanonicalizesLegacyModel(t *testing.T) {
	if got := browserModelLabel(" GPT-5.5-PRO "); got != DefaultBrowserModelLabel {
		t.Fatalf("browserModelLabel = %q, want %q", got, DefaultBrowserModelLabel)
	}
}

func TestBuildChromeHandoffIncludesSubmittedFilesInPrivacyManifest(t *testing.T) {
	resolved := DryRunResolved{
		ResolvedEngine: "browser",
		Model:          DefaultBrowserModel,
		Files:          []string{"README.md", "doc/workflow.md"},
		Browser: &BrowserResolved{
			ChatGPTURL:   "https://chatgpt.com/",
			ThinkingTime: "extended",
		},
	}

	handoff, err := BuildChromeHandoff(resolved, "# Bundle\n\nReview this plan.", nil)
	if err != nil {
		t.Fatalf("BuildChromeHandoff returned error: %v", err)
	}

	want := []string{"README.md", "doc/workflow.md"}
	if len(handoff.Privacy.SubmittedFiles) != len(want) {
		t.Fatalf("SubmittedFiles = %#v, want %#v", handoff.Privacy.SubmittedFiles, want)
	}
	for i := range want {
		if handoff.Privacy.SubmittedFiles[i] != want[i] {
			t.Fatalf("SubmittedFiles = %#v, want %#v", handoff.Privacy.SubmittedFiles, want)
		}
	}
}

func TestBuildChromeHandoffRejectsNonHTTPSChatGPTURL(t *testing.T) {
	resolved := DryRunResolved{
		ResolvedEngine: "browser",
		Model:          DefaultBrowserModel,
		Browser: &BrowserResolved{
			ChatGPTURL: "http://chatgpt.com/",
		},
	}

	_, err := BuildChromeHandoff(resolved, "# Bundle\n\nReview this plan.", nil)
	if err == nil {
		t.Fatal("BuildChromeHandoff returned nil error for http ChatGPT URL")
	}
	if !strings.Contains(err.Error(), "https://chatgpt.com") {
		t.Fatalf("error = %q, want https://chatgpt.com guidance", err.Error())
	}
}

func TestBuildChromeHandoffDeclaresExternalChromeDependency(t *testing.T) {
	resolved := DryRunResolved{
		ResolvedEngine: "browser",
		Model:          DefaultBrowserModel,
		Files:          []string{"README.md"},
		Browser: &BrowserResolved{
			ChatGPTURL:   "https://chatgpt.com/",
			ThinkingTime: "extended",
		},
	}

	handoff, err := BuildChromeHandoff(resolved, "# Bundle\n\nReview this plan.", nil)
	if err != nil {
		t.Fatalf("BuildChromeHandoff returned error: %v", err)
	}
	if handoff.Orchestration.Mode != "external-chrome-plugin" {
		t.Fatalf("Orchestration.Mode = %q, want external-chrome-plugin", handoff.Orchestration.Mode)
	}
	if handoff.Orchestration.RequiredPlugin != "chrome@openai-bundled" {
		t.Fatalf("Orchestration.RequiredPlugin = %q, want chrome@openai-bundled", handoff.Orchestration.RequiredPlugin)
	}
	if !handoff.Orchestration.RequiresSignedInUserSession {
		t.Fatal("RequiresSignedInUserSession = false, want true")
	}
	if handoff.Orchestration.OwnsBrowserAutomation {
		t.Fatal("OwnsBrowserAutomation = true, want false")
	}
	if !handoff.Orchestration.AllowAutomaticSubmission {
		t.Fatal("AllowAutomaticSubmission = false, want true")
	}
	if handoff.Orchestration.RequiresSeparateSendConfirmation {
		t.Fatal("RequiresSeparateSendConfirmation = true, want false")
	}
	if handoff.Orchestration.AutomationOwner != "codex-agent-via-chrome-plugin" {
		t.Fatalf("AutomationOwner = %q, want codex-agent-via-chrome-plugin", handoff.Orchestration.AutomationOwner)
	}
}

func TestBuildChromeHandoffUsesAttachmentSubmissionPrompt(t *testing.T) {
	resolved := DryRunResolved{
		ResolvedEngine: "browser",
		Model:          DefaultBrowserModel,
		Files:          []string{"README.md", "doc/workflow.md"},
		Browser: &BrowserResolved{
			ChatGPTURL:   "https://chatgpt.com/",
			ThinkingTime: "extended",
		},
	}
	bundleText := "# Oracle Consult Bundle\n\n## Prompt\n\nReview the attached files.\n\n## Files\n\n```md\ncontent\n```"

	handoff, err := BuildChromeHandoff(resolved, bundleText, nil, ChromeHandoffOptions{AttachmentMode: "text"})
	if err != nil {
		t.Fatalf("BuildChromeHandoff returned error: %v", err)
	}

	if handoff.Submission.Mode != "pasted-text-attachment" {
		t.Fatalf("Submission.Mode = %q, want pasted-text-attachment", handoff.Submission.Mode)
	}
	if handoff.Submission.ContextSource != "handoff.prompt" {
		t.Fatalf("Submission.ContextSource = %q, want handoff.prompt", handoff.Submission.ContextSource)
	}
	if strings.Contains(handoff.Submission.PromptText, "```") {
		t.Fatalf("Submission.PromptText contains code fences; want short instruction: %q", handoff.Submission.PromptText)
	}
	if !strings.Contains(handoff.Submission.PromptText, "Oracle Consult Bundle") {
		t.Fatalf("Submission.PromptText = %q, want Oracle bundle instruction", handoff.Submission.PromptText)
	}
	if !handoff.Submission.ExternalSubmissionAuthorizedByInvocation {
		t.Fatal("Submission.ExternalSubmissionAuthorizedByInvocation = false, want true")
	}
	if handoff.Submission.RequiresSeparateSendConfirmation {
		t.Fatal("Submission.RequiresSeparateSendConfirmation = true, want false")
	}
	if !handoff.Submission.StopOnHumanVerification {
		t.Fatal("Submission.StopOnHumanVerification = false, want true")
	}
	if handoff.Prompt != bundleText {
		t.Fatal("Handoff.Prompt should retain canonical full bundle text")
	}
}

func TestBuildChromeHandoffRejectsEmptyBundle(t *testing.T) {
	_, err := BuildChromeHandoff(DryRunResolved{ResolvedEngine: "browser", Model: DefaultBrowserModel}, "  ", nil)
	if err == nil {
		t.Fatal("BuildChromeHandoff returned nil error for empty bundle")
	}
}
