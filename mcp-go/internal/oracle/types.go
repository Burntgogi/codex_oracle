package oracle

import (
	"bytes"
	"encoding/json"
	"errors"
)

const (
	PresetChatGPTProHeavy       = "chatgpt-pro-heavy"
	DefaultBrowserModel         = "gpt-6-astra-pro"
	DefaultBrowserModelLabel    = "GPT-6 Astra"
	DefaultBrowserThinkingLabel = "Pro"
	LegacyBrowserModel          = "gpt-5.5-pro"
	DefaultAPIModel             = "gpt-5.5"
	DefaultChatGPTURL           = "https://chatgpt.com/"
)

type ExecutionMode string

type WorkspaceRoot string

func (root *WorkspaceRoot) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return errors.New("workspaceRoot must be a string")
	}
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*root = WorkspaceRoot(value)
	return nil
}

const (
	ExecutionModeDryRun         ExecutionMode = "dry-run"
	ExecutionModeChromeAssisted ExecutionMode = "chrome-assisted"
	ExecutionModeLegacyCDP      ExecutionMode = "legacy-cdp"
)

type ConsultInput struct {
	Preset               string        `json:"preset,omitempty"`
	Prompt               string        `json:"prompt,omitempty"`
	Files                []string      `json:"files,omitempty"`
	WorkspaceRoot        WorkspaceRoot `json:"workspaceRoot,omitempty"`
	Model                string        `json:"model,omitempty"`
	Models               []string      `json:"models,omitempty"`
	Engine               string        `json:"engine,omitempty"`
	BrowserModelLabel    string        `json:"browserModelLabel,omitempty"`
	BrowserAttachments   string        `json:"browserAttachments,omitempty"`
	BrowserBundleFiles   bool          `json:"browserBundleFiles,omitempty"`
	BrowserBundleFormat  string        `json:"browserBundleFormat,omitempty"`
	BrowserThinkingTime  string        `json:"browserThinkingTime,omitempty"`
	BrowserModelStrategy string        `json:"browserModelStrategy,omitempty"`
	BrowserResearchMode  string        `json:"browserResearchMode,omitempty"`
	BrowserArchive       string        `json:"browserArchive,omitempty"`
	BrowserFollowUps     []string      `json:"browserFollowUps,omitempty"`
	BrowserKeepBrowser   bool          `json:"browserKeepBrowser,omitempty"`
	GenerateImage        string        `json:"generateImage,omitempty"`
	OutputPath           string        `json:"outputPath,omitempty"`
	DryRun               bool          `json:"dryRun,omitempty"`
	Search               *bool         `json:"search,omitempty"`
	Slug                 string        `json:"slug,omitempty"`
}

type BrowserResolved struct {
	DesiredModel    string `json:"desiredModel,omitempty"`
	ThinkingTime    string `json:"thinkingTime,omitempty"`
	ThinkingLabel   string `json:"thinkingLabel,omitempty"`
	ModelStrategy   string `json:"modelStrategy,omitempty"`
	ResearchMode    string `json:"researchMode,omitempty"`
	Attachments     string `json:"attachments,omitempty"`
	BundleFiles     bool   `json:"bundleFiles,omitempty"`
	BundleFormat    string `json:"bundleFormat,omitempty"`
	KeepBrowser     bool   `json:"keepBrowser,omitempty"`
	ManualLogin     bool   `json:"manualLogin"`
	ProfileDir      string `json:"profileDir,omitempty"`
	ChatGPTURL      string `json:"chatgptUrl,omitempty"`
	ImageOutputPath string `json:"imageOutputPath,omitempty"`
}

type DryRunResolved struct {
	ResolvedEngine string           `json:"resolvedEngine"`
	Model          string           `json:"model"`
	Models         []string         `json:"models,omitempty"`
	Files          []string         `json:"files"`
	FollowUpCount  int              `json:"followUpCount"`
	Browser        *BrowserResolved `json:"browser,omitempty"`
	Guidance       []string         `json:"guidance"`
}

type ChromeCaptureHints struct {
	PreferMarkdownCopy       bool `json:"preferMarkdownCopy"`
	RequireFreshConversation bool `json:"requireFreshConversation"`
}

type ChromePrivacyManifest struct {
	TargetHost                                string   `json:"targetHost"`
	AllowBrowserHistory                       bool     `json:"allowBrowserHistory"`
	AllowUnrelatedTabs                        bool     `json:"allowUnrelatedTabs"`
	RequiresUserApprovalForExternalSubmission bool     `json:"requiresUserApprovalForExternalSubmission"`
	ExternalSubmissionAuthorizedByInvocation  bool     `json:"externalSubmissionAuthorizedByInvocation"`
	SubmittedFiles                            []string `json:"submittedFiles,omitempty"`
}

type ChromeOrchestrationContract struct {
	Mode                             string   `json:"mode"`
	RequiredPlugin                   string   `json:"requiredPlugin"`
	RequiresSignedInUserSession      bool     `json:"requiresSignedInUserSession"`
	OwnsBrowserAutomation            bool     `json:"ownsBrowserAutomation"`
	AutomationOwner                  string   `json:"automationOwner,omitempty"`
	AllowAutomaticSubmission         bool     `json:"allowAutomaticSubmission"`
	RequiresSeparateSendConfirmation bool     `json:"requiresSeparateSendConfirmation"`
	StopConditions                   []string `json:"stopConditions,omitempty"`
}

type ChromeHandoffOptions struct {
	AttachmentMode string
}

type ChromeSubmissionContract struct {
	Mode                                     string `json:"mode"`
	PromptText                               string `json:"promptText"`
	ContextSource                            string `json:"contextSource"`
	ExternalSubmissionAuthorizedByInvocation bool   `json:"externalSubmissionAuthorizedByInvocation"`
	RequiresSeparateSendConfirmation         bool   `json:"requiresSeparateSendConfirmation"`
	StopOnHumanVerification                  bool   `json:"stopOnHumanVerification"`
}

type ChromeHandoff struct {
	ChatGPTURL    string                      `json:"chatgptUrl"`
	Model         string                      `json:"model"`
	ModelLabel    string                      `json:"modelLabel,omitempty"`
	Thinking      string                      `json:"thinking,omitempty"`
	ThinkingLabel string                      `json:"thinkingLabel,omitempty"`
	HandoffDigest string                      `json:"handoffDigest"`
	HandoffNonce  string                      `json:"handoffNonce"`
	Prompt        string                      `json:"prompt"`
	FollowUps     []string                    `json:"followUps,omitempty"`
	Privacy       ChromePrivacyManifest       `json:"privacy"`
	Capture       ChromeCaptureHints          `json:"capture"`
	Submission    ChromeSubmissionContract    `json:"submission"`
	Orchestration ChromeOrchestrationContract `json:"orchestration"`
}
