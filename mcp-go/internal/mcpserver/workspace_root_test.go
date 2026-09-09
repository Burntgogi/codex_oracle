package mcpserver_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex_oracle/internal/mcpserver"
	"codex_oracle/internal/session"
)

func TestConsultToolsExposeWorkspaceRootString(t *testing.T) {
	server := mcpserver.New(mcpserver.Options{Store: session.NewStore(t.TempDir())})
	response := server.HandleForTest(json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	if response.Error != nil {
		t.Fatalf("tools/list error = %#v", response.Error)
	}

	var payload struct {
		Tools []struct {
			Name        string `json:"name"`
			InputSchema struct {
				Properties map[string]map[string]any `json:"properties"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	mustMarshalResult(t, response.Result, &payload)

	for _, name := range []string{"consult", "consult_prepare"} {
		tool := workspaceRootTool(t, payload.Tools, name)
		property, ok := tool.InputSchema.Properties["workspaceRoot"]
		if !ok {
			t.Fatalf("%s input schema has no workspaceRoot property", name)
		}
		if property["type"] != "string" {
			t.Fatalf("%s workspaceRoot schema = %#v, want string", name, property)
		}
	}
}

func TestConsultUsesExplicitWorkspaceRoot(t *testing.T) {
	t.Setenv("ORACLE_ENABLE_LEGACY_CDP", "1")
	pluginRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	workspaceFile := filepath.Join(workspaceRoot, "plan.md")
	writeWorkspaceRootFile(t, workspaceFile, "workspace plan")
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: pluginRoot, Store: store, BrowserRunner: fakeRunner{}})

	payload := workspaceRootCall(t, server, "consult", map[string]any{
		"preset":        "chatgpt-pro-heavy",
		"prompt":        "review",
		"files":         []string{workspaceFile},
		"workspaceRoot": workspaceRoot,
	})

	if payload.IsError {
		t.Fatalf("consult returned error: %s", payload.StructuredContent.Output)
	}
	detail, err := store.ReadDetail(payload.StructuredContent.SessionID)
	if err != nil {
		t.Fatalf("ReadDetail: %v", err)
	}
	if detail.Metadata.CWD != workspaceRoot {
		t.Fatalf("session cwd = %q, want %q", detail.Metadata.CWD, workspaceRoot)
	}
	if got, ok := detail.Request["files"].([]any); !ok || len(got) != 1 || got[0] != "plan.md" {
		t.Fatalf("session files = %#v, want [plan.md]", detail.Request["files"])
	}
}

func TestConsultPrepareUsesExplicitWorkspaceRootForSessionCWD(t *testing.T) {
	pluginRoot := t.TempDir()
	workspaceRoot := t.TempDir()
	workspaceFile := filepath.Join(workspaceRoot, "plan.md")
	writeWorkspaceRootFile(t, workspaceFile, "workspace plan")
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: pluginRoot, Store: store})

	payload := workspaceRootCall(t, server, "consult_prepare", map[string]any{
		"preset":        "chatgpt-pro-heavy",
		"prompt":        "review",
		"files":         []string{workspaceFile},
		"workspaceRoot": workspaceRoot,
	})

	if payload.IsError {
		t.Fatalf("consult_prepare returned error: %s", payload.StructuredContent.Output)
	}
	if got := payload.StructuredContent.Resolved.Files; len(got) != 1 || got[0] != "plan.md" {
		t.Fatalf("resolved files = %#v, want [plan.md]", got)
	}
	detail, err := store.ReadDetail(payload.StructuredContent.SessionID)
	if err != nil {
		t.Fatalf("ReadDetail: %v", err)
	}
	if detail.Metadata.CWD != workspaceRoot {
		t.Fatalf("session cwd = %q, want %q", detail.Metadata.CWD, workspaceRoot)
	}
}

func TestConsultWithoutWorkspaceRootUsesServerCWD(t *testing.T) {
	pluginRoot := t.TempDir()
	writeWorkspaceRootFile(t, filepath.Join(pluginRoot, "plugin.md"), "plugin file")
	server := mcpserver.New(mcpserver.Options{CWD: pluginRoot, Store: session.NewStore(t.TempDir())})

	payload := workspaceRootCall(t, server, "consult", map[string]any{
		"preset": "chatgpt-pro-heavy",
		"prompt": "review",
		"files":  []string{"plugin.md"},
		"dryRun": true,
	})

	if payload.IsError {
		t.Fatalf("consult returned error: %s", payload.StructuredContent.Output)
	}
	if got := payload.StructuredContent.Resolved.Files; len(got) != 1 || got[0] != "plugin.md" {
		t.Fatalf("resolved files = %#v, want [plugin.md]", got)
	}
}

func TestConsultLiveWithoutWorkspaceRootStoresServerCWD(t *testing.T) {
	t.Setenv("ORACLE_ENABLE_LEGACY_CDP", "1")
	pluginRoot := t.TempDir()
	writeWorkspaceRootFile(t, filepath.Join(pluginRoot, "plugin.md"), "plugin file")
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: pluginRoot, Store: store, BrowserRunner: fakeRunner{}})

	payload := workspaceRootCall(t, server, "consult", map[string]any{
		"preset": "chatgpt-pro-heavy",
		"prompt": "review",
		"files":  []string{"plugin.md"},
	})

	if payload.IsError {
		t.Fatalf("consult returned error: %s", payload.StructuredContent.Output)
	}
	detail, err := store.ReadDetail(payload.StructuredContent.SessionID)
	if err != nil {
		t.Fatalf("ReadDetail: %v", err)
	}
	if detail.Metadata.CWD != pluginRoot {
		t.Fatalf("session cwd = %q, want server cwd %q", detail.Metadata.CWD, pluginRoot)
	}
}

func TestConsultRejectsNullWorkspaceRoot(t *testing.T) {
	server := mcpserver.New(mcpserver.Options{CWD: t.TempDir(), Store: session.NewStore(t.TempDir())})

	payload := workspaceRootCall(t, server, "consult", map[string]any{
		"preset":        "chatgpt-pro-heavy",
		"prompt":        "review",
		"workspaceRoot": nil,
		"dryRun":        true,
	})

	if !payload.IsError {
		t.Fatal("consult accepted workspaceRoot:null")
	}
	if !strings.Contains(payload.StructuredContent.Output, "workspaceRoot") || !strings.Contains(payload.StructuredContent.Output, "string") {
		t.Fatalf("error = %q, want workspaceRoot string error", payload.StructuredContent.Output)
	}
}

func TestConsultRejectsInvalidWorkspaceRoots(t *testing.T) {
	root := t.TempDir()
	notDirectory := filepath.Join(root, "not-a-directory")
	writeWorkspaceRootFile(t, notDirectory, "file")
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: session.NewStore(t.TempDir())})

	for _, test := range []struct {
		name          string
		workspaceRoot string
		want          string
	}{
		{name: "nonexistent", workspaceRoot: filepath.Join(root, "missing"), want: "does not exist"},
		{name: "not a directory", workspaceRoot: notDirectory, want: "not a directory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := workspaceRootCall(t, server, "consult", map[string]any{
				"preset":        "chatgpt-pro-heavy",
				"prompt":        "review",
				"workspaceRoot": test.workspaceRoot,
				"dryRun":        true,
			})

			if !payload.IsError {
				t.Fatal("consult returned no error for invalid workspaceRoot")
			}
			if !strings.Contains(payload.StructuredContent.Output, "workspaceRoot") || !strings.Contains(payload.StructuredContent.Output, test.want) {
				t.Fatalf("error = %q, want workspaceRoot %s error", payload.StructuredContent.Output, test.want)
			}
		})
	}
}

func TestConsultIgnoresFilesOutsideExplicitWorkspaceRoot(t *testing.T) {
	workspaceRoot := t.TempDir()
	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "outside.md")
	writeWorkspaceRootFile(t, outsideFile, "outside file")
	server := mcpserver.New(mcpserver.Options{CWD: t.TempDir(), Store: session.NewStore(t.TempDir())})

	payload := workspaceRootCall(t, server, "consult", map[string]any{
		"preset":        "chatgpt-pro-heavy",
		"prompt":        "review",
		"files":         []string{outsideFile},
		"workspaceRoot": workspaceRoot,
		"dryRun":        true,
	})

	if payload.IsError {
		t.Fatalf("consult returned error: %s", payload.StructuredContent.Output)
	}
	if got := payload.StructuredContent.Resolved.Files; len(got) != 0 {
		t.Fatalf("resolved files = %#v, want none", got)
	}
	if len(payload.StructuredContent.Ignored) != 1 || payload.StructuredContent.Ignored[0].Reason != "outside root" {
		t.Fatalf("ignored = %#v, want outside-root file", payload.StructuredContent.Ignored)
	}
}

func TestConsultPrepareRejectsInvalidWorkspaceRoots(t *testing.T) {
	root := t.TempDir()
	notDirectory := filepath.Join(root, "not-a-directory")
	writeWorkspaceRootFile(t, notDirectory, "file")
	server := mcpserver.New(mcpserver.Options{CWD: root, Store: session.NewStore(t.TempDir())})

	for _, test := range []struct {
		name          string
		workspaceRoot string
		want          string
	}{
		{name: "nonexistent", workspaceRoot: filepath.Join(root, "missing"), want: "does not exist"},
		{name: "not a directory", workspaceRoot: notDirectory, want: "not a directory"},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload := workspaceRootCall(t, server, "consult_prepare", map[string]any{
				"preset":        "chatgpt-pro-heavy",
				"prompt":        "review",
				"workspaceRoot": test.workspaceRoot,
			})

			if !payload.IsError {
				t.Fatal("consult_prepare returned no error for invalid workspaceRoot")
			}
			if !strings.Contains(payload.StructuredContent.Output, "workspaceRoot") || !strings.Contains(payload.StructuredContent.Output, test.want) {
				t.Fatalf("error = %q, want workspaceRoot %s error", payload.StructuredContent.Output, test.want)
			}
		})
	}
}

func TestConsultPrepareIgnoresFilesOutsideExplicitWorkspaceRoot(t *testing.T) {
	workspaceRoot := t.TempDir()
	outsideRoot := t.TempDir()
	outsideFile := filepath.Join(outsideRoot, "outside.md")
	writeWorkspaceRootFile(t, outsideFile, "outside file")
	server := mcpserver.New(mcpserver.Options{CWD: t.TempDir(), Store: session.NewStore(t.TempDir())})

	payload := workspaceRootCall(t, server, "consult_prepare", map[string]any{
		"preset":        "chatgpt-pro-heavy",
		"prompt":        "review",
		"files":         []string{outsideFile},
		"workspaceRoot": workspaceRoot,
	})

	if payload.IsError {
		t.Fatalf("consult_prepare returned error: %s", payload.StructuredContent.Output)
	}
	if got := payload.StructuredContent.Resolved.Files; len(got) != 0 {
		t.Fatalf("resolved files = %#v, want none", got)
	}
	if len(payload.StructuredContent.Ignored) != 1 || payload.StructuredContent.Ignored[0].Reason != "outside root" {
		t.Fatalf("ignored = %#v, want outside-root file", payload.StructuredContent.Ignored)
	}
}

type workspaceRootToolSchema struct {
	Name        string `json:"name"`
	InputSchema struct {
		Properties map[string]map[string]any `json:"properties"`
	} `json:"inputSchema"`
}

func workspaceRootTool(t *testing.T, tools []struct {
	Name        string `json:"name"`
	InputSchema struct {
		Properties map[string]map[string]any `json:"properties"`
	} `json:"inputSchema"`
}, name string) workspaceRootToolSchema {
	t.Helper()
	for _, tool := range tools {
		if tool.Name == name {
			return workspaceRootToolSchema{Name: tool.Name, InputSchema: tool.InputSchema}
		}
	}
	t.Fatalf("tools has no %q", name)
	return workspaceRootToolSchema{}
}

type workspaceRootPayload struct {
	IsError           bool `json:"isError"`
	StructuredContent struct {
		Output    string `json:"output"`
		SessionID string `json:"sessionId"`
		Resolved  struct {
			Files []string `json:"files"`
		} `json:"resolved"`
		Ignored []struct {
			Reason string `json:"reason"`
		} `json:"ignored"`
	} `json:"structuredContent"`
}

func workspaceRootCall(t *testing.T, server *mcpserver.Server, tool string, arguments map[string]any) workspaceRootPayload {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      tool,
			"arguments": arguments,
		},
	})
	if err != nil {
		t.Fatalf("Marshal request: %v", err)
	}
	response := server.HandleForTest(raw)
	if response.Error != nil {
		t.Fatalf("tools/call error = %#v", response.Error)
	}
	var payload workspaceRootPayload
	mustMarshalResult(t, response.Result, &payload)
	return payload
}

func writeWorkspaceRootFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
