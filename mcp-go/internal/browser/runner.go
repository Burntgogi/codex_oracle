package browser

import (
	"context"
	"errors"
	"time"
)

var ErrNotImplemented = errors.New("live ChatGPT browser automation is not implemented in this Go milestone")

type Request struct {
	Prompt               string
	BundleText           string
	FollowUps            []string
	AttachmentPaths      []string
	Model                string
	DesiredModelLabel    string
	BrowserThinkingTime  string
	BrowserModelStrategy string
	ProfileDir           string
	ChatGPTURL           string
	KeepBrowser          bool
}

type Result struct {
	Answer     string
	Transcript string
	Runtime    map[string]any
}

type Runner interface {
	Run(ctx context.Context, request Request) (Result, error)
}

type ReattachRequest struct {
	Endpoint        string
	TargetID        string
	ConversationURL string
	ProfileDir      string
	ChatGPTURL      string
	Timeout         time.Duration
}

type Reattacher interface {
	Reattach(ctx context.Context, request ReattachRequest) (Result, error)
}

type SmokeRequest struct {
	Endpoint   string
	ProfileDir string
	ChatGPTURL string
	Timeout    time.Duration
}

type SmokeCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

type CompletionStatus struct {
	InProgress bool   `json:"inProgress"`
	Ready      bool   `json:"ready"`
	Reason     string `json:"reason,omitempty"`
}

type SmokeResult struct {
	Endpoint   string       `json:"endpoint,omitempty"`
	ProfileDir string       `json:"profileDir,omitempty"`
	ChatGPTURL string       `json:"chatgptUrl,omitempty"`
	Auth       AuthStatus   `json:"auth"`
	Checks     []SmokeCheck `json:"checks"`
}

type SmokeChecker interface {
	Check(ctx context.Context, request SmokeRequest) (SmokeResult, error)
}

type NotImplementedRunner struct{}

func (NotImplementedRunner) Run(context.Context, Request) (Result, error) {
	return Result{}, ErrNotImplemented
}
