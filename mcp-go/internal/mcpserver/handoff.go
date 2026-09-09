package mcpserver

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"codex_oracle/internal/bundle"
	"codex_oracle/internal/oracle"
	"codex_oracle/internal/session"
)

func (s *Server) callConsultPrepare(arguments json.RawMessage) any {
	var input oracle.ConsultInput
	if err := decodeStrict(arguments, &input); err != nil {
		return toolError(err.Error())
	}
	if strings.TrimSpace(input.Prompt) == "" {
		return toolError("Prompt is required.")
	}
	resolvedInput, err := oracle.ApplyConsultPreset(input)
	if err != nil {
		return toolError(err.Error())
	}
	workspaceRoot, err := s.resolveWorkspaceRoot(string(resolvedInput.WorkspaceRoot))
	if err != nil {
		return toolError(err.Error())
	}
	files, err := bundle.Resolve(workspaceRoot, resolvedInput.Files, bundle.Options{MaxFileSizeBytes: 1024 * 1024})
	if err != nil {
		return toolError(err.Error())
	}
	filePaths := files.RelativePaths()
	rendered, err := bundle.RenderPromptBundle(resolvedInput.Prompt, files.Files)
	if err != nil {
		return toolError(err.Error())
	}
	resolved := oracle.BuildDryRunResolved(resolvedInput, filePaths, s.env)
	if resolved.ResolvedEngine != "browser" {
		return toolError("consult_prepare requires browser engine; use preset:\"chatgpt-pro-heavy\" or engine:\"browser\".")
	}
	attachmentPlan := buildChromeHandoffAttachmentPlan(buildAttachmentPlan(resolvedInput, rendered, files.Files))
	handoff, err := oracle.BuildChromeHandoff(resolved, rendered.Text, resolvedInput.BrowserFollowUps, oracle.ChromeHandoffOptions{
		AttachmentMode: stringValue(attachmentPlan["mode"]),
	})
	if err != nil {
		return toolError(err.Error())
	}
	request := map[string]any{
		"model":          resolved.Model,
		"engine":         resolved.ResolvedEngine,
		"executionMode":  string(oracle.ExecutionModeChromeAssisted),
		"files":          filePaths,
		"bundleText":     rendered.Text,
		"attachmentPlan": attachmentPlan,
		"handoff":        handoff,
		"handoffDigest":  handoff.HandoffDigest,
		"handoffNonce":   handoff.HandoffNonce,
		"privacy":        handoff.Privacy,
	}
	if len(handoff.FollowUps) > 0 {
		request["followUps"] = handoff.FollowUps
	}
	meta, err := s.store.Create(session.CreateOptions{
		Prompt:  resolvedInput.Prompt,
		Model:   resolved.Model,
		Mode:    string(oracle.ExecutionModeChromeAssisted),
		CWD:     workspaceRoot,
		Status:  "running",
		Slug:    resolvedInput.Slug,
		Request: request,
		Output:  "",
	})
	if err != nil {
		return toolError(err.Error())
	}
	if session.ShouldPersistContent() {
		handoffJSON, err := json.MarshalIndent(handoff, "", "  ")
		if err != nil {
			return toolError(err.Error())
		}
		if err := s.store.WriteArtifact(meta.ID, "artifacts/handoff.json", handoffJSON); err != nil {
			return toolError(err.Error())
		}
	}

	output := "Session " + meta.ID + " is ready for Chrome handoff."
	return toolResult(output, map[string]any{
		"status":    "handoff-required",
		"output":    output,
		"sessionId": meta.ID,
		"checkpoint": map[string]any{
			"tool":            "consult_checkpoint",
			"when":            "After sending, wait for a durable /c/<id> conversation URL, then checkpoint promptly. If the URL contains WEB:, wait in the same tab; never resubmit the prompt.",
			"arguments":       map[string]any{"sessionId": meta.ID, "handoffDigest": handoff.HandoffDigest, "handoffNonce": handoff.HandoffNonce},
			"requiredCapture": "conversationUrl",
		},
		"handoff":  handoff,
		"resolved": resolved,
		"bundle": map[string]any{
			"fileCount":       rendered.FileCount,
			"characterCount":  rendered.CharacterCount,
			"estimatedTokens": rendered.EstimatedTokens,
			"preview":         rendered.Preview(4000),
		},
		"attachmentPlan": attachmentPlan,
		"files":          files.Files,
		"ignored":        files.Ignored,
	})
}

func buildChromeHandoffAttachmentPlan(plan map[string]any) map[string]any {
	normalized := map[string]any{}
	for key, value := range plan {
		normalized[key] = value
	}
	if stringValue(normalized["mode"]) == "zip" {
		normalized["mode"] = "text"
		normalized["reason"] = "Chrome-assisted handoff uses pasted-text attachment; file upload is legacy direct-CDP diagnostic only"
	}
	return normalized
}

func (s *Server) callConsultFinalize(arguments json.RawMessage) any {
	var input struct {
		SessionID     string         `json:"sessionId"`
		Status        string         `json:"status"`
		Answer        string         `json:"answer,omitempty"`
		Error         string         `json:"error,omitempty"`
		Transcript    string         `json:"transcript,omitempty"`
		HandoffDigest string         `json:"handoffDigest,omitempty"`
		HandoffNonce  string         `json:"handoffNonce,omitempty"`
		Force         bool           `json:"force,omitempty"`
		Runtime       map[string]any `json:"runtime,omitempty"`
	}
	if err := decodeStrict(arguments, &input); err != nil {
		return toolError(err.Error())
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		return toolError("Session id is required.")
	}
	detail, err := s.store.ReadDetail(sessionID)
	if err != nil {
		return toolError(fmt.Sprintf("Session %q not found.", sessionID))
	}
	if detail.Metadata.Mode != string(oracle.ExecutionModeChromeAssisted) && detail.Request["executionMode"] != string(oracle.ExecutionModeChromeAssisted) {
		return toolError("session was not prepared for chrome-assisted finalize")
	}
	if isTerminalStatus(detail.Metadata.Status) && !input.Force {
		return toolError("session is already terminal; pass force:true to overwrite")
	}
	if err := validateHandoffMatch(detail.Request, input.HandoffDigest, input.HandoffNonce); err != nil {
		return toolError(err.Error())
	}
	status := strings.TrimSpace(input.Status)
	if status != "completed" && status != "failed" {
		return toolError("status must be completed or failed")
	}
	output := input.Answer
	if status == "completed" && strings.TrimSpace(output) == "" {
		return toolError("answer is required when status is completed")
	}
	if status == "failed" {
		output = strings.TrimSpace(input.Error)
		if output == "" {
			return toolError("error is required when status is failed")
		}
	}
	runtime := sanitizeFinalizeRuntime(input.Runtime)
	if status == "failed" && session.ShouldPersistContent() {
		runtime["error"] = output
	}
	if err := validateFinalizeConversationURL(status, runtime); err != nil {
		return toolError(err.Error())
	}
	if previous := stringValue(detail.Metadata.Runtime["conversationUrl"]); previous != "" && !input.Force {
		if current := stringValue(runtime["conversationUrl"]); current != "" && current != previous {
			return toolError("captured conversationUrl differs from the saved checkpoint")
		}
	}
	if err := s.store.UpdateResult(sessionID, session.UpdateOptions{
		Status:     status,
		Output:     output,
		Transcript: input.Transcript,
		Runtime:    runtime,
	}); err != nil {
		return toolError(err.Error())
	}
	text := "Session " + sessionID + " (" + status + ")"
	if output != "" {
		text += "\n" + output
	}
	return toolResult(text, map[string]any{
		"sessionId": sessionID,
		"status":    status,
		"output":    output,
		"runtime":   runtime,
	})
}

func (s *Server) callConsultCheckpoint(arguments json.RawMessage) any {
	var input struct {
		SessionID       string `json:"sessionId"`
		HandoffDigest   string `json:"handoffDigest"`
		HandoffNonce    string `json:"handoffNonce"`
		ConversationURL string `json:"conversationUrl"`
	}
	if err := decodeStrict(arguments, &input); err != nil {
		return toolError(err.Error())
	}
	if strings.TrimSpace(input.SessionID) == "" || strings.TrimSpace(input.HandoffDigest) == "" || strings.TrimSpace(input.HandoffNonce) == "" {
		return toolError("sessionId, handoffDigest and handoffNonce are required")
	}
	detail, err := s.store.ReadDetail(input.SessionID)
	if err != nil {
		return toolError(err.Error())
	}
	if detail.Metadata.Mode != string(oracle.ExecutionModeChromeAssisted) {
		return toolError("only a Chrome-assisted session can be checkpointed")
	}
	if stringValue(detail.Request["handoffDigest"]) == "" || stringValue(detail.Request["handoffNonce"]) == "" {
		return toolError("session has no prepared handoff identity")
	}
	if err := validateHandoffMatch(detail.Request, input.HandoffDigest, input.HandoffNonce); err != nil {
		return toolError(err.Error())
	}
	conversationURL := strings.TrimSpace(input.ConversationURL)
	if err := validateFinalizeConversationURL("completed", map[string]any{"conversationUrl": conversationURL}); err != nil {
		return toolError(err.Error())
	}
	if err := s.store.UpdateConversation(input.SessionID, conversationURL); err != nil {
		return toolError(err.Error())
	}
	return toolResult("Conversation URL saved; session remains running.", map[string]any{
		"status": "running", "sessionId": detail.Metadata.ID, "checkpointed": true,
		"runtime": map[string]any{"conversationUrl": conversationURL},
	})
}

func isTerminalStatus(status string) bool {
	switch strings.TrimSpace(status) {
	case "completed", "failed":
		return true
	default:
		return false
	}
}

// Chrome recovery is a handoff to the browser-owning agent, just like prepare.
// It must never fall through to the legacy private-profile CDP runner.
func chromeRecoveryPlan(detail *session.Detail) map[string]any {
	conversationURL := stringValue(detail.Metadata.Runtime["conversationUrl"])
	reasons := []string{}
	if err := validateFinalizeConversationURL("completed", map[string]any{"conversationUrl": conversationURL}); err != nil {
		conversationURL = ""
		reasons = append(reasons, "No valid saved ChatGPT conversation URL; recover it from this task's previously claimed tab or ask the user for the URL.")
	}
	steps := []string{
		"Use @Chrome with the user's signed-in session to open the saved conversation URL, or recover this task's previously claimed tab when no URL was saved.",
		"Do not inspect unrelated tabs or browser history, create a new conversation, or resubmit the prompt. Stop for login or human verification.",
		"Verify this is the original review conversation and wait for its answer to finish. Capture the answer and conversation URL.",
		"Call consult_finalize with the returned finalize identity, status, captured answer, and runtime.conversationUrl. A failed capture can be finalized with status:failed and error.",
	}
	terminal := isTerminalStatus(detail.Metadata.Status)
	if terminal {
		steps = append(steps, "This session is already terminal. Reading its answer in Chrome does not require another finalize. Only explicitly replace the saved result with force:true when intended.")
	}
	output := "Session " + detail.Metadata.ID + " requires @Chrome answer capture; no browser was opened and no session was changed."
	if conversationURL == "" {
		output += " The conversation URL was not saved; recover this task's tab or ask the user for its URL."
	}
	return toolResult(output, map[string]any{
		"status":                 "chrome-recovery-required",
		"output":                 output,
		"sessionId":              detail.Metadata.ID,
		"sessionStatus":          detail.Metadata.Status,
		"executionMode":          string(oracle.ExecutionModeChromeAssisted),
		"requiresBrowserCapture": true,
		"requiresForce":          terminal,
		"reattachable":           conversationURL != "",
		"reasons":                reasons,
		"runtime":                map[string]any{"conversationUrl": conversationURL},
		"finalize": map[string]any{
			"sessionId":     detail.Metadata.ID,
			"handoffDigest": stringValue(detail.Request["handoffDigest"]),
			"handoffNonce":  stringValue(detail.Request["handoffNonce"]),
		},
		"plan": map[string]any{"steps": steps},
	})
}

func validateHandoffMatch(request map[string]any, providedDigest string, providedNonce string) error {
	expectedDigest := stringValue(request["handoffDigest"])
	expectedNonce := stringValue(request["handoffNonce"])
	if expectedDigest != "" {
		if strings.TrimSpace(providedDigest) == "" {
			return fmt.Errorf("handoff digest is required for this session")
		}
		if strings.TrimSpace(providedDigest) != expectedDigest {
			return fmt.Errorf("handoff digest mismatch")
		}
	}
	if expectedNonce != "" {
		if strings.TrimSpace(providedNonce) == "" {
			return fmt.Errorf("handoff nonce is required for this session")
		}
		if strings.TrimSpace(providedNonce) != expectedNonce {
			return fmt.Errorf("handoff nonce mismatch")
		}
	}
	return nil
}

var conversationPath = regexp.MustCompile(`^/(?:g/[^/]+/)?c/[A-Za-z0-9_-]+$`)

func validateFinalizeConversationURL(status string, runtime map[string]any) error {
	if status != "completed" && stringValue(runtime["conversationUrl"]) == "" {
		return nil
	}
	conversationURL := stringValue(runtime["conversationUrl"])
	if conversationURL == "" {
		return fmt.Errorf("conversationUrl is required when status is completed")
	}
	parsed, err := url.Parse(conversationURL)
	if err != nil {
		return fmt.Errorf("conversationUrl must be a valid chatgpt.com URL")
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return fmt.Errorf("conversationUrl must be an https://chatgpt.com URL")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "chatgpt.com" {
		return fmt.Errorf("conversationUrl must be on chatgpt.com")
	}
	if strings.Contains(parsed.Path, "/c/WEB:") {
		return fmt.Errorf("conversationUrl is temporary (WEB:); wait in the same tab for a durable /c/<id> URL and retry without resubmitting the prompt")
	}
	if parsed.User != nil || parsed.Port() != "" || parsed.RawQuery != "" || parsed.Fragment != "" || !conversationPath.MatchString(parsed.EscapedPath()) {
		return fmt.Errorf("conversationUrl must identify a ChatGPT /c/<id> conversation without credentials, port, query or fragment")
	}
	return nil
}

func sanitizeFinalizeRuntime(runtime map[string]any) map[string]any {
	sanitized := map[string]any{}
	for key, value := range runtime {
		if isSensitiveRuntimeKey(key) {
			continue
		}
		if !session.ShouldPersistContent() && isContentRuntimeKey(key) {
			continue
		}
		sanitized[key] = sanitizeRuntimeValue(value)
	}
	return sanitized
}

func sanitizeRuntimeValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return sanitizeFinalizeRuntime(typed)
	case []any:
		values := make([]any, 0, len(typed))
		for _, item := range typed {
			values = append(values, sanitizeRuntimeValue(item))
		}
		return values
	default:
		return value
	}
}

func isSensitiveRuntimeKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return strings.Contains(normalized, "cookie") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "api_key") ||
		strings.Contains(normalized, "apikey")
}

func isContentRuntimeKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "error", "prompt", "answer", "transcript", "output", "followups":
		return true
	default:
		return false
	}
}
