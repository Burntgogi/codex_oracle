package browser

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type CDPReattacher struct {
	Timeout      time.Duration
	PollInterval time.Duration
}

func NewCDPReattacher() *CDPReattacher {
	return &CDPReattacher{
		Timeout:      2 * time.Minute,
		PollInterval: 2 * time.Second,
	}
}

func (r *CDPReattacher) Reattach(ctx context.Context, request ReattachRequest) (Result, error) {
	if strings.TrimSpace(request.Endpoint) == "" {
		return Result{}, fmt.Errorf("browser DevTools endpoint is required")
	}
	timeout := request.Timeout
	if timeout <= 0 {
		timeout = r.Timeout
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cdp, err := DialCDP(runCtx, request.Endpoint)
	if err != nil {
		return Result{}, err
	}
	defer cdp.Close()

	var page *Page
	if strings.TrimSpace(request.TargetID) != "" {
		page, err = AttachPage(runCtx, cdp, request.TargetID)
	} else if strings.TrimSpace(request.ConversationURL) != "" {
		page, err = OpenPage(runCtx, cdp, request.ConversationURL)
	} else {
		err = fmt.Errorf("target id or conversation URL is required")
	}
	if err != nil {
		return Result{}, err
	}

	client := NewChatGPTClient(page)
	auth, err := client.ProbeAuth(runCtx)
	if err != nil {
		return Result{}, err
	}
	if !auth.Authenticated && auth.LoginVisible {
		return Result{}, fmt.Errorf("%w: run login_setup and sign into ChatGPT at %s", ErrLoginRequired, request.ProfileDir)
	}
	answer, err := WaitForStableAnswer(runCtx, client, WaitOptions{
		PollInterval:  r.pollInterval(),
		StableSamples: 2,
	})
	if err != nil {
		return Result{}, err
	}

	conversationURL := strings.TrimSpace(request.ConversationURL)
	if currentURL, err := page.CurrentURL(runCtx); err == nil && strings.TrimSpace(currentURL) != "" {
		conversationURL = strings.TrimSpace(currentURL)
	}
	return Result{
		Answer:     answer,
		Transcript: answer,
		Runtime: runtimeMetadata(runtimeMetadataInput{
			Endpoint:        request.Endpoint,
			TargetID:        page.TargetID,
			ConversationURL: conversationURL,
			ProfileDir:      request.ProfileDir,
			ChatGPTURL:      request.ChatGPTURL,
			Reattached:      true,
		}),
	}, nil
}

func (r *CDPReattacher) pollInterval() time.Duration {
	if r.PollInterval > 0 {
		return r.PollInterval
	}
	return 2 * time.Second
}
