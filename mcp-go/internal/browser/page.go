package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type CDPCaller interface {
	Call(ctx context.Context, method string, params any) (json.RawMessage, error)
	CallSession(ctx context.Context, sessionID string, method string, params any) (json.RawMessage, error)
}

type Page struct {
	cdp       CDPCaller
	TargetID  string
	SessionID string
}

func OpenPage(ctx context.Context, cdp CDPCaller, pageURL string) (*Page, error) {
	if strings.TrimSpace(pageURL) == "" {
		return nil, fmt.Errorf("page URL is required")
	}
	createResult, err := cdp.Call(ctx, "Target.createTarget", map[string]any{"url": pageURL})
	if err != nil {
		return nil, err
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err := json.Unmarshal(createResult, &created); err != nil {
		return nil, err
	}
	if created.TargetID == "" {
		return nil, fmt.Errorf("Target.createTarget did not return targetId")
	}
	return AttachPage(ctx, cdp, created.TargetID)
}

func AttachPage(ctx context.Context, cdp CDPCaller, targetID string) (*Page, error) {
	targetID = strings.TrimSpace(targetID)
	if targetID == "" {
		return nil, fmt.Errorf("target id is required")
	}
	attachResult, err := cdp.Call(ctx, "Target.attachToTarget", map[string]any{
		"targetId": targetID,
		"flatten":  true,
	})
	if err != nil {
		return nil, err
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(attachResult, &attached); err != nil {
		return nil, err
	}
	if attached.SessionID == "" {
		return nil, fmt.Errorf("Target.attachToTarget did not return sessionId")
	}
	page := &Page{cdp: cdp, TargetID: targetID, SessionID: attached.SessionID}
	if err := page.enable(ctx); err != nil {
		return nil, err
	}
	return page, nil
}

func (p *Page) enable(ctx context.Context) error {
	if _, err := p.cdp.CallSession(ctx, p.SessionID, "Page.enable", map[string]any{}); err != nil {
		return err
	}
	if _, err := p.cdp.CallSession(ctx, p.SessionID, "Runtime.enable", map[string]any{}); err != nil {
		return err
	}
	return nil
}

func (p *Page) EvaluateString(ctx context.Context, expression string) (string, error) {
	result, err := p.cdp.CallSession(ctx, p.SessionID, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
		"awaitPromise":  true,
	})
	if err != nil {
		return "", err
	}
	var evaluated struct {
		Result struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(result, &evaluated); err != nil {
		return "", err
	}
	if len(evaluated.ExceptionDetails) > 0 {
		return "", fmt.Errorf("Runtime.evaluate exception: %s", string(evaluated.ExceptionDetails))
	}
	return evaluated.Result.Value, nil
}

func (p *Page) UploadFiles(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	document, err := p.cdp.CallSession(ctx, p.SessionID, "DOM.getDocument", map[string]any{})
	if err != nil {
		return err
	}
	var doc struct {
		Root struct {
			NodeID int `json:"nodeId"`
		} `json:"root"`
	}
	if err := json.Unmarshal(document, &doc); err != nil {
		return err
	}
	if doc.Root.NodeID == 0 {
		return fmt.Errorf("DOM.getDocument did not return root node")
	}
	query, err := p.cdp.CallSession(ctx, p.SessionID, "DOM.querySelector", map[string]any{
		"nodeId":   doc.Root.NodeID,
		"selector": `input[type="file"]`,
	})
	if err != nil {
		return err
	}
	var selected struct {
		NodeID int `json:"nodeId"`
	}
	if err := json.Unmarshal(query, &selected); err != nil {
		return err
	}
	if selected.NodeID == 0 {
		return fmt.Errorf("file input not found")
	}
	_, err = p.cdp.CallSession(ctx, p.SessionID, "DOM.setFileInputFiles", map[string]any{
		"nodeId": selected.NodeID,
		"files":  paths,
	})
	return err
}

func (p *Page) CurrentURL(ctx context.Context) (string, error) {
	return p.EvaluateString(ctx, "window.location.href")
}
