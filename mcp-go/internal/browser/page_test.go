package browser_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"codex_oracle/internal/browser"
)

type fakeCDP struct {
	calls          []string
	uploadedFiles  []string
	querySelectors []string
}

func (f *fakeCDP) Call(_ context.Context, method string, params any) (json.RawMessage, error) {
	f.calls = append(f.calls, method)
	if method == "Target.createTarget" {
		return json.RawMessage(`{"targetId":"target-1"}`), nil
	}
	if method == "Target.attachToTarget" {
		return json.RawMessage(`{"sessionId":"session-1"}`), nil
	}
	return json.RawMessage(`{}`), nil
}

func (f *fakeCDP) CallSession(_ context.Context, sessionID string, method string, params any) (json.RawMessage, error) {
	f.calls = append(f.calls, sessionID+":"+method)
	switch method {
	case "Runtime.evaluate":
		return json.RawMessage(`{"result":{"type":"string","value":"ok"}}`), nil
	case "DOM.getDocument":
		return json.RawMessage(`{"root":{"nodeId":11}}`), nil
	case "DOM.querySelector":
		if values, ok := params.(map[string]any); ok {
			if selector, ok := values["selector"].(string); ok {
				f.querySelectors = append(f.querySelectors, selector)
			}
		}
		return json.RawMessage(`{"nodeId":22}`), nil
	case "DOM.setFileInputFiles":
		if values, ok := params.(map[string]any); ok {
			if files, ok := values["files"].([]string); ok {
				f.uploadedFiles = append([]string{}, files...)
			}
		}
		return json.RawMessage(`{}`), nil
	default:
		return json.RawMessage(`{}`), nil
	}
}

func TestOpenPageCreatesTargetAndEnablesDomains(t *testing.T) {
	cdp := &fakeCDP{}

	page, err := browser.OpenPage(context.Background(), cdp, "https://chatgpt.com/")
	if err != nil {
		t.Fatalf("OpenPage returned error: %v", err)
	}

	if page.TargetID != "target-1" {
		t.Fatalf("TargetID = %q, want target-1", page.TargetID)
	}
	if page.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want session-1", page.SessionID)
	}
	wantCalls := []string{
		"Target.createTarget",
		"Target.attachToTarget",
		"session-1:Page.enable",
		"session-1:Runtime.enable",
	}
	if !reflect.DeepEqual(cdp.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", cdp.calls, wantCalls)
	}
}

func TestAttachPageUsesExistingTargetAndEnablesDomains(t *testing.T) {
	cdp := &fakeCDP{}

	page, err := browser.AttachPage(context.Background(), cdp, "target-existing")
	if err != nil {
		t.Fatalf("AttachPage returned error: %v", err)
	}

	if page.TargetID != "target-existing" {
		t.Fatalf("TargetID = %q, want target-existing", page.TargetID)
	}
	if page.SessionID != "session-1" {
		t.Fatalf("SessionID = %q, want session-1", page.SessionID)
	}
	wantCalls := []string{
		"Target.attachToTarget",
		"session-1:Page.enable",
		"session-1:Runtime.enable",
	}
	if !reflect.DeepEqual(cdp.calls, wantCalls) {
		t.Fatalf("calls = %#v, want %#v", cdp.calls, wantCalls)
	}
}

func TestPageEvaluateReturnsStringValue(t *testing.T) {
	cdp := &fakeCDP{}
	page, err := browser.OpenPage(context.Background(), cdp, "https://chatgpt.com/")
	if err != nil {
		t.Fatalf("OpenPage returned error: %v", err)
	}

	value, err := page.EvaluateString(context.Background(), `"ok"`)
	if err != nil {
		t.Fatalf("EvaluateString returned error: %v", err)
	}
	if value != "ok" {
		t.Fatalf("value = %q, want ok", value)
	}
}

func TestPageUploadFilesUsesDOMSetFileInputFiles(t *testing.T) {
	cdp := &fakeCDP{}
	page, err := browser.OpenPage(context.Background(), cdp, "https://chatgpt.com/")
	if err != nil {
		t.Fatalf("OpenPage returned error: %v", err)
	}

	err = page.UploadFiles(context.Background(), []string{"C:\\tmp\\context.zip"})
	if err != nil {
		t.Fatalf("UploadFiles returned error: %v", err)
	}

	if len(cdp.uploadedFiles) != 1 || cdp.uploadedFiles[0] != "C:\\tmp\\context.zip" {
		t.Fatalf("uploadedFiles = %#v, want context.zip", cdp.uploadedFiles)
	}
	if len(cdp.querySelectors) != 1 || cdp.querySelectors[0] != `input[type="file"]` {
		t.Fatalf("querySelectors = %#v, want file input selector", cdp.querySelectors)
	}
}
