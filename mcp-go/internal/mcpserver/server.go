package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"codex_oracle/internal/browser"
	"codex_oracle/internal/bundle"
	"codex_oracle/internal/oracle"
	"codex_oracle/internal/session"
)

const version = "0.1.6"

type Options struct {
	CWD               string
	Env               map[string]string
	Store             *session.Store
	BrowserRunner     browser.Runner
	BrowserReattacher browser.Reattacher
	SmokeChecker      browser.SmokeChecker
	HeartbeatInterval time.Duration
}

type Server struct {
	cwd               string
	env               map[string]string
	store             *session.Store
	browserRunner     browser.Runner
	browserReattacher browser.Reattacher
	smokeChecker      browser.SmokeChecker
	heartbeatInterval time.Duration
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type RPCResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *RPCError `json:"error,omitempty"`
}

type rpcRequest struct {
	jsonrpc string
	id      json.RawMessage
	hasID   bool
	method  string
	params  json.RawMessage
}

func New(options Options) *Server {
	cwd := options.CWD
	if cwd == "" {
		if current, err := os.Getwd(); err == nil {
			cwd = current
		}
	}
	env := options.Env
	if env == nil {
		env = environMap()
	}
	store := options.Store
	if store == nil {
		store = session.NewStore("")
	}
	browserRunner := options.BrowserRunner
	if browserRunner == nil {
		browserRunner = browser.NewCDPRunner(env)
	}
	browserReattacher := options.BrowserReattacher
	if browserReattacher == nil {
		browserReattacher = browser.NewCDPReattacher()
	}
	smokeChecker := options.SmokeChecker
	if smokeChecker == nil {
		smokeChecker = browser.NewCDPSmokeChecker()
	}
	heartbeatInterval := options.HeartbeatInterval
	if heartbeatInterval <= 0 {
		heartbeatInterval = 30 * time.Second
	}
	return &Server{
		cwd:               cwd,
		env:               env,
		store:             store,
		browserRunner:     browserRunner,
		browserReattacher: browserReattacher,
		smokeChecker:      smokeChecker,
		heartbeatInterval: heartbeatInterval,
	}
}

func (s *Server) Serve(reader io.Reader, writer io.Writer) error {
	decoder := json.NewDecoder(reader)
	encoder := json.NewEncoder(writer)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		response := s.HandleForTest(raw)
		if response.ID == nil && response.Error == nil {
			continue
		}
		if err := encoder.Encode(response); err != nil {
			return err
		}
	}
}

func (s *Server) HandleForTest(raw json.RawMessage) RPCResponse {
	request, err := parseRequest(raw)
	if err != nil {
		return errorResponse(nil, -32700, err.Error())
	}
	if !request.hasID {
		return RPCResponse{JSONRPC: "2.0"}
	}
	switch request.method {
	case "initialize":
		return okResponse(request.id, map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities": map[string]any{
				"tools": map[string]any{"listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    "codex_oracle",
				"version": version,
			},
		})
	case "ping":
		return okResponse(request.id, map[string]any{})
	case "tools/list":
		return okResponse(request.id, map[string]any{"tools": tools()})
	case "tools/call":
		result, err := s.callTool(request.params)
		if err != nil {
			return errorResponse(request.id, -32602, err.Error())
		}
		return okResponse(request.id, result)
	case "shutdown":
		return okResponse(request.id, map[string]any{})
	default:
		return errorResponse(request.id, -32601, "method not found: "+request.method)
	}
}

func parseRequest(raw json.RawMessage) (rpcRequest, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return rpcRequest{}, err
	}
	var request rpcRequest
	if value, ok := fields["jsonrpc"]; ok {
		_ = json.Unmarshal(value, &request.jsonrpc)
	}
	if value, ok := fields["id"]; ok {
		request.id = append(json.RawMessage(nil), value...)
		request.hasID = true
	}
	if value, ok := fields["method"]; ok {
		_ = json.Unmarshal(value, &request.method)
	}
	if request.method == "" {
		return rpcRequest{}, errors.New("missing JSON-RPC method")
	}
	if value, ok := fields["params"]; ok {
		request.params = value
	} else {
		request.params = json.RawMessage(`{}`)
	}
	return request, nil
}

func (s *Server) callTool(params json.RawMessage) (any, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return nil, err
	}
	if len(call.Arguments) == 0 {
		call.Arguments = json.RawMessage(`{}`)
	}
	switch call.Name {
	case "consult":
		return s.callConsult(call.Arguments), nil
	case "consult_prepare":
		return s.callConsultPrepare(call.Arguments), nil
	case "consult_finalize":
		return s.callConsultFinalize(call.Arguments), nil
	case "consult_checkpoint":
		return s.callConsultCheckpoint(call.Arguments), nil
	case "login_setup":
		return s.callLoginSetup(call.Arguments), nil
	case "reattach":
		return s.callReattach(call.Arguments), nil
	case "sessions":
		return s.callSessions(call.Arguments), nil
	case "session_delete":
		return s.callSessionDelete(call.Arguments), nil
	case "smoke_check":
		return s.callSmokeCheck(call.Arguments), nil
	default:
		return toolError("unknown tool: " + call.Name), nil
	}
}

func (s *Server) callLoginSetup(arguments json.RawMessage) any {
	var input struct {
		ChromePath  string `json:"chromePath,omitempty"`
		ProfileDir  string `json:"profileDir,omitempty"`
		ChatGPTURL  string `json:"chatgptUrl,omitempty"`
		KeepBrowser *bool  `json:"keepBrowser,omitempty"`
		TimeoutMs   int    `json:"timeoutMs,omitempty"`
		DryRun      bool   `json:"dryRun,omitempty"`
	}
	if err := decodeStrict(arguments, &input); err != nil {
		return toolError(err.Error())
	}
	env := copyEnv(s.env)
	if strings.TrimSpace(input.ChromePath) != "" {
		env["ORACLE_BROWSER_CHROME_PATH"] = input.ChromePath
	}
	chromePath, chromeErr := browser.FindChromePath(env)
	profileDir := strings.TrimSpace(input.ProfileDir)
	if profileDir == "" {
		profileDir = strings.TrimSpace(env["ORACLE_BROWSER_PROFILE_DIR"])
	}
	if profileDir == "" {
		profileDir = filepath.Join(session.DefaultHome(), "browser-profile")
	}
	chatgptURL := strings.TrimSpace(input.ChatGPTURL)
	if chatgptURL == "" {
		chatgptURL = strings.TrimSpace(env["ORACLE_CHATGPT_URL"])
	}
	if chatgptURL == "" {
		chatgptURL = oracle.DefaultChatGPTURL
	}
	keepBrowser := true
	if input.KeepBrowser != nil {
		keepBrowser = *input.KeepBrowser
	}
	config := browser.LaunchConfig{
		ProfileDir:  profileDir,
		ChatGPTURL:  chatgptURL,
		KeepBrowser: keepBrowser,
	}
	launchArgs := browser.BuildLaunchArgs(config)
	checks := []browser.SmokeCheck{}
	if chromeErr != nil {
		checks = append(checks, browser.SmokeCheck{Name: "chrome", Status: "fail", Message: chromeErr.Error()})
	} else {
		checks = append(checks, browser.SmokeCheck{Name: "chrome", Status: "ok"})
	}
	if input.DryRun {
		output := "login_setup dry-run: Chrome will open ChatGPT with a private automation profile."
		if chromeErr != nil {
			output = "login_setup dry-run: Chrome is not ready; set ORACLE_BROWSER_CHROME_PATH before using -OpenBrowser."
		}
		return toolResult(output, map[string]any{
			"status":      "dry-run",
			"output":      output,
			"chromePath":  chromePath,
			"profileDir":  profileDir,
			"chatgptUrl":  chatgptURL,
			"keepBrowser": keepBrowser,
			"launchArgs":  launchArgs,
			"checks":      checks,
		})
	}
	if chromeErr != nil {
		return toolError(chromeErr.Error())
	}
	timeout := time.Duration(input.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	process, endpoint, err := browser.LaunchChromeAndWait(ctx, chromePath, config, 100*time.Millisecond)
	if err != nil {
		return toolError(err.Error())
	}
	if !keepBrowser {
		defer process.Kill()
	}
	output := "Chrome login setup is ready. Sign into ChatGPT in the opened browser profile, then retry consult."
	return toolResult(output, map[string]any{
		"status":      "ready",
		"output":      output,
		"chromePath":  chromePath,
		"profileDir":  profileDir,
		"chatgptUrl":  chatgptURL,
		"keepBrowser": keepBrowser,
		"endpoint":    endpoint,
		"pid":         process.PID(),
	})
}

func (s *Server) callSmokeCheck(arguments json.RawMessage) any {
	var input struct {
		Endpoint   string `json:"endpoint,omitempty"`
		ChromePath string `json:"chromePath,omitempty"`
		ProfileDir string `json:"profileDir,omitempty"`
		ChatGPTURL string `json:"chatgptUrl,omitempty"`
		TimeoutMs  int    `json:"timeoutMs,omitempty"`
		DryRun     bool   `json:"dryRun,omitempty"`
	}
	if err := decodeStrict(arguments, &input); err != nil {
		return toolError(err.Error())
	}
	env := copyEnv(s.env)
	if strings.TrimSpace(input.ChromePath) != "" {
		env["ORACLE_BROWSER_CHROME_PATH"] = input.ChromePath
	}
	profileDir := strings.TrimSpace(input.ProfileDir)
	if profileDir == "" {
		profileDir = strings.TrimSpace(env["ORACLE_BROWSER_PROFILE_DIR"])
	}
	if profileDir == "" {
		profileDir = filepath.Join(session.DefaultHome(), "browser-profile")
	}
	chatgptURL := strings.TrimSpace(input.ChatGPTURL)
	if chatgptURL == "" {
		chatgptURL = strings.TrimSpace(env["ORACLE_CHATGPT_URL"])
	}
	if chatgptURL == "" {
		chatgptURL = oracle.DefaultChatGPTURL
	}
	chromePath, chromeErr := browser.FindChromePath(env)
	endpoint := strings.TrimSpace(input.Endpoint)
	checks := []browser.SmokeCheck{}
	if chromeErr != nil {
		checks = append(checks, browser.SmokeCheck{Name: "chrome", Status: "fail", Message: chromeErr.Error()})
	} else {
		checks = append(checks, browser.SmokeCheck{Name: "chrome", Status: "ok"})
	}
	if endpoint == "" {
		if readEndpoint, err := browser.ReadDevToolsEndpoint(profileDir); err == nil {
			endpoint = readEndpoint
			checks = append(checks, browser.SmokeCheck{Name: "devtools-endpoint", Status: "ok"})
		} else {
			checks = append(checks, browser.SmokeCheck{Name: "devtools-endpoint", Status: "pending", Message: "run login_setup if no signed-in browser profile is active"})
		}
	} else {
		checks = append(checks, browser.SmokeCheck{Name: "devtools-endpoint", Status: "ok"})
	}
	if input.DryRun {
		output := "smoke_check dry-run: readiness inspected without launching Chrome."
		return toolResult(output, map[string]any{
			"status":     "dry-run",
			"output":     output,
			"chromePath": chromePath,
			"profileDir": profileDir,
			"chatgptUrl": chatgptURL,
			"endpoint":   endpoint,
			"checks":     checks,
		})
	}
	timeout := time.Duration(input.TimeoutMs) * time.Millisecond
	result, err := s.smokeChecker.Check(context.Background(), browser.SmokeRequest{
		Endpoint:   endpoint,
		ProfileDir: profileDir,
		ChatGPTURL: chatgptURL,
		Timeout:    timeout,
	})
	if err != nil {
		resultProfileDir := profileDir
		if strings.TrimSpace(result.ProfileDir) != "" {
			resultProfileDir = result.ProfileDir
		}
		resultChatGPTURL := chatgptURL
		if strings.TrimSpace(result.ChatGPTURL) != "" {
			resultChatGPTURL = result.ChatGPTURL
		}
		resultEndpoint := endpoint
		if strings.TrimSpace(result.Endpoint) != "" {
			resultEndpoint = result.Endpoint
		}
		resultChecks := checks
		if len(result.Checks) > 0 {
			resultChecks = result.Checks
		}
		return toolErrorWithStructured(err.Error(), map[string]any{
			"status":     "error",
			"output":     err.Error(),
			"chromePath": chromePath,
			"profileDir": resultProfileDir,
			"chatgptUrl": resultChatGPTURL,
			"endpoint":   resultEndpoint,
			"auth":       result.Auth,
			"checks":     resultChecks,
		})
	}
	status := "login-required"
	if result.Auth.Authenticated && result.Auth.ComposerVisible {
		status = "ready"
	}
	output := "smoke_check: " + status
	return toolResult(output, map[string]any{
		"status":     status,
		"output":     output,
		"profileDir": profileDir,
		"chatgptUrl": chatgptURL,
		"endpoint":   result.Endpoint,
		"auth":       result.Auth,
		"checks":     result.Checks,
	})
}

func (s *Server) callConsult(arguments json.RawMessage) any {
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
	attachmentPlan := buildAttachmentPlan(resolvedInput, rendered, files.Files)
	if resolvedInput.DryRun {
		output := oracle.FormatDryRun(resolved) +
			fmt.Sprintf("\n  bundle characters: %d\n  estimated tokens: %d", rendered.CharacterCount, rendered.EstimatedTokens)
		return toolResult(output, map[string]any{
			"status":   "dry-run",
			"output":   output,
			"dryRun":   true,
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
	if resolved.ResolvedEngine != "browser" {
		return toolError("API live consult is not implemented in this Go plugin; use dryRun:true or browser mode.")
	}
	if !s.legacyCDPEnabled() {
		return toolError("legacy direct-CDP live consult is disabled in the preview release; use consult_prepare with @Chrome and then consult_finalize.")
	}
	browserConfig := resolved.Browser
	if browserConfig == nil {
		return toolError("browser configuration did not resolve")
	}
	attachmentArtifacts, err := s.createAttachmentArtifacts(resolvedInput, attachmentPlan, files.Files)
	if err != nil {
		return toolError(err.Error())
	}
	attachmentPaths := make([]string, 0, len(attachmentArtifacts))
	for _, artifact := range attachmentArtifacts {
		attachmentPaths = append(attachmentPaths, artifact.Path)
	}
	submitText := rendered.Text
	if len(attachmentPaths) > 0 {
		submitText = resolvedInput.Prompt
	}
	followUps := normalizedFollowUps(resolvedInput.BrowserFollowUps)
	request := map[string]any{
		"model":          resolved.Model,
		"engine":         resolved.ResolvedEngine,
		"files":          filePaths,
		"bundleText":     rendered.Text,
		"attachmentPlan": attachmentPlan,
	}
	if len(followUps) > 0 {
		request["followUps"] = followUps
	}
	if len(attachmentArtifacts) > 0 {
		request["attachments"] = attachmentArtifacts
	}
	meta, err := s.store.Create(session.CreateOptions{
		Prompt:  resolvedInput.Prompt,
		Model:   resolved.Model,
		Mode:    resolved.ResolvedEngine,
		CWD:     workspaceRoot,
		Status:  "running",
		Slug:    resolvedInput.Slug,
		Request: request,
		Output:  "",
	})
	if err != nil {
		return toolError(err.Error())
	}
	stopHeartbeat := s.startHeartbeat(meta.ID)
	defer stopHeartbeat()
	runResult, err := s.browserRunner.Run(context.Background(), browser.Request{
		Prompt:               resolvedInput.Prompt,
		BundleText:           submitText,
		FollowUps:            followUps,
		AttachmentPaths:      attachmentPaths,
		Model:                resolved.Model,
		DesiredModelLabel:    browserConfig.DesiredModel,
		BrowserThinkingTime:  browserConfig.ThinkingTime,
		BrowserModelStrategy: browserConfig.ModelStrategy,
		ProfileDir:           browserConfig.ProfileDir,
		ChatGPTURL:           browserConfig.ChatGPTURL,
		KeepBrowser:          browserConfig.KeepBrowser,
	})
	if err != nil {
		_ = s.store.UpdateResult(meta.ID, session.UpdateOptions{
			Status: "failed",
			Output: err.Error(),
		})
		return toolError(err.Error())
	}
	if err := s.store.UpdateResult(meta.ID, session.UpdateOptions{
		Status:     "completed",
		Output:     runResult.Answer,
		Transcript: runResult.Transcript,
		Runtime:    runResult.Runtime,
	}); err != nil {
		return toolError(err.Error())
	}
	return toolResult("Session "+meta.ID+" (completed)\n"+runResult.Answer, map[string]any{
		"sessionId": meta.ID,
		"status":    "completed",
		"output":    runResult.Answer,
		"runtime":   runResult.Runtime,
	})
}

func (s *Server) legacyCDPEnabled() bool {
	value := strings.ToLower(strings.TrimSpace(s.env["ORACLE_ENABLE_LEGACY_CDP"]))
	return value == "1" || value == "true" || value == "yes"
}

func (s *Server) resolveWorkspaceRoot(requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return s.cwd, nil
	}
	workspaceRoot, err := filepath.Abs(requested)
	if err != nil {
		return "", fmt.Errorf("workspaceRoot %q could not be resolved: %w", requested, err)
	}
	info, err := os.Stat(workspaceRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("workspaceRoot %q does not exist", workspaceRoot)
		}
		return "", fmt.Errorf("workspaceRoot %q cannot be accessed: %w", workspaceRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspaceRoot %q is not a directory", workspaceRoot)
	}
	return workspaceRoot, nil
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

func (s *Server) startHeartbeat(sessionID string) func() {
	if s.heartbeatInterval <= 0 {
		return func() {}
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(s.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				_ = s.store.UpdateHeartbeat(sessionID)
			case <-stop:
				return
			}
		}
	}()
	return func() {
		close(stop)
		<-done
	}
}

func (s *Server) callSessions(arguments json.RawMessage) any {
	var input struct {
		ID         string  `json:"id,omitempty"`
		Hours      float64 `json:"hours,omitempty"`
		Limit      int     `json:"limit,omitempty"`
		IncludeAll bool    `json:"includeAll,omitempty"`
		Detail     bool    `json:"detail,omitempty"`
	}
	if err := decodeStrict(arguments, &input); err != nil {
		return toolError(err.Error())
	}
	if input.ID != "" {
		if input.Detail {
			detail, err := s.store.ReadDetail(input.ID)
			if err != nil {
				return toolError(fmt.Sprintf("Session %q not found.", input.ID))
			}
			return toolResult(detail.Log, map[string]any{"session": detail})
		}
		meta, err := s.store.Read(input.ID)
		if err != nil {
			return toolError(fmt.Sprintf("Session %q not found.", input.ID))
		}
		line := sessionLine(*meta)
		return toolResult(line, map[string]any{
			"entries":   []session.Metadata{*meta},
			"total":     1,
			"truncated": false,
		})
	}
	limit := input.Limit
	if limit == 0 {
		limit = 100
	}
	entries, err := s.store.List(session.ListOptions{
		Hours:      input.Hours,
		Limit:      limit,
		IncludeAll: input.IncludeAll,
	})
	if err != nil {
		return toolError(err.Error())
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, sessionLine(entry))
	}
	return toolResult(strings.Join(lines, "\n"), map[string]any{
		"entries":   entries,
		"total":     len(entries),
		"truncated": false,
	})
}

func (s *Server) callSessionDelete(arguments json.RawMessage) any {
	var input struct {
		ID string `json:"id"`
	}
	if err := decodeStrict(arguments, &input); err != nil {
		return toolError(err.Error())
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return toolError("Session id is required.")
	}
	if _, err := s.store.Read(id); err != nil {
		return toolError(fmt.Sprintf("Session %q not found.", id))
	}
	if err := s.store.Delete(id); err != nil {
		return toolError(err.Error())
	}
	output := "Session " + id + " deleted."
	return toolResult(output, map[string]any{
		"sessionId": id,
		"deleted":   true,
		"output":    output,
	})
}

func (s *Server) callReattach(arguments json.RawMessage) any {
	var input struct {
		ID        string `json:"id"`
		DryRun    *bool  `json:"dryRun,omitempty"`
		TimeoutMs int    `json:"timeoutMs,omitempty"`
	}
	if err := decodeStrict(arguments, &input); err != nil {
		return toolError(err.Error())
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		return toolError("Session id is required.")
	}
	detail, err := s.store.ReadDetail(id)
	if err != nil {
		return toolError(fmt.Sprintf("Session %q not found.", id))
	}
	if detail.Metadata.Mode == string(oracle.ExecutionModeChromeAssisted) || detail.Request["executionMode"] == string(oracle.ExecutionModeChromeAssisted) {
		return chromeRecoveryPlan(detail)
	}
	status := buildReattachStatus(detail)
	dryRun := true
	if input.DryRun != nil {
		dryRun = *input.DryRun
	}
	if !dryRun {
		if !status.Reattachable {
			return toolError("session is not reattachable: " + strings.Join(status.Reasons, "; "))
		}
		timeout := time.Duration(input.TimeoutMs) * time.Millisecond
		result, err := s.browserReattacher.Reattach(context.Background(), browser.ReattachRequest{
			Endpoint:        status.Runtime.Endpoint,
			TargetID:        status.Runtime.TargetID,
			ConversationURL: status.Runtime.ConversationURL,
			ProfileDir:      status.Runtime.ProfileDir,
			ChatGPTURL:      status.Runtime.ChatGPTURL,
			Timeout:         timeout,
		})
		if err != nil {
			return toolError(err.Error())
		}
		if err := s.store.UpdateResult(detail.Metadata.ID, session.UpdateOptions{
			Status:     "completed",
			Output:     result.Answer,
			Transcript: result.Transcript,
			Runtime:    result.Runtime,
		}); err != nil {
			return toolError(err.Error())
		}
		return toolResult("Session "+detail.Metadata.ID+" (completed)\n"+result.Answer, map[string]any{
			"sessionId": detail.Metadata.ID,
			"status":    "completed",
			"output":    result.Answer,
			"runtime":   result.Runtime,
		})
	}
	output := formatReattachDryRun(status)
	return toolResult(output, map[string]any{
		"status":       "dry-run",
		"output":       output,
		"sessionId":    detail.Metadata.ID,
		"session":      detail.Metadata,
		"reattachable": status.Reattachable,
		"reasons":      status.Reasons,
		"runtime":      status.Runtime,
		"plan": map[string]any{
			"steps": status.Steps,
		},
	})
}

func tools() []map[string]any {
	objectSchema := map[string]any{"type": "object"}
	return []map[string]any{
		{
			"name":        "consult_checkpoint",
			"title":       "Save the review conversation URL",
			"description": "After sending the prepared review in Chrome, save its conversation URL immediately, before waiting for the answer. Keeps the session running and stores no prompt or answer. Requires the original handoff identity; retrying the same URL is safe.",
			"inputSchema": map[string]any{
				"type": "object", "additionalProperties": false,
				"properties": map[string]any{
					"sessionId":       map[string]any{"type": "string"},
					"handoffDigest":   map[string]any{"type": "string"},
					"handoffNonce":    map[string]any{"type": "string"},
					"conversationUrl": map[string]any{"type": "string"},
				},
				"required": []string{"sessionId", "handoffDigest", "handoffNonce", "conversationUrl"},
			},
			"outputSchema": objectSchema,
		},
		{
			"name":        "consult",
			"title":       "Run an oracle consult",
			"description": "Preview an Oracle-style second-opinion consult. Legacy direct-CDP live execution is disabled by default; use consult_prepare with @Chrome for live review.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"preset":               enumSchema("chatgpt-pro-heavy"),
					"prompt":               map[string]any{"type": "string"},
					"files":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"workspaceRoot":        map[string]any{"type": "string"},
					"model":                map[string]any{"type": "string"},
					"models":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"engine":               enumSchema("api", "browser"),
					"browserThinkingTime":  enumSchema("light", "standard", "extended", "heavy"),
					"browserModelStrategy": enumSchema("select", "current", "ignore"),
					"browserAttachments":   enumSchema("auto", "never", "always"),
					"dryRun":               map[string]any{"type": "boolean"},
					"slug":                 map[string]any{"type": "string"},
				},
				"required": []string{"prompt"},
			},
			"outputSchema": objectSchema,
		},
		{
			"name":        "consult_prepare",
			"title":       "Prepare a Chrome-assisted oracle consult",
			"description": "Resolve files, render the prompt bundle, create a running session, and return a handoff packet for the bundled Chrome plugin. This tool does not open Chrome.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"preset":               enumSchema("chatgpt-pro-heavy"),
					"prompt":               map[string]any{"type": "string"},
					"files":                map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"workspaceRoot":        map[string]any{"type": "string"},
					"model":                map[string]any{"type": "string"},
					"models":               map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"engine":               enumSchema("browser"),
					"browserThinkingTime":  enumSchema("light", "standard", "extended", "heavy"),
					"browserModelStrategy": enumSchema("select", "current", "ignore"),
					"browserAttachments":   enumSchema("auto", "never", "always"),
					"browserBundleFiles":   map[string]any{"type": "boolean"},
					"browserBundleFormat":  enumSchema("text", "zip"),
					"browserFollowUps":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"slug":                 map[string]any{"type": "string"},
				},
				"required": []string{"prompt"},
			},
			"outputSchema": objectSchema,
		},
		{
			"name":        "consult_finalize",
			"title":       "Finalize a Chrome-assisted oracle consult",
			"description": "Record a captured Chrome answer or bounded failure diagnostic into a prepared Oracle session.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"sessionId":     map[string]any{"type": "string"},
					"status":        enumSchema("completed", "failed"),
					"handoffDigest": map[string]any{"type": "string"},
					"handoffNonce":  map[string]any{"type": "string"},
					"force":         map[string]any{"type": "boolean"},
					"answer":        map[string]any{"type": "string"},
					"error":         map[string]any{"type": "string"},
					"transcript":    map[string]any{"type": "string"},
					"runtime":       objectSchema,
				},
				"required": []string{"sessionId", "status"},
			},
			"outputSchema": objectSchema,
		},
		{
			"name":        "sessions",
			"title":       "List or fetch oracle sessions",
			"description": "Inspect codex_oracle sessions stored under ORACLE_HOME_DIR or ~/.codex_oracle.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"id":         map[string]any{"type": "string"},
					"hours":      map[string]any{"type": "number"},
					"limit":      map[string]any{"type": "number"},
					"includeAll": map[string]any{"type": "boolean"},
					"detail":     map[string]any{"type": "boolean"},
				},
			},
			"outputSchema": objectSchema,
		},
		{
			"name":        "session_delete",
			"title":       "Delete an oracle session",
			"description": "Delete one local codex_oracle session folder and its artifacts after the result has been returned to Codex.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"id": map[string]any{"type": "string"},
				},
				"required": []string{"id"},
			},
			"outputSchema": objectSchema,
		},
		{
			"name":        "login_setup",
			"title":       "Open ChatGPT login browser",
			"description": "Legacy diagnostic: launch Chrome with a private codex_oracle profile. Preferred live use goes through the external @Chrome plugin.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"chromePath":  map[string]any{"type": "string"},
					"profileDir":  map[string]any{"type": "string"},
					"chatgptUrl":  map[string]any{"type": "string"},
					"keepBrowser": map[string]any{"type": "boolean"},
					"timeoutMs":   map[string]any{"type": "number"},
					"dryRun":      map[string]any{"type": "boolean"},
				},
			},
			"outputSchema": objectSchema,
		},
		{
			"name":        "smoke_check",
			"title":       "Check ChatGPT browser readiness",
			"description": "Inspect Chrome/profile/CDP readiness and optionally verify ChatGPT auth/composer state without launching a new visible browser.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"endpoint":   map[string]any{"type": "string"},
					"chromePath": map[string]any{"type": "string"},
					"profileDir": map[string]any{"type": "string"},
					"chatgptUrl": map[string]any{"type": "string"},
					"timeoutMs":  map[string]any{"type": "number"},
					"dryRun":     map[string]any{"type": "boolean"},
				},
			},
			"outputSchema": objectSchema,
		},
		{
			"name":        "reattach",
			"title":       "Inspect an oracle session reattach plan",
			"description": "Recover a session: Chrome-assisted sessions return a read-only @Chrome capture plan and finalize identity, even with dryRun:false. Legacy CDP sessions return a dry-run plan by default or capture with dryRun:false.",
			"inputSchema": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"id":        map[string]any{"type": "string"},
					"dryRun":    map[string]any{"type": "boolean"},
					"timeoutMs": map[string]any{"type": "number"},
				},
				"required": []string{"id"},
			},
			"outputSchema": objectSchema,
		},
	}
}

type reattachRuntime struct {
	PID             any    `json:"pid,omitempty"`
	Endpoint        string `json:"endpoint,omitempty"`
	TargetID        string `json:"targetId,omitempty"`
	ConversationURL string `json:"conversationUrl,omitempty"`
	ProfileDir      string `json:"profileDir,omitempty"`
	ChatGPTURL      string `json:"chatgptUrl,omitempty"`
	LastActivityAt  string `json:"lastActivityAt,omitempty"`
}

type reattachStatus struct {
	Reattachable bool             `json:"reattachable"`
	Reasons      []string         `json:"reasons"`
	Runtime      reattachRuntime  `json:"runtime"`
	Steps        []string         `json:"steps"`
	Session      session.Metadata `json:"session"`
}

func buildReattachStatus(detail *session.Detail) reattachStatus {
	runtime := normalizeReattachRuntime(detail.Metadata.Runtime)
	reasons := []string{}
	if runtime.Endpoint == "" {
		reasons = append(reasons, "missing browser DevTools endpoint")
	}
	if runtime.TargetID == "" && runtime.ConversationURL == "" {
		reasons = append(reasons, "missing target id or conversation URL")
	}
	if runtime.ProfileDir == "" {
		reasons = append(reasons, "missing browser profile directory")
	}
	reattachable := len(reasons) == 0
	steps := []string{
		"Check whether the stored DevTools endpoint still accepts connections.",
		"Open the stored ChatGPT target id, or navigate to the stored conversation URL.",
		"Capture the latest assistant message and update the session artifacts.",
	}
	if !reattachable {
		steps = []string{
			"Run login_setup to prepare a signed-in private ChatGPT profile.",
			"Rerun consult if the original browser runtime is no longer available.",
		}
	}
	return reattachStatus{
		Reattachable: reattachable,
		Reasons:      reasons,
		Runtime:      runtime,
		Steps:        steps,
		Session:      detail.Metadata,
	}
}

func normalizeReattachRuntime(values map[string]any) reattachRuntime {
	return reattachRuntime{
		PID:             values["pid"],
		Endpoint:        stringValue(values["endpoint"]),
		TargetID:        stringValue(values["targetId"]),
		ConversationURL: stringValue(values["conversationUrl"]),
		ProfileDir:      stringValue(values["profileDir"]),
		ChatGPTURL:      stringValue(values["chatgptUrl"]),
		LastActivityAt:  stringValue(values["lastActivityAt"]),
	}
}

func formatReattachDryRun(status reattachStatus) string {
	state := "not reattachable"
	if status.Reattachable {
		state = "reattachable"
	}
	lines := []string{
		"reattach dry-run: " + status.Session.ID + " is " + state + ".",
	}
	if status.Runtime.Endpoint != "" {
		lines = append(lines, "  endpoint: "+status.Runtime.Endpoint)
	}
	if status.Runtime.TargetID != "" {
		lines = append(lines, "  target id: "+status.Runtime.TargetID)
	}
	if status.Runtime.ConversationURL != "" {
		lines = append(lines, "  conversation: "+status.Runtime.ConversationURL)
	}
	for _, reason := range status.Reasons {
		lines = append(lines, "  missing: "+reason)
	}
	return strings.Join(lines, "\n")
}

func stringValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return ""
	}
}

func toolResult(text string, structured map[string]any) map[string]any {
	return map[string]any{
		"content": []map[string]string{
			{"type": "text", "text": text},
		},
		"structuredContent": structured,
	}
}

func buildAttachmentPlan(input oracle.ConsultInput, rendered bundle.RenderedBundle, files []bundle.File) map[string]any {
	mode := "inline"
	reason := "text bundle fits inline dry-run preview"
	if input.BrowserAttachments == "always" || input.BrowserBundleFiles || input.BrowserBundleFormat == "zip" {
		mode = "zip"
		reason = "request forces browser attachment bundle"
	} else if rendered.CharacterCount > 60000 {
		mode = "text"
		reason = "text bundle exceeds inline threshold"
	}
	return map[string]any{
		"mode":            mode,
		"reason":          reason,
		"fileCount":       len(files),
		"characterCount":  rendered.CharacterCount,
		"estimatedTokens": rendered.EstimatedTokens,
	}
}

func (s *Server) createAttachmentArtifacts(input oracle.ConsultInput, plan map[string]any, files []bundle.File) ([]bundle.BundleArtifact, error) {
	mode, _ := plan["mode"].(string)
	if mode == "" || mode == "inline" {
		return nil, nil
	}
	timestamp := time.Now().UTC().Format("20060102T150405.000000000Z")
	bundlesDir := filepath.Join(s.store.HomeDir(), "artifacts", "bundles")
	switch mode {
	case "text":
		artifact, err := bundle.CreateTextBundle(filepath.Join(bundlesDir, "context-"+timestamp+".md"), input.Prompt, files)
		if err != nil {
			return nil, err
		}
		return []bundle.BundleArtifact{artifact}, nil
	case "zip":
		artifact, err := bundle.CreateZipBundle(filepath.Join(bundlesDir, "context-"+timestamp+".zip"), files)
		if err != nil {
			return nil, err
		}
		return []bundle.BundleArtifact{artifact}, nil
	default:
		return nil, fmt.Errorf("unsupported attachment mode %q", mode)
	}
}

func toolError(message string) map[string]any {
	return toolErrorWithStructured(message, map[string]any{"status": "error", "output": message})
}

func toolErrorWithStructured(message string, structured map[string]any) map[string]any {
	if structured == nil {
		structured = map[string]any{}
	}
	if _, ok := structured["status"]; !ok {
		structured["status"] = "error"
	}
	if _, ok := structured["output"]; !ok {
		structured["output"] = message
	}
	result := toolResult(message, structured)
	result["isError"] = true
	return result
}

func enumSchema(values ...string) map[string]any {
	items := make([]any, 0, len(values))
	for _, value := range values {
		items = append(items, value)
	}
	return map[string]any{"type": "string", "enum": items}
}

func sessionLine(meta session.Metadata) string {
	model := meta.Model
	if model == "" {
		model = "n/a"
	}
	return meta.CreatedAt + " | " + meta.Status + " | " + model + " | " + meta.ID
}

func decodeStrict(data json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return nil
}

func okResponse(id json.RawMessage, result any) RPCResponse {
	return RPCResponse{JSONRPC: "2.0", ID: json.RawMessage(id), Result: result}
}

func errorResponse(id json.RawMessage, code int, message string) RPCResponse {
	var responseID any
	if len(id) > 0 {
		responseID = json.RawMessage(id)
	}
	return RPCResponse{JSONRPC: "2.0", ID: responseID, Error: &RPCError{Code: code, Message: message}}
}

func environMap() map[string]string {
	env := map[string]string{}
	for _, pair := range os.Environ() {
		key, value, ok := strings.Cut(pair, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func copyEnv(env map[string]string) map[string]string {
	copied := map[string]string{}
	for key, value := range env {
		copied[key] = value
	}
	return copied
}
