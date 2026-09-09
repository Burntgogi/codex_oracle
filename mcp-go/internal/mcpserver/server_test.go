package mcpserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"codex_oracle/internal/browser"
	"codex_oracle/internal/mcpserver"
	"codex_oracle/internal/session"
)

type fakeRunner struct{}

func (fakeRunner) Run(_ context.Context, request browser.Request) (browser.Result, error) {
	return browser.Result{
		Answer:     "fake answer for " + request.Model,
		Transcript: "transcript",
		Runtime:    map[string]any{"fake": true},
	}, nil
}

type recordingRunner struct {
	request browser.Request
}

func (r *recordingRunner) Run(_ context.Context, request browser.Request) (browser.Result, error) {
	r.request = request
	return browser.Result{
		Answer:     "answer with attachments",
		Transcript: "transcript",
		Runtime:    map[string]any{"fake": true},
	}, nil
}

type inspectingRunner struct {
	store      *session.Store
	sawRunning bool
}

func (r *inspectingRunner) Run(_ context.Context, request browser.Request) (browser.Result, error) {
	entries, err := r.store.List(session.ListOptions{IncludeAll: true})
	if err == nil && len(entries) == 1 && entries[0].Status == "running" {
		r.sawRunning = true
	}
	return browser.Result{
		Answer:     "inspected answer",
		Transcript: "transcript",
		Runtime:    map[string]any{"fake": true},
	}, nil
}

type waitingRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r *waitingRunner) Run(_ context.Context, request browser.Request) (browser.Result, error) {
	close(r.started)
	<-r.release
	return browser.Result{
		Answer:     "waited answer",
		Transcript: "transcript",
		Runtime:    map[string]any{"fake": true},
	}, nil
}

type fakeReattacher struct {
	request browser.ReattachRequest
}

func (f *fakeReattacher) Reattach(_ context.Context, request browser.ReattachRequest) (browser.Result, error) {
	f.request = request
	return browser.Result{
		Answer:     "reattached answer",
		Transcript: "# Transcript\n\nreattached answer\n",
		Runtime: map[string]any{
			"endpoint":        request.Endpoint,
			"targetId":        request.TargetID,
			"conversationUrl": request.ConversationURL,
			"reattached":      true,
		},
	}, nil
}

type fakeSmokeChecker struct {
	request browser.SmokeRequest
}

func (f *fakeSmokeChecker) Check(_ context.Context, request browser.SmokeRequest) (browser.SmokeResult, error) {
	f.request = request
	return browser.SmokeResult{
		Endpoint: request.Endpoint,
		Auth: browser.AuthStatus{
			Authenticated:   true,
			ComposerVisible: true,
			Reason:          "composer-visible",
		},
		Checks: []browser.SmokeCheck{
			{Name: "cdp", Status: "ok"},
			{Name: "auth", Status: "ok"},
		},
	}, nil
}

type failingSmokeChecker struct {
	request browser.SmokeRequest
}

func (f *failingSmokeChecker) Check(_ context.Context, request browser.SmokeRequest) (browser.SmokeResult, error) {
	f.request = request
	return browser.SmokeResult{
		Endpoint:   request.Endpoint,
		ProfileDir: request.ProfileDir,
		ChatGPTURL: request.ChatGPTURL,
		Checks: []browser.SmokeCheck{
			{Name: "devtools-endpoint", Status: "fail", Message: "DevToolsActivePort missing"},
		},
	}, errors.New("DevToolsActivePort missing")
}

func TestToolsListExposesConsultAndSessions(t *testing.T) {
	server := mcpserver.New(mcpserver.Options{Store: session.NewStore(t.TempDir())})

	response := server.HandleForTest(json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	if response.Error != nil {
		t.Fatalf("tools/list error = %#v", response.Error)
	}

	var payload struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if !hasTool(payload.Tools, "consult") {
		t.Fatalf("tools = %#v, want consult", payload.Tools)
	}
	if !hasTool(payload.Tools, "consult_prepare") {
		t.Fatalf("tools = %#v, want consult_prepare", payload.Tools)
	}
	if !hasTool(payload.Tools, "consult_finalize") {
		t.Fatalf("tools = %#v, want consult_finalize", payload.Tools)
	}
	if !hasTool(payload.Tools, "sessions") {
		t.Fatalf("tools = %#v, want sessions", payload.Tools)
	}
	if !hasTool(payload.Tools, "session_delete") {
		t.Fatalf("tools = %#v, want session_delete", payload.Tools)
	}
	if !hasTool(payload.Tools, "login_setup") {
		t.Fatalf("tools = %#v, want login_setup", payload.Tools)
	}
	if !hasTool(payload.Tools, "reattach") {
		t.Fatalf("tools = %#v, want reattach", payload.Tools)
	}
	if !hasTool(payload.Tools, "smoke_check") {
		t.Fatalf("tools = %#v, want smoke_check", payload.Tools)
	}
}

func TestInitializeReturnsReleaseVersion(t *testing.T) {
	server := mcpserver.New(mcpserver.Options{Store: session.NewStore(t.TempDir())})

	response := server.HandleForTest(json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if response.Error != nil {
		t.Fatalf("initialize error = %#v", response.Error)
	}

	var payload struct {
		ServerInfo struct {
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.ServerInfo.Version != "0.1.6" {
		t.Fatalf("serverInfo.version = %q, want 0.1.6", payload.ServerInfo.Version)
	}
}

func TestConsultDryRunReturnsStructuredContent(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: session.NewStore(t.TempDir())})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "files": ["plan.md"],
      "dryRun": true
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		StructuredContent struct {
			Status         string `json:"status"`
			DryRun         bool   `json:"dryRun"`
			Output         string `json:"output"`
			AttachmentPlan struct {
				Mode string `json:"mode"`
			} `json:"attachmentPlan"`
			Resolved struct {
				ResolvedEngine string   `json:"resolvedEngine"`
				Model          string   `json:"model"`
				Files          []string `json:"files"`
				Browser        struct {
					DesiredModel  string `json:"desiredModel"`
					ThinkingLabel string `json:"thinkingLabel"`
				} `json:"browser"`
			} `json:"resolved"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.StructuredContent.Status != "dry-run" {
		t.Fatalf("status = %q, want dry-run", payload.StructuredContent.Status)
	}
	if !payload.StructuredContent.DryRun {
		t.Fatal("dryRun = false, want true")
	}
	if payload.StructuredContent.Resolved.ResolvedEngine != "browser" {
		t.Fatalf("resolvedEngine = %q, want browser", payload.StructuredContent.Resolved.ResolvedEngine)
	}
	if payload.StructuredContent.Resolved.Model != "gpt-6-astra-pro" {
		t.Fatalf("model = %q, want gpt-6-astra-pro", payload.StructuredContent.Resolved.Model)
	}
	if payload.StructuredContent.Resolved.Browser.DesiredModel != "GPT-6 Astra" {
		t.Fatalf("desired model = %q, want GPT-6 Astra", payload.StructuredContent.Resolved.Browser.DesiredModel)
	}
	if payload.StructuredContent.Resolved.Browser.ThinkingLabel != "Pro" {
		t.Fatalf("thinking label = %q, want Pro", payload.StructuredContent.Resolved.Browser.ThinkingLabel)
	}
	if len(payload.StructuredContent.Resolved.Files) != 1 || payload.StructuredContent.Resolved.Files[0] != "plan.md" {
		t.Fatalf("files = %#v, want [plan.md]", payload.StructuredContent.Resolved.Files)
	}
	if !strings.Contains(payload.StructuredContent.Output, "bundle characters:") {
		t.Fatalf("output = %q, want bundle character summary", payload.StructuredContent.Output)
	}
	if payload.StructuredContent.AttachmentPlan.Mode != "inline" {
		t.Fatalf("attachment mode = %q, want inline", payload.StructuredContent.AttachmentPlan.Mode)
	}
}

func TestConsultDryRunNormalizesExplicitGPT55(t *testing.T) {
	server := mcpserver.New(mcpserver.Options{Store: session.NewStore(t.TempDir())})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "model": " GPT-5.5 ",
      "dryRun": true
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		StructuredContent struct {
			Output   string `json:"output"`
			Resolved struct {
				Model   string `json:"model"`
				Browser struct {
					DesiredModel  string `json:"desiredModel"`
					ThinkingLabel string `json:"thinkingLabel"`
				} `json:"browser"`
			} `json:"resolved"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.StructuredContent.Resolved.Model != "gpt-6-astra-pro" {
		t.Fatalf("model = %q, want gpt-6-astra-pro", payload.StructuredContent.Resolved.Model)
	}
	if payload.StructuredContent.Resolved.Browser.DesiredModel != "GPT-6 Astra" {
		t.Fatalf("desired model = %q, want GPT-6 Astra", payload.StructuredContent.Resolved.Browser.DesiredModel)
	}
	if payload.StructuredContent.Resolved.Browser.ThinkingLabel != "Pro" {
		t.Fatalf("thinking label = %q, want Pro", payload.StructuredContent.Resolved.Browser.ThinkingLabel)
	}
	if !strings.Contains(strings.ToLower(payload.StructuredContent.Output), "normalized") {
		t.Fatalf("output = %q, want normalization action", payload.StructuredContent.Output)
	}
	if !strings.Contains(payload.StructuredContent.Output, "GPT-5.5") {
		t.Fatalf("output = %q, want original normalized model identifier", payload.StructuredContent.Output)
	}
	if !strings.Contains(payload.StructuredContent.Output, "gpt-6-astra-pro") {
		t.Fatalf("output = %q, want canonical model identifier", payload.StructuredContent.Output)
	}
}

func TestSessionDeleteRemovesLocalSessionFolder(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Model:  "gpt-5.5-pro",
		Mode:   "chrome-assisted",
		Status: "completed",
	})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	rawRequest, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      42,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "session_delete",
			"arguments": map[string]any{
				"id": meta.ID,
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal request: %v", err)
	}
	response := server.HandleForTest(rawRequest)
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			SessionID string `json:"sessionId"`
			Deleted   bool   `json:"deleted"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if payload.IsError {
		t.Fatal("isError = true, want false")
	}
	if payload.StructuredContent.SessionID != meta.ID || !payload.StructuredContent.Deleted {
		t.Fatalf("structuredContent = %#v, want deleted session", payload.StructuredContent)
	}
	if _, err := os.Stat(filepath.Join(store.SessionsDir(), meta.ID)); !os.IsNotExist(err) {
		t.Fatalf("session folder still exists, err=%v", err)
	}
}

func TestConsultPrepareCreatesChromeHandoffAndRunningSession(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: store})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 20,
  "method": "tools/call",
  "params": {
    "name": "consult_prepare",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "model": "gpt-5.5",
      "prompt": "review",
      "files": ["plan.md"],
      "browserThinkingTime": "extended",
      "browserFollowUps": [" summarize risk ", " ", "recommend next step"]
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status    string `json:"status"`
			SessionID string `json:"sessionId"`
			Handoff   struct {
				ChatGPTURL    string   `json:"chatgptUrl"`
				Model         string   `json:"model"`
				ModelLabel    string   `json:"modelLabel"`
				Thinking      string   `json:"thinking"`
				ThinkingLabel string   `json:"thinkingLabel"`
				HandoffDigest string   `json:"handoffDigest"`
				HandoffNonce  string   `json:"handoffNonce"`
				Prompt        string   `json:"prompt"`
				FollowUps     []string `json:"followUps"`
				Privacy       struct {
					TargetHost                                string   `json:"targetHost"`
					AllowBrowserHistory                       bool     `json:"allowBrowserHistory"`
					AllowUnrelatedTabs                        bool     `json:"allowUnrelatedTabs"`
					RequiresUserApprovalForExternalSubmission bool     `json:"requiresUserApprovalForExternalSubmission"`
					ExternalSubmissionAuthorizedByInvocation  bool     `json:"externalSubmissionAuthorizedByInvocation"`
					SubmittedFiles                            []string `json:"submittedFiles"`
				} `json:"privacy"`
				Capture struct {
					PreferMarkdownCopy       bool `json:"preferMarkdownCopy"`
					RequireFreshConversation bool `json:"requireFreshConversation"`
				} `json:"capture"`
				Submission struct {
					Mode                                     string `json:"mode"`
					PromptText                               string `json:"promptText"`
					ContextSource                            string `json:"contextSource"`
					ExternalSubmissionAuthorizedByInvocation bool   `json:"externalSubmissionAuthorizedByInvocation"`
					RequiresSeparateSendConfirmation         bool   `json:"requiresSeparateSendConfirmation"`
					StopOnHumanVerification                  bool   `json:"stopOnHumanVerification"`
				} `json:"submission"`
				Orchestration struct {
					RequiredPlugin                   string `json:"requiredPlugin"`
					AllowAutomaticSubmission         bool   `json:"allowAutomaticSubmission"`
					RequiresSeparateSendConfirmation bool   `json:"requiresSeparateSendConfirmation"`
				} `json:"orchestration"`
			} `json:"handoff"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.IsError {
		t.Fatalf("isError = true, want false")
	}
	if payload.StructuredContent.Status != "handoff-required" {
		t.Fatalf("status = %q, want handoff-required", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.SessionID == "" {
		t.Fatal("sessionId is empty")
	}
	if payload.StructuredContent.Handoff.ChatGPTURL != "https://chatgpt.com/" {
		t.Fatalf("handoff URL = %q, want ChatGPT", payload.StructuredContent.Handoff.ChatGPTURL)
	}
	if payload.StructuredContent.Handoff.Model != "gpt-6-astra-pro" {
		t.Fatalf("handoff model = %q, want gpt-6-astra-pro", payload.StructuredContent.Handoff.Model)
	}
	if payload.StructuredContent.Handoff.ModelLabel != "GPT-6 Astra" {
		t.Fatalf("handoff model label = %q, want GPT-6 Astra", payload.StructuredContent.Handoff.ModelLabel)
	}
	if payload.StructuredContent.Handoff.Thinking != "extended" {
		t.Fatalf("handoff thinking = %q, want extended", payload.StructuredContent.Handoff.Thinking)
	}
	if payload.StructuredContent.Handoff.ThinkingLabel != "Pro" {
		t.Fatalf("handoff thinking label = %q, want Pro", payload.StructuredContent.Handoff.ThinkingLabel)
	}
	if payload.StructuredContent.Handoff.HandoffDigest == "" {
		t.Fatal("handoff digest is empty")
	}
	if payload.StructuredContent.Handoff.HandoffNonce == "" {
		t.Fatal("handoff nonce is empty")
	}
	if payload.StructuredContent.Handoff.Privacy.TargetHost != "chatgpt.com" {
		t.Fatalf("privacy target host = %q, want chatgpt.com", payload.StructuredContent.Handoff.Privacy.TargetHost)
	}
	if payload.StructuredContent.Handoff.Privacy.AllowBrowserHistory {
		t.Fatal("privacy allows browser history, want false")
	}
	if payload.StructuredContent.Handoff.Privacy.AllowUnrelatedTabs {
		t.Fatal("privacy allows unrelated tabs, want false")
	}
	if payload.StructuredContent.Handoff.Privacy.RequiresUserApprovalForExternalSubmission {
		t.Fatal("privacy user approval = true, want false because invocation authorizes submission")
	}
	if !payload.StructuredContent.Handoff.Privacy.ExternalSubmissionAuthorizedByInvocation {
		t.Fatal("privacy invocation authorization = false, want true")
	}
	if len(payload.StructuredContent.Handoff.Privacy.SubmittedFiles) != 1 || payload.StructuredContent.Handoff.Privacy.SubmittedFiles[0] != "plan.md" {
		t.Fatalf("privacy submitted files = %#v, want [plan.md]", payload.StructuredContent.Handoff.Privacy.SubmittedFiles)
	}
	if !strings.Contains(payload.StructuredContent.Handoff.Prompt, "plan") {
		t.Fatalf("handoff prompt = %q, want file content", payload.StructuredContent.Handoff.Prompt)
	}
	if len(payload.StructuredContent.Handoff.FollowUps) != 2 {
		t.Fatalf("handoff followUps = %#v, want two normalized follow-ups", payload.StructuredContent.Handoff.FollowUps)
	}
	if !payload.StructuredContent.Handoff.Capture.PreferMarkdownCopy || !payload.StructuredContent.Handoff.Capture.RequireFreshConversation {
		t.Fatalf("handoff capture = %#v, want enabled copy/fresh hints", payload.StructuredContent.Handoff.Capture)
	}
	if payload.StructuredContent.Handoff.Submission.Mode != "inline" {
		t.Fatalf("submission mode = %q, want inline", payload.StructuredContent.Handoff.Submission.Mode)
	}
	if !strings.Contains(payload.StructuredContent.Handoff.Submission.PromptText, "plan") {
		t.Fatalf("submission prompt = %q, want inline bundle text", payload.StructuredContent.Handoff.Submission.PromptText)
	}
	if !payload.StructuredContent.Handoff.Submission.ExternalSubmissionAuthorizedByInvocation {
		t.Fatal("submission invocation authorization = false, want true")
	}
	if payload.StructuredContent.Handoff.Submission.RequiresSeparateSendConfirmation {
		t.Fatal("submission requires separate send confirmation, want false")
	}
	if !payload.StructuredContent.Handoff.Submission.StopOnHumanVerification {
		t.Fatal("submission stopOnHumanVerification = false, want true")
	}
	if payload.StructuredContent.Handoff.Orchestration.RequiredPlugin != "chrome@openai-bundled" {
		t.Fatalf("orchestration required plugin = %q, want chrome@openai-bundled", payload.StructuredContent.Handoff.Orchestration.RequiredPlugin)
	}
	if !payload.StructuredContent.Handoff.Orchestration.AllowAutomaticSubmission {
		t.Fatal("orchestration automatic submission = false, want true")
	}
	if payload.StructuredContent.Handoff.Orchestration.RequiresSeparateSendConfirmation {
		t.Fatal("orchestration requires separate send confirmation, want false")
	}

	detail, err := store.ReadDetail(payload.StructuredContent.SessionID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if detail.Metadata.Status != "running" {
		t.Fatalf("stored status = %q, want running", detail.Metadata.Status)
	}
	if detail.Metadata.Mode != "chrome-assisted" {
		t.Fatalf("stored mode = %q, want chrome-assisted", detail.Metadata.Mode)
	}
	if detail.Request["executionMode"] != "chrome-assisted" {
		t.Fatalf("stored executionMode = %#v, want chrome-assisted", detail.Request["executionMode"])
	}
	if detail.Request["handoffDigest"] != payload.StructuredContent.Handoff.HandoffDigest {
		t.Fatalf("stored handoffDigest = %#v, want response digest", detail.Request["handoffDigest"])
	}
	if detail.Request["handoffNonce"] != payload.StructuredContent.Handoff.HandoffNonce {
		t.Fatalf("stored handoffNonce = %#v, want response nonce", detail.Request["handoffNonce"])
	}
	if _, ok := detail.Request["followUps"]; ok {
		t.Fatalf("request persisted followUps in metadata mode: %#v", detail.Request)
	}
	for _, name := range []string{"meta.json", "request.json", "output.log"} {
		data, err := os.ReadFile(filepath.Join(store.SessionsDir(), payload.StructuredContent.SessionID, name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if strings.Contains(string(data), "summarize risk") || strings.Contains(string(data), "recommend next step") {
			t.Fatalf("%s persisted follow-up content: %q", name, string(data))
		}
	}
	handoffPath := filepath.Join(store.SessionsDir(), payload.StructuredContent.SessionID, "artifacts", "handoff.json")
	if _, err := os.Stat(handoffPath); !os.IsNotExist(err) {
		t.Fatalf("handoff artifact exists in metadata mode, err=%v", err)
	}
}

func TestConsultPrepareConvertsZipRequestToPastedTextHandoff(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: store})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 120,
  "method": "tools/call",
  "params": {
    "name": "consult_prepare",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "files": ["plan.md"],
      "browserBundleFormat": "zip"
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			AttachmentPlan struct {
				Mode   string `json:"mode"`
				Reason string `json:"reason"`
			} `json:"attachmentPlan"`
			Handoff struct {
				Submission struct {
					Mode       string `json:"mode"`
					PromptText string `json:"promptText"`
				} `json:"submission"`
			} `json:"handoff"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if payload.IsError {
		t.Fatal("isError = true, want false")
	}
	if payload.StructuredContent.AttachmentPlan.Mode != "text" {
		t.Fatalf("attachment mode = %q, want text", payload.StructuredContent.AttachmentPlan.Mode)
	}
	if !strings.Contains(payload.StructuredContent.AttachmentPlan.Reason, "pasted-text") {
		t.Fatalf("attachment reason = %q, want pasted-text explanation", payload.StructuredContent.AttachmentPlan.Reason)
	}
	if payload.StructuredContent.Handoff.Submission.Mode != "pasted-text-attachment" {
		t.Fatalf("submission mode = %q, want pasted-text-attachment", payload.StructuredContent.Handoff.Submission.Mode)
	}
	if !strings.Contains(payload.StructuredContent.Handoff.Submission.PromptText, "Oracle Consult Bundle") {
		t.Fatalf("submission prompt = %q, want Oracle bundle instruction", payload.StructuredContent.Handoff.Submission.PromptText)
	}
}

func TestConsultFinalizeCompletesChromeAssistedSession(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Prompt: "review",
		Model:  "gpt-5.5-pro",
		Mode:   "chrome-assisted",
		Status: "running",
		Request: map[string]any{
			"executionMode": "chrome-assisted",
			"handoffDigest": "digest-1",
			"handoffNonce":  "nonce-1",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 21,
  "method": "tools/call",
  "params": {
    "name": "consult_finalize",
    "arguments": {
      "sessionId": "` + meta.ID + `",
      "status": "completed",
      "handoffDigest": "digest-1",
      "handoffNonce": "nonce-1",
      "answer": "Chrome assisted answer",
      "transcript": "# Transcript\n\nChrome assisted answer\n",
      "runtime": {
        "runner": "chrome-plugin",
        "conversationUrl": "https://chatgpt.com/c/test"
      }
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status    string `json:"status"`
			SessionID string `json:"sessionId"`
			Output    string `json:"output"`
			Runtime   struct {
				Runner          string `json:"runner"`
				ConversationURL string `json:"conversationUrl"`
			} `json:"runtime"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.IsError {
		t.Fatalf("isError = true, want false")
	}
	if payload.StructuredContent.Status != "completed" {
		t.Fatalf("status = %q, want completed", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.SessionID != meta.ID {
		t.Fatalf("sessionId = %q, want %q", payload.StructuredContent.SessionID, meta.ID)
	}
	if payload.StructuredContent.Output != "Chrome assisted answer" {
		t.Fatalf("output = %q, want answer", payload.StructuredContent.Output)
	}
	if payload.StructuredContent.Runtime.Runner != "chrome-plugin" {
		t.Fatalf("runtime runner = %q, want chrome-plugin", payload.StructuredContent.Runtime.Runner)
	}

	detail, err := store.ReadDetail(meta.ID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if detail.Metadata.Status != "completed" {
		t.Fatalf("stored status = %q, want completed", detail.Metadata.Status)
	}
	if detail.Log != "Chrome assisted answer" {
		t.Fatalf("stored log = %q, want answer", detail.Log)
	}
	if detail.Metadata.Runtime["conversationUrl"] != "https://chatgpt.com/c/test" {
		t.Fatalf("stored runtime = %#v, want conversationUrl", detail.Metadata.Runtime)
	}
	transcriptPath := filepath.Join(store.SessionsDir(), meta.ID, "artifacts", "transcript.md")
	transcript, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("ReadFile transcript: %v", err)
	}
	if string(transcript) != "# Transcript\n\nChrome assisted answer\n" {
		t.Fatalf("transcript = %q, want provided transcript", string(transcript))
	}
}

func TestConsultFinalizeRejectsMismatchedHandoffDigest(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Prompt: "review",
		Model:  "gpt-5.5-pro",
		Mode:   "chrome-assisted",
		Status: "running",
		Request: map[string]any{
			"executionMode": "chrome-assisted",
			"handoffDigest": "digest-1",
			"handoffNonce":  "nonce-1",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 22,
  "method": "tools/call",
  "params": {
    "name": "consult_finalize",
    "arguments": {
      "sessionId": "` + meta.ID + `",
      "status": "completed",
      "handoffDigest": "wrong-digest",
      "handoffNonce": "nonce-1",
      "answer": "answer",
      "runtime": {"runner": "chrome-plugin", "conversationUrl": "https://chatgpt.com/c/test"}
    }
  }
}`))

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Output string `json:"output"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if !payload.IsError {
		t.Fatal("isError = false, want mismatched digest error")
	}
	if !strings.Contains(payload.StructuredContent.Output, "handoff digest") {
		t.Fatalf("output = %q, want handoff digest error", payload.StructuredContent.Output)
	}
}

func TestConsultFinalizeRejectsNonChatGPTConversationURL(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Prompt: "review",
		Model:  "gpt-5.5-pro",
		Mode:   "chrome-assisted",
		Status: "running",
		Request: map[string]any{
			"executionMode": "chrome-assisted",
			"handoffDigest": "digest-1",
			"handoffNonce":  "nonce-1",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 23,
  "method": "tools/call",
  "params": {
    "name": "consult_finalize",
    "arguments": {
      "sessionId": "` + meta.ID + `",
      "status": "completed",
      "handoffDigest": "digest-1",
      "handoffNonce": "nonce-1",
      "answer": "answer",
      "runtime": {"runner": "chrome-plugin", "conversationUrl": "https://example.com/c/test"}
    }
  }
}`))

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Output string `json:"output"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if !payload.IsError {
		t.Fatal("isError = false, want conversation host error")
	}
	if !strings.Contains(payload.StructuredContent.Output, "chatgpt.com") {
		t.Fatalf("output = %q, want chatgpt.com error", payload.StructuredContent.Output)
	}
}

func TestConsultFinalizeRejectsTerminalSessionWithoutForce(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Prompt: "review",
		Model:  "gpt-5.5-pro",
		Mode:   "chrome-assisted",
		Status: "completed",
		Output: "old answer",
		Request: map[string]any{
			"executionMode": "chrome-assisted",
			"handoffDigest": "digest-1",
			"handoffNonce":  "nonce-1",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 24,
  "method": "tools/call",
  "params": {
    "name": "consult_finalize",
    "arguments": {
      "sessionId": "` + meta.ID + `",
      "status": "completed",
      "handoffDigest": "digest-1",
      "handoffNonce": "nonce-1",
      "answer": "new answer",
      "runtime": {"runner": "chrome-plugin", "conversationUrl": "https://chatgpt.com/c/test"}
    }
  }
}`))

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Output string `json:"output"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if !payload.IsError {
		t.Fatal("isError = false, want terminal-session error")
	}
	if !strings.Contains(payload.StructuredContent.Output, "already terminal") {
		t.Fatalf("output = %q, want terminal-session error", payload.StructuredContent.Output)
	}
}

func TestConsultFinalizeRejectsMissingConversationURL(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Prompt: "review",
		Model:  "gpt-5.5-pro",
		Mode:   "chrome-assisted",
		Status: "running",
		Request: map[string]any{
			"executionMode": "chrome-assisted",
			"handoffDigest": "digest-1",
			"handoffNonce":  "nonce-1",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 122,
  "method": "tools/call",
  "params": {
    "name": "consult_finalize",
    "arguments": {
      "sessionId": "` + meta.ID + `",
      "status": "completed",
      "handoffDigest": "digest-1",
      "handoffNonce": "nonce-1",
      "answer": "answer",
      "runtime": {"runner": "chrome-plugin"}
    }
  }
}`))

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Output string `json:"output"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if !payload.IsError {
		t.Fatal("isError = false, want missing conversationUrl error")
	}
	if !strings.Contains(payload.StructuredContent.Output, "conversationUrl") {
		t.Fatalf("output = %q, want conversationUrl error", payload.StructuredContent.Output)
	}
}

func TestConsultFinalizeRejectsHTTPChatGPTConversationURL(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Prompt: "review",
		Model:  "gpt-5.5-pro",
		Mode:   "chrome-assisted",
		Status: "running",
		Request: map[string]any{
			"executionMode": "chrome-assisted",
			"handoffDigest": "digest-1",
			"handoffNonce":  "nonce-1",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 123,
  "method": "tools/call",
  "params": {
    "name": "consult_finalize",
    "arguments": {
      "sessionId": "` + meta.ID + `",
      "status": "completed",
      "handoffDigest": "digest-1",
      "handoffNonce": "nonce-1",
      "answer": "answer",
      "runtime": {"runner": "chrome-plugin", "conversationUrl": "http://chatgpt.com/c/test"}
    }
  }
}`))

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Output string `json:"output"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if !payload.IsError {
		t.Fatal("isError = false, want http conversationUrl error")
	}
	if !strings.Contains(payload.StructuredContent.Output, "https://chatgpt.com") {
		t.Fatalf("output = %q, want https://chatgpt.com error", payload.StructuredContent.Output)
	}
}

func TestConsultLiveDirectCDPDisabledByDefault(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: session.NewStore(t.TempDir()), BrowserRunner: fakeRunner{}})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 400,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "files": ["plan.md"]
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Output string `json:"output"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if !payload.IsError {
		t.Fatal("isError = false, want legacy direct-CDP disabled error")
	}
	if !strings.Contains(payload.StructuredContent.Output, "consult_prepare") {
		t.Fatalf("output = %q, want consult_prepare guidance", payload.StructuredContent.Output)
	}
}

func TestConsultLiveUsesInjectedRunnerAndStoresSession(t *testing.T) {
	t.Setenv("ORACLE_ENABLE_LEGACY_CDP", "1")
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: store, BrowserRunner: fakeRunner{}})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "files": ["plan.md"]
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		StructuredContent struct {
			SessionID string `json:"sessionId"`
			Status    string `json:"status"`
			Output    string `json:"output"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.StructuredContent.Status != "completed" {
		t.Fatalf("status = %q, want completed", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.SessionID == "" {
		t.Fatal("sessionId is empty")
	}
	if !strings.Contains(payload.StructuredContent.Output, "fake answer") {
		t.Fatalf("output = %q, want fake answer", payload.StructuredContent.Output)
	}
	detail, err := store.ReadDetail(payload.StructuredContent.SessionID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if !strings.Contains(detail.Log, "fake answer") {
		t.Fatalf("stored log = %q, want fake answer", detail.Log)
	}
	transcriptPath := filepath.Join(store.SessionsDir(), payload.StructuredContent.SessionID, "artifacts", "transcript.md")
	transcript, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("ReadFile transcript: %v", err)
	}
	if string(transcript) != "transcript" {
		t.Fatalf("transcript = %q, want transcript", string(transcript))
	}
	if detail.Metadata.Runtime["fake"] != true {
		t.Fatalf("runtime = %#v, want fake marker", detail.Metadata.Runtime)
	}
}

func TestConsultLiveCreatesRunningSessionBeforeRunner(t *testing.T) {
	t.Setenv("ORACLE_ENABLE_LEGACY_CDP", "1")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := session.NewStore(t.TempDir())
	runner := &inspectingRunner{store: store}
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: store, BrowserRunner: runner})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 12,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "files": ["plan.md"]
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}
	if !runner.sawRunning {
		t.Fatal("runner did not observe a running session before execution")
	}

	entries, err := store.List(session.ListOptions{IncludeAll: true})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %#v, want one session", entries)
	}
	if entries[0].Status != "completed" {
		t.Fatalf("final status = %q, want completed", entries[0].Status)
	}
}

func TestConsultLiveUpdatesHeartbeatWhileRunnerIsActive(t *testing.T) {
	t.Setenv("ORACLE_ENABLE_LEGACY_CDP", "1")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	store := session.NewStore(t.TempDir())
	runner := &waitingRunner{started: make(chan struct{}), release: make(chan struct{})}
	server := mcpserver.New(mcpserver.Options{
		CWD:               root,
		Store:             store,
		BrowserRunner:     runner,
		HeartbeatInterval: 5 * time.Millisecond,
	})
	done := make(chan mcpserver.RPCResponse, 1)

	go func() {
		done <- server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 13,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "files": ["plan.md"]
    }
  }
}`))
	}()

	<-runner.started
	deadline := time.Now().Add(500 * time.Millisecond)
	var heartbeat string
	for time.Now().Before(deadline) {
		entries, err := store.List(session.ListOptions{IncludeAll: true})
		if err == nil && len(entries) == 1 && entries[0].LastHeartbeatAt != "" {
			heartbeat = entries[0].LastHeartbeatAt
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	close(runner.release)
	response := <-done
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}
	if heartbeat == "" {
		t.Fatal("heartbeat was not written while runner was active")
	}
}

func TestConsultLiveCreatesForcedZipAttachmentForRunner(t *testing.T) {
	t.Setenv("ORACLE_ENABLE_LEGACY_CDP", "1")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runner := &recordingRunner{}
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: store, BrowserRunner: runner})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 11,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review attached context",
      "files": ["plan.md"],
      "browserAttachments": "always",
      "browserBundleFormat": "zip"
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	if len(runner.request.AttachmentPaths) != 1 {
		t.Fatalf("AttachmentPaths = %#v, want one attachment", runner.request.AttachmentPaths)
	}
	attachmentPath := runner.request.AttachmentPaths[0]
	if filepath.Ext(attachmentPath) != ".zip" {
		t.Fatalf("attachment path = %q, want .zip", attachmentPath)
	}
	if _, err := os.Stat(attachmentPath); err != nil {
		t.Fatalf("attachment artifact was not written: %v", err)
	}

	var payload struct {
		StructuredContent struct {
			SessionID string `json:"sessionId"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	detail, err := store.ReadDetail(payload.StructuredContent.SessionID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	attachments, ok := detail.Request["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("stored request attachments = %#v, want one attachment", detail.Request["attachments"])
	}
}

func TestConsultLivePassesFollowUpsToRunnerAndSession(t *testing.T) {
	t.Setenv("ORACLE_ENABLE_LEGACY_CDP", "1")
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runner := &recordingRunner{}
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: store, BrowserRunner: runner})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 18,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "files": ["plan.md"],
      "browserFollowUps": ["summarize risk", "  ", "recommend next step"]
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	wantFollowUps := []string{"summarize risk", "recommend next step"}
	if len(runner.request.FollowUps) != len(wantFollowUps) {
		t.Fatalf("runner followUps = %#v, want %#v", runner.request.FollowUps, wantFollowUps)
	}
	for i := range wantFollowUps {
		if runner.request.FollowUps[i] != wantFollowUps[i] {
			t.Fatalf("runner followUps = %#v, want %#v", runner.request.FollowUps, wantFollowUps)
		}
	}

	var payload struct {
		StructuredContent struct {
			SessionID string `json:"sessionId"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	detail, err := store.ReadDetail(payload.StructuredContent.SessionID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	stored, ok := detail.Request["followUps"].([]any)
	if !ok || len(stored) != len(wantFollowUps) {
		t.Fatalf("stored followUps = %#v, want %#v", detail.Request["followUps"], wantFollowUps)
	}
	for i := range wantFollowUps {
		if stored[i] != wantFollowUps[i] {
			t.Fatalf("stored followUps = %#v, want %#v", stored, wantFollowUps)
		}
	}
}

func TestConsultLiveDefaultRunnerUsesChromePathConfiguration(t *testing.T) {
	t.Setenv("ORACLE_ENABLE_LEGACY_CDP", "1")
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "plan.md"), []byte("plan"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{
		CWD: root,
		Env: map[string]string{
			"ORACLE_BROWSER_CHROME_PATH": filepath.Join(root, "missing-chrome.exe"),
			"ORACLE_ENABLE_LEGACY_CDP":   "1",
		},
		Store: session.NewStore(t.TempDir()),
	})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "tools/call",
  "params": {
    "name": "consult",
    "arguments": {
      "preset": "chatgpt-pro-heavy",
      "prompt": "review",
      "files": ["plan.md"]
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Output string `json:"output"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if !payload.IsError {
		t.Fatal("isError = false, want true")
	}
	if !strings.Contains(payload.StructuredContent.Output, "ORACLE_BROWSER_CHROME_PATH") {
		t.Fatalf("output = %q, want Chrome path configuration error", payload.StructuredContent.Output)
	}
	if strings.Contains(payload.StructuredContent.Output, "not implemented") {
		t.Fatalf("output = %q, default runner still reports not implemented", payload.StructuredContent.Output)
	}
}

func TestLoginSetupDryRunReturnsLaunchPlan(t *testing.T) {
	profileDir := t.TempDir()
	chromePath := filepath.Join(t.TempDir(), "chrome.exe")
	if err := os.WriteFile(chromePath, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{
		Env: map[string]string{
			"ORACLE_BROWSER_CHROME_PATH": chromePath,
			"ORACLE_BROWSER_PROFILE_DIR": profileDir,
		},
		Store: session.NewStore(t.TempDir()),
	})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "tools/call",
  "params": {
    "name": "login_setup",
    "arguments": {
      "dryRun": true
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		StructuredContent struct {
			Status    string   `json:"status"`
			Chrome    string   `json:"chromePath"`
			Profile   string   `json:"profileDir"`
			LaunchArg []string `json:"launchArgs"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.StructuredContent.Status != "dry-run" {
		t.Fatalf("status = %q, want dry-run", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.Chrome != chromePath {
		t.Fatalf("chromePath = %q, want %q", payload.StructuredContent.Chrome, chromePath)
	}
	if payload.StructuredContent.Profile != profileDir {
		t.Fatalf("profileDir = %q, want %q", payload.StructuredContent.Profile, profileDir)
	}
	if len(payload.StructuredContent.LaunchArg) == 0 {
		t.Fatal("launchArgs is empty")
	}
}

func TestLoginSetupDryRunReportsMissingChromeWithoutToolError(t *testing.T) {
	missingChrome := filepath.Join(t.TempDir(), "missing-chrome.exe")
	profileDir := t.TempDir()
	server := mcpserver.New(mcpserver.Options{
		Env: map[string]string{
			"ORACLE_BROWSER_CHROME_PATH": missingChrome,
			"ORACLE_BROWSER_PROFILE_DIR": profileDir,
		},
		Store: session.NewStore(t.TempDir()),
	})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 17,
  "method": "tools/call",
  "params": {
    "name": "login_setup",
    "arguments": {
      "dryRun": true
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status     string `json:"status"`
			Output     string `json:"output"`
			ChromePath string `json:"chromePath"`
			ProfileDir string `json:"profileDir"`
			Checks     []struct {
				Name    string `json:"name"`
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"checks"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.IsError {
		t.Fatal("isError = true, want false for dry-run diagnostics")
	}
	if payload.StructuredContent.Status != "dry-run" {
		t.Fatalf("status = %q, want dry-run", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.ProfileDir != profileDir {
		t.Fatalf("profileDir = %q, want %q", payload.StructuredContent.ProfileDir, profileDir)
	}
	if payload.StructuredContent.ChromePath != "" {
		t.Fatalf("chromePath = %q, want empty when Chrome is missing", payload.StructuredContent.ChromePath)
	}
	if !strings.Contains(payload.StructuredContent.Output, "Chrome") {
		t.Fatalf("output = %q, want Chrome diagnostic", payload.StructuredContent.Output)
	}
	if len(payload.StructuredContent.Checks) != 1 || payload.StructuredContent.Checks[0].Status != "fail" {
		t.Fatalf("checks = %#v, want one failing Chrome check", payload.StructuredContent.Checks)
	}
}

func TestLoginSetupLaunchesFakeChrome(t *testing.T) {
	profileDir := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	fakeChrome := filepath.Join(t.TempDir(), "fake-chrome.cmd")
	script := fmt.Sprintf(`@echo off
set profile=%%ORACLE_BROWSER_PROFILE_DIR%%
if "%%profile%%"=="" exit /b 2
if not exist "%%profile%%" mkdir "%%profile%%"
> "%%profile%%\DevToolsActivePort" echo %d
>> "%%profile%%\DevToolsActivePort" echo /devtools/browser/login
ping -n 5 127.0.0.1 > nul
`, port)
	if err := os.WriteFile(fakeChrome, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{
		Env: map[string]string{
			"ORACLE_BROWSER_CHROME_PATH": fakeChrome,
			"ORACLE_BROWSER_PROFILE_DIR": profileDir,
		},
		Store: session.NewStore(t.TempDir()),
	})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "tools/call",
  "params": {
    "name": "login_setup",
    "arguments": {
      "keepBrowser": false,
      "timeoutMs": 2000
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		StructuredContent struct {
			Status   string `json:"status"`
			Endpoint string `json:"endpoint"`
			PID      int    `json:"pid"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.StructuredContent.Status != "ready" {
		t.Fatalf("status = %q, want ready", payload.StructuredContent.Status)
	}
	wantEndpoint := fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser/login", port)
	if payload.StructuredContent.Endpoint != wantEndpoint {
		t.Fatalf("endpoint = %q, want fake login endpoint", payload.StructuredContent.Endpoint)
	}
	if payload.StructuredContent.PID <= 0 {
		t.Fatalf("PID = %d, want positive PID", payload.StructuredContent.PID)
	}
}

func TestSmokeCheckDryRunReturnsPlanWithoutLaunchingChrome(t *testing.T) {
	profileDir := t.TempDir()
	chromePath := filepath.Join(t.TempDir(), "chrome.exe")
	if err := os.WriteFile(chromePath, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{
		Env: map[string]string{
			"ORACLE_BROWSER_CHROME_PATH": chromePath,
			"ORACLE_BROWSER_PROFILE_DIR": profileDir,
		},
		Store: session.NewStore(t.TempDir()),
	})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 14,
  "method": "tools/call",
  "params": {
    "name": "smoke_check",
    "arguments": {
      "dryRun": true
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status     string `json:"status"`
			ChromePath string `json:"chromePath"`
			ProfileDir string `json:"profileDir"`
			Checks     []struct {
				Name   string `json:"name"`
				Status string `json:"status"`
			} `json:"checks"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.IsError {
		t.Fatalf("isError = true, want false")
	}
	if payload.StructuredContent.Status != "dry-run" {
		t.Fatalf("status = %q, want dry-run", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.ChromePath != chromePath {
		t.Fatalf("chromePath = %q, want %q", payload.StructuredContent.ChromePath, chromePath)
	}
	if payload.StructuredContent.ProfileDir != profileDir {
		t.Fatalf("profileDir = %q, want %q", payload.StructuredContent.ProfileDir, profileDir)
	}
	if len(payload.StructuredContent.Checks) == 0 {
		t.Fatal("checks is empty")
	}
}

func TestSmokeCheckLiveUsesInjectedChecker(t *testing.T) {
	profileDir := t.TempDir()
	checker := &fakeSmokeChecker{}
	server := mcpserver.New(mcpserver.Options{
		Env: map[string]string{
			"ORACLE_BROWSER_PROFILE_DIR": profileDir,
		},
		Store:        session.NewStore(t.TempDir()),
		SmokeChecker: checker,
	})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 15,
  "method": "tools/call",
  "params": {
    "name": "smoke_check",
    "arguments": {
      "endpoint": "ws://127.0.0.1:9222/devtools/browser/smoke",
      "dryRun": false,
      "timeoutMs": 1000
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status string `json:"status"`
			Auth   struct {
				Authenticated   bool `json:"authenticated"`
				ComposerVisible bool `json:"composerVisible"`
			} `json:"auth"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.IsError {
		t.Fatalf("isError = true, want false")
	}
	if payload.StructuredContent.Status != "ready" {
		t.Fatalf("status = %q, want ready", payload.StructuredContent.Status)
	}
	if !payload.StructuredContent.Auth.Authenticated || !payload.StructuredContent.Auth.ComposerVisible {
		t.Fatalf("auth = %#v, want authenticated composer", payload.StructuredContent.Auth)
	}
	if checker.request.Endpoint != "ws://127.0.0.1:9222/devtools/browser/smoke" {
		t.Fatalf("checker endpoint = %q, want smoke endpoint", checker.request.Endpoint)
	}
	if checker.request.ProfileDir != profileDir {
		t.Fatalf("checker profileDir = %q, want %q", checker.request.ProfileDir, profileDir)
	}
}

func TestSmokeCheckLiveErrorPreservesStructuredDiagnostics(t *testing.T) {
	profileDir := t.TempDir()
	checker := &failingSmokeChecker{}
	server := mcpserver.New(mcpserver.Options{
		Env: map[string]string{
			"ORACLE_BROWSER_PROFILE_DIR": profileDir,
		},
		Store:        session.NewStore(t.TempDir()),
		SmokeChecker: checker,
	})

	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 16,
  "method": "tools/call",
  "params": {
    "name": "smoke_check",
    "arguments": {
      "endpoint": "ws://127.0.0.1:9222/devtools/browser/missing",
      "dryRun": false
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status     string `json:"status"`
			Output     string `json:"output"`
			ProfileDir string `json:"profileDir"`
			Endpoint   string `json:"endpoint"`
			Checks     []struct {
				Name    string `json:"name"`
				Status  string `json:"status"`
				Message string `json:"message"`
			} `json:"checks"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if !payload.IsError {
		t.Fatalf("isError = false, want true")
	}
	if payload.StructuredContent.Status != "error" {
		t.Fatalf("status = %q, want error", payload.StructuredContent.Status)
	}
	if !strings.Contains(payload.StructuredContent.Output, "DevToolsActivePort missing") {
		t.Fatalf("output = %q, want DevToolsActivePort message", payload.StructuredContent.Output)
	}
	if payload.StructuredContent.ProfileDir != profileDir {
		t.Fatalf("profileDir = %q, want %q", payload.StructuredContent.ProfileDir, profileDir)
	}
	if payload.StructuredContent.Endpoint != "ws://127.0.0.1:9222/devtools/browser/missing" {
		t.Fatalf("endpoint = %q, want missing endpoint", payload.StructuredContent.Endpoint)
	}
	if len(payload.StructuredContent.Checks) != 1 || payload.StructuredContent.Checks[0].Status != "fail" {
		t.Fatalf("checks = %#v, want one failing check", payload.StructuredContent.Checks)
	}
}

func TestReattachDryRunReturnsRuntimePlan(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Model:  "gpt-5.5-pro",
		Mode:   "browser",
		Status: "running",
		Output: "partial answer",
		Runtime: map[string]any{
			"pid":             1234,
			"endpoint":        "ws://127.0.0.1:9222/devtools/browser/session",
			"targetId":        "target-1",
			"conversationUrl": "https://chatgpt.com/c/session",
			"profileDir":      filepath.Join(t.TempDir(), "profile"),
		},
	})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	rawRequest, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      8,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "reattach",
			"arguments": map[string]any{
				"id":     meta.ID,
				"dryRun": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal request: %v", err)
	}
	response := server.HandleForTest(rawRequest)
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status       string `json:"status"`
			SessionID    string `json:"sessionId"`
			Reattachable bool   `json:"reattachable"`
			Runtime      struct {
				Endpoint        string `json:"endpoint"`
				TargetID        string `json:"targetId"`
				ConversationURL string `json:"conversationUrl"`
				ProfileDir      string `json:"profileDir"`
			} `json:"runtime"`
			Plan struct {
				Steps []string `json:"steps"`
			} `json:"plan"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.IsError {
		t.Fatalf("isError = true, want false")
	}
	if payload.StructuredContent.Status != "dry-run" {
		t.Fatalf("status = %q, want dry-run", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.SessionID != meta.ID {
		t.Fatalf("sessionId = %q, want %q", payload.StructuredContent.SessionID, meta.ID)
	}
	if !payload.StructuredContent.Reattachable {
		t.Fatal("reattachable = false, want true")
	}
	if payload.StructuredContent.Runtime.Endpoint == "" {
		t.Fatal("runtime.endpoint is empty")
	}
	if payload.StructuredContent.Runtime.TargetID != "target-1" {
		t.Fatalf("targetId = %q, want target-1", payload.StructuredContent.Runtime.TargetID)
	}
	if payload.StructuredContent.Runtime.ConversationURL == "" {
		t.Fatal("runtime.conversationUrl is empty")
	}
	if payload.StructuredContent.Runtime.ProfileDir == "" {
		t.Fatal("runtime.profileDir is empty")
	}
	if len(payload.StructuredContent.Plan.Steps) == 0 {
		t.Fatal("plan.steps is empty")
	}
}

func TestReattachDryRunReportsMissingRuntime(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Model:  "gpt-5.5-pro",
		Mode:   "browser",
		Status: "completed",
		Output: "answer",
	})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})

	rawRequest, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      9,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "reattach",
			"arguments": map[string]any{
				"id":     meta.ID,
				"dryRun": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal request: %v", err)
	}
	response := server.HandleForTest(rawRequest)
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status       string   `json:"status"`
			Reattachable bool     `json:"reattachable"`
			Reasons      []string `json:"reasons"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.IsError {
		t.Fatalf("isError = true, want false")
	}
	if payload.StructuredContent.Status != "dry-run" {
		t.Fatalf("status = %q, want dry-run", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.Reattachable {
		t.Fatal("reattachable = true, want false")
	}
	if len(payload.StructuredContent.Reasons) == 0 {
		t.Fatal("reasons is empty")
	}
}

func TestReattachLiveUsesInjectedReattacherAndUpdatesSession(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	store := session.NewStore(t.TempDir())
	profileDir := filepath.Join(t.TempDir(), "profile")
	meta, err := store.Create(session.CreateOptions{
		Model:  "gpt-5.5-pro",
		Mode:   "browser",
		Status: "running",
		Output: "partial answer",
		Runtime: map[string]any{
			"endpoint":        "ws://127.0.0.1:9222/devtools/browser/session",
			"targetId":        "target-1",
			"conversationUrl": "https://chatgpt.com/c/session",
			"profileDir":      profileDir,
		},
	})
	if err != nil {
		t.Fatalf("Create session: %v", err)
	}
	reattacher := &fakeReattacher{}
	server := mcpserver.New(mcpserver.Options{Store: store, BrowserReattacher: reattacher})

	rawRequest, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      10,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "reattach",
			"arguments": map[string]any{
				"id":        meta.ID,
				"dryRun":    false,
				"timeoutMs": 1000,
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal request: %v", err)
	}
	response := server.HandleForTest(rawRequest)
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Status    string `json:"status"`
			SessionID string `json:"sessionId"`
			Output    string `json:"output"`
			Runtime   struct {
				Reattached bool `json:"reattached"`
			} `json:"runtime"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)

	if payload.IsError {
		t.Fatalf("isError = true, want false")
	}
	if payload.StructuredContent.Status != "completed" {
		t.Fatalf("status = %q, want completed", payload.StructuredContent.Status)
	}
	if payload.StructuredContent.SessionID != meta.ID {
		t.Fatalf("sessionId = %q, want %q", payload.StructuredContent.SessionID, meta.ID)
	}
	if payload.StructuredContent.Output != "reattached answer" {
		t.Fatalf("output = %q, want reattached answer", payload.StructuredContent.Output)
	}
	if !payload.StructuredContent.Runtime.Reattached {
		t.Fatal("runtime.reattached = false, want true")
	}
	if reattacher.request.Endpoint == "" {
		t.Fatal("reattacher request endpoint is empty")
	}
	if reattacher.request.TargetID != "target-1" {
		t.Fatalf("reattacher targetId = %q, want target-1", reattacher.request.TargetID)
	}
	if reattacher.request.ProfileDir != profileDir {
		t.Fatalf("reattacher profileDir = %q, want %q", reattacher.request.ProfileDir, profileDir)
	}

	detail, err := store.ReadDetail(meta.ID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if detail.Metadata.Status != "completed" {
		t.Fatalf("stored status = %q, want completed", detail.Metadata.Status)
	}
	if detail.Log != "reattached answer" {
		t.Fatalf("stored log = %q, want reattached answer", detail.Log)
	}
	if detail.Metadata.Runtime["reattached"] != true {
		t.Fatalf("stored runtime = %#v, want reattached marker", detail.Metadata.Runtime)
	}
}

func TestConsultFinalizeMetadataModeReturnsButDoesNotPersistFailureContent(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "")
	const failure = "sensitive finalize failure text"
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Model:  "gpt-6-astra-pro",
		Mode:   "chrome-assisted",
		Status: "running",
		Request: map[string]any{
			"executionMode": "chrome-assisted",
			"handoffDigest": "digest-1",
			"handoffNonce":  "nonce-1",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})
	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 301,
  "method": "tools/call",
  "params": {
    "name": "consult_finalize",
    "arguments": {
      "sessionId": "` + meta.ID + `",
      "status": "failed",
      "handoffDigest": "digest-1",
      "handoffNonce": "nonce-1",
      "error": "` + failure + `",
      "runtime": {
        "runner": "chrome-plugin",
        "conversationUrl": "https://chatgpt.com/c/test",
        "error": "` + failure + `",
        "prompt": "sensitive prompt",
        "answer": "sensitive answer",
        "transcript": "sensitive transcript",
        "output": "sensitive output",
        "followUps": ["sensitive follow-up"]
      }
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}

	var payload struct {
		IsError           bool `json:"isError"`
		StructuredContent struct {
			Output  string         `json:"output"`
			Runtime map[string]any `json:"runtime"`
		} `json:"structuredContent"`
	}
	mustMarshalResult(t, response.Result, &payload)
	if payload.IsError {
		t.Fatal("isError = true, want failed session result returned to caller")
	}
	if payload.StructuredContent.Output != failure {
		t.Fatalf("output = %q, want failure text returned to caller", payload.StructuredContent.Output)
	}
	if _, ok := payload.StructuredContent.Runtime["error"]; ok {
		t.Fatalf("returned runtime = %#v, want no persisted content-shaped error", payload.StructuredContent.Runtime)
	}

	detail, err := store.ReadDetail(meta.ID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if detail.Log != "" {
		t.Fatalf("stored log = %q, want no failure content in metadata mode", detail.Log)
	}
	if detail.Metadata.Runtime["runner"] != "chrome-plugin" || detail.Metadata.Runtime["conversationUrl"] != "https://chatgpt.com/c/test" {
		t.Fatalf("stored runtime = %#v, want operational metadata", detail.Metadata.Runtime)
	}
	for _, key := range []string{"error", "prompt", "answer", "transcript", "output", "followUps"} {
		if _, ok := detail.Metadata.Runtime[key]; ok {
			t.Fatalf("stored runtime = %#v, want %q removed", detail.Metadata.Runtime, key)
		}
	}
	for _, name := range []string{"meta.json", "request.json", "output.log"} {
		data, err := os.ReadFile(filepath.Join(store.SessionsDir(), meta.ID, name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if strings.Contains(string(data), failure) {
			t.Fatalf("%s persisted failure content: %q", name, string(data))
		}
	}
}

func TestConsultFinalizeFullContentModePersistsFailureContent(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	const failure = "full content failure text"
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Model:  "gpt-6-astra-pro",
		Mode:   "chrome-assisted",
		Status: "running",
		Request: map[string]any{
			"executionMode": "chrome-assisted",
			"handoffDigest": "digest-1",
			"handoffNonce":  "nonce-1",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	server := mcpserver.New(mcpserver.Options{Store: store})
	response := server.HandleForTest(json.RawMessage(`{
  "jsonrpc": "2.0",
  "id": 302,
  "method": "tools/call",
  "params": {
    "name": "consult_finalize",
    "arguments": {
      "sessionId": "` + meta.ID + `",
      "status": "failed",
      "handoffDigest": "digest-1",
      "handoffNonce": "nonce-1",
      "error": "` + failure + `",
      "runtime": {"runner": "chrome-plugin", "error": "` + failure + `"}
    }
  }
}`))
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}
	detail, err := store.ReadDetail(meta.ID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if detail.Log != failure {
		t.Fatalf("stored log = %q, want full-content failure", detail.Log)
	}
	if detail.Metadata.Runtime["error"] != failure {
		t.Fatalf("stored runtime = %#v, want full-content failure error", detail.Metadata.Runtime)
	}
}

func hasTool(tools []struct {
	Name string `json:"name"`
}, name string) bool {
	for _, tool := range tools {
		if tool.Name == name {
			return true
		}
	}
	return false
}

func mustMarshalResult(t *testing.T, result any, out any) {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("Unmarshal result: %v; json=%s", err, data)
	}
}
