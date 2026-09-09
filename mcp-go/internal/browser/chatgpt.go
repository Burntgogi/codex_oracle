package browser

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"codex_oracle/internal/oracle"
)

type Evaluator interface {
	EvaluateString(ctx context.Context, expression string) (string, error)
}

type FileUploader interface {
	UploadFiles(ctx context.Context, paths []string) error
}

type ChatGPTClient struct {
	page Evaluator
}

type AuthStatus struct {
	Authenticated   bool   `json:"authenticated"`
	ComposerVisible bool   `json:"composerVisible"`
	LoginVisible    bool   `json:"loginVisible"`
	Reason          string `json:"reason,omitempty"`
}

type ModelSelection struct {
	Model        string `json:"model,omitempty"`
	DesiredLabel string `json:"desiredLabel,omitempty"`
	Strategy     string `json:"strategy,omitempty"`
	ThinkingTime string `json:"thinkingTime,omitempty"`
}

type ModelEvidence struct {
	Status        string `json:"status"`
	ObservedLabel string `json:"observedLabel,omitempty"`
	ThinkingTime  string `json:"thinkingTime,omitempty"`
	ProConfirmed  bool   `json:"proConfirmed"`
	Confirmed     bool   `json:"confirmed"`
	Reason        string `json:"reason,omitempty"`
}

func NewChatGPTClient(page Evaluator) *ChatGPTClient {
	return &ChatGPTClient{page: page}
}

func (c *ChatGPTClient) ProbeAuth(ctx context.Context) (AuthStatus, error) {
	raw, err := c.page.EvaluateString(ctx, authProbeExpression())
	if err != nil {
		return AuthStatus{}, err
	}
	var status AuthStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return AuthStatus{}, err
	}
	return status, nil
}

func (c *ChatGPTClient) SubmitPrompt(ctx context.Context, prompt string) error {
	raw, err := c.page.EvaluateString(ctx, submitPromptExpression(prompt))
	if err != nil {
		return err
	}
	var result struct {
		Submitted bool   `json:"submitted"`
		Reason    string `json:"reason,omitempty"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return err
	}
	if !result.Submitted {
		if result.Reason == "" {
			result.Reason = "ChatGPT submit button was not activated"
		}
		return errors.New(result.Reason)
	}
	return nil
}

func (c *ChatGPTClient) UploadFiles(ctx context.Context, paths []string) error {
	uploader, ok := c.page.(FileUploader)
	if !ok {
		return errors.New("ChatGPT page does not support file upload")
	}
	raw, err := c.page.EvaluateString(ctx, prepareFileUploadExpression())
	if err != nil {
		return err
	}
	var result struct {
		Ready  bool   `json:"ready"`
		Reason string `json:"reason,omitempty"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return err
	}
	if !result.Ready {
		if result.Reason == "" {
			result.Reason = "ChatGPT file input was not available"
		}
		return errors.New(result.Reason)
	}
	if err := uploader.UploadFiles(ctx, paths); err != nil {
		return err
	}
	raw, err = c.page.EvaluateString(ctx, waitForFileUploadExpression(len(paths)))
	if err != nil {
		return err
	}
	var accepted struct {
		Accepted  bool   `json:"accepted"`
		FileCount int    `json:"fileCount,omitempty"`
		Reason    string `json:"reason,omitempty"`
	}
	if err := json.Unmarshal([]byte(raw), &accepted); err != nil {
		return err
	}
	if !accepted.Accepted {
		if accepted.Reason == "" {
			accepted.Reason = "ChatGPT did not accept uploaded file"
		}
		return errors.New(accepted.Reason)
	}
	return nil
}

func (c *ChatGPTClient) ConfigureModel(ctx context.Context, selection ModelSelection) (ModelEvidence, error) {
	normalized := normalizeModelSelection(selection)
	raw, err := c.page.EvaluateString(ctx, configureModelExpression(normalized))
	if err != nil {
		return ModelEvidence{}, err
	}
	var evidence ModelEvidence
	if err := json.Unmarshal([]byte(raw), &evidence); err != nil {
		return ModelEvidence{}, err
	}
	return sanitizeModelEvidence(evidence), nil
}

func prepareFileUploadExpression() string {
	return `(async function oraclePrepareFileUpload() {
  let input = document.querySelector('input[type="file"]');
  if (input) return JSON.stringify({ ready: true, reason: 'file-input-present' });
  const controls = Array.from(document.querySelectorAll('button,[role="button"],label'));
  const attach = controls.find(control => {
    const label = [control.getAttribute('aria-label'), control.getAttribute('data-testid'), control.textContent].join(' ');
    return /attach|upload|file|paperclip|\+/i.test(label || '');
  });
  if (attach) attach.click();
  for (let i = 0; i < 20; i++) {
    await new Promise(resolve => setTimeout(resolve, 100));
    input = document.querySelector('input[type="file"]');
    if (input) return JSON.stringify({ ready: true, reason: 'file-input-created' });
  }
  return JSON.stringify({ ready: false, reason: 'file input not found' });
})()`
}

func waitForFileUploadExpression(expectedCount int) string {
	return `(async function oracleWaitForFileUpload() {
  const expected = ` + strconv.Itoa(expectedCount) + `;
  for (let i = 0; i < 50; i++) {
    const input = document.querySelector('input[type="file"]');
    const inputCount = input && input.files ? input.files.length : 0;
    const chips = Array.from(document.querySelectorAll('[data-testid*="attachment"], [data-testid*="file"], [aria-label*="file"], [aria-label*="attachment"]'))
      .filter(node => (node.innerText || node.textContent || node.getAttribute('aria-label') || '').trim());
    const fileCount = Math.max(inputCount, chips.length);
    if (fileCount >= expected) {
      return JSON.stringify({ accepted: true, fileCount, reason: 'upload-visible' });
    }
    await new Promise(resolve => setTimeout(resolve, 100));
  }
  return JSON.stringify({ accepted: false, fileCount: 0, reason: 'uploaded file not visible before timeout' });
})()`
}

func VerifyModelEvidence(selection ModelSelection, evidence ModelEvidence) error {
	normalized := normalizeModelSelection(selection)
	if normalized.Strategy == "" {
		normalized.Strategy = "select"
	}
	if normalized.Strategy != "ignore" && normalized.DesiredLabel != "" {
		if evidence.ObservedLabel == "" || !visibleLabelContains(evidence.ObservedLabel, normalized.DesiredLabel) {
			return errors.New("requested ChatGPT model was not confirmed before submit")
		}
	}
	if normalized.Model == oracle.DefaultBrowserModel {
		if !proModeConfirmed(evidence) {
			return errors.New("GPT-6 Astra with Pro was not confirmed before submit")
		}
	}
	if normalized.Strategy != "ignore" && !evidence.Confirmed {
		return errors.New("ChatGPT model selection was not confirmed before submit")
	}
	return nil
}

func (c *ChatGPTClient) CaptureLatestAssistantText(ctx context.Context) (string, error) {
	raw, err := c.page.EvaluateString(ctx, captureAssistantExpression())
	if err != nil {
		return "", err
	}
	var result struct {
		Text   string `json:"text"`
		Reason string `json:"reason,omitempty"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return "", err
	}
	if strings.TrimSpace(result.Text) == "" {
		if result.Reason == "" {
			result.Reason = "no assistant text found"
		}
		return "", errors.New(result.Reason)
	}
	return result.Text, nil
}

func (c *ChatGPTClient) ProbeCompletion(ctx context.Context) (CompletionStatus, error) {
	raw, err := c.page.EvaluateString(ctx, completionStatusExpression())
	if err != nil {
		return CompletionStatus{}, err
	}
	var status CompletionStatus
	if err := json.Unmarshal([]byte(raw), &status); err != nil {
		return CompletionStatus{}, err
	}
	status.Reason = boundedRedactedText(status.Reason, 160)
	return status, nil
}

func normalizeModelSelection(selection ModelSelection) ModelSelection {
	selection.Model = strings.ToLower(strings.TrimSpace(selection.Model))
	selection.DesiredLabel = strings.TrimSpace(selection.DesiredLabel)
	selection.Strategy = strings.TrimSpace(selection.Strategy)
	selection.ThinkingTime = strings.ToLower(strings.TrimSpace(selection.ThinkingTime))
	if selection.Strategy == "" {
		selection.Strategy = "select"
	}
	selection.Model, _ = oracle.NormalizeBrowserModel(selection.Model)
	if selection.Model == oracle.DefaultBrowserModel {
		selection.DesiredLabel = oracle.DefaultBrowserModelLabel
	} else if selection.DesiredLabel == "" {
		selection.DesiredLabel = selection.Model
	}
	return selection
}

func normalizeVisibleLabel(value string) string {
	replacer := strings.NewReplacer(
		"\u2010", "-", "\u2011", "-", "\u2012", "-", "\u2013", "-",
		"\u2014", "-", "\u2212", "-", "\u00a0", " ",
	)
	return strings.ToLower(strings.Join(strings.Fields(replacer.Replace(value)), " "))
}

func visibleLabelContains(observed string, desired string) bool {
	normalizedObserved := normalizeVisibleLabel(observed)
	normalizedDesired := normalizeVisibleLabel(desired)
	if normalizedObserved == normalizedDesired {
		return true
	}
	if !strings.HasPrefix(normalizedObserved, normalizedDesired+" ") {
		return false
	}
	suffix := strings.TrimSpace(strings.TrimPrefix(normalizedObserved, normalizedDesired))
	return suffix == "pro" || suffix == "pro 확장" || suffix == "프로 확장"
}

func proModeConfirmed(evidence ModelEvidence) bool {
	if !evidence.ProConfirmed {
		return false
	}
	thinkingTime := strings.TrimSpace(evidence.ThinkingTime)
	return thinkingTime == "" || strings.EqualFold(thinkingTime, "extended")
}

func completionStatusExpression() string {
	return `(function oracleCompletionStatus() {
  const controls = Array.from(document.querySelectorAll('button,[role="button"],[data-testid*="stop"],[aria-label*="Stop"],[data-testid="send-button"],[aria-label*="Send"],[aria-busy="true"]'));
  const labelOf = element => [element.getAttribute('aria-label'), element.getAttribute('data-testid'), element.getAttribute('title'), element.textContent].join(' ').replace(/\s+/g, ' ').trim();
  const stop = controls.find(control => /stop|interrupt|cancel|generating/i.test(labelOf(control) || ''));
  const busy = controls.find(control => control.getAttribute('aria-busy') === 'true');
  const send = controls.find(control => /send|submit|composer-submit|send-button/i.test(labelOf(control) || ''));
  const composer = document.querySelector('#prompt-textarea, [data-testid="composer-text-input"], [contenteditable="true"][data-testid*="composer"], div.ProseMirror, textarea');
  const inProgress = Boolean(stop || busy);
  const ready = Boolean(!inProgress && composer && send && !send.disabled);
  let reason = 'unknown';
  if (stop) reason = 'stop-button-visible';
  else if (busy) reason = 'aria-busy';
  else if (ready) reason = 'send-button-ready';
  else if (composer) reason = 'composer-visible';
  return JSON.stringify({ inProgress, ready, reason });
})()`
}

func authProbeExpression() string {
	return `(async function oracleAuthProbe() {
  const composer = document.querySelector('textarea, [contenteditable="true"], div.ProseMirror');
  const login = document.querySelector('a[href*="/auth/login"], button[data-testid*="login"], button');
  let sessionUser = false;
  try {
    const response = await fetch('/api/auth/session', { credentials: 'include' });
    if (response.ok) {
      const data = await response.json();
      sessionUser = Boolean(data && data.user);
    }
  } catch (_) {}
  return JSON.stringify({
    authenticated: Boolean(sessionUser || composer),
    composerVisible: Boolean(composer),
    loginVisible: Boolean(login && /log in|sign in/i.test(login.textContent || '')),
    reason: sessionUser ? 'session-user' : (composer ? 'composer-visible' : 'no-session')
  });
})()`
}

func submitPromptExpression(prompt string) string {
	encodedPrompt := strconv.Quote(prompt)
	return `(async function oracleSubmitPrompt() {
  const prompt = ` + encodedPrompt + `;
  const candidates = [
    document.querySelector('#prompt-textarea'),
    document.querySelector('[data-testid="composer-text-input"]'),
    document.querySelector('[contenteditable="true"][data-testid*="composer"]'),
    document.querySelector('div.ProseMirror'),
    document.querySelector('textarea'),
    document.querySelector('[contenteditable="true"]')
  ].filter(Boolean);
  const input = candidates[0];
  if (!input) return JSON.stringify({ submitted: false, reason: 'composer not found' });
  input.focus();
  if ('value' in input) {
    input.value = prompt;
    input.dispatchEvent(new Event('input', { bubbles: true }));
  } else {
    input.textContent = prompt;
    input.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertText', data: prompt }));
  }
  await new Promise(resolve => setTimeout(resolve, 50));
  const buttons = Array.from(document.querySelectorAll('button,[role="button"],[data-testid="send-button"],[aria-label*="Send"]'));
  const send = buttons.find(button => {
    const label = [button.getAttribute('aria-label'), button.getAttribute('data-testid'), button.textContent].join(' ');
    return /send|submit|composer-submit|send-button/i.test(label || '');
  });
  if (!send || send.disabled) return JSON.stringify({ submitted: false, reason: 'send button unavailable' });
  send.click();
  return JSON.stringify({ submitted: true });
})()`
}

func configureModelExpression(selection ModelSelection) string {
	payload, _ := json.Marshal(selection)
	return `(async function oracleConfigureModel() {
  const selection = ` + string(payload) + `;
  const controlSelector = 'button,[role="button"],[role="menuitem"],[role="option"],[data-testid*="model"],[aria-label*="model"],[data-testid*="thinking"],[aria-label*="thinking"]';
  const requeryControls = () => Array.from(document.querySelectorAll(controlSelector));
  const textOf = element => (element.innerText || element.textContent || '').replace(/\s+/g, ' ').trim();
  const normalizeLabel = value => (value || '').replace(/[\u2010-\u2014\u2212]/g, '-').replace(/\s+/g, ' ').trim().toLowerCase();
  const desired = normalizeLabel(selection.desiredLabel || '');
  const modelLabelMatches = label => {
    const normalized = normalizeLabel(label);
    if (!desired || normalized === desired) return Boolean(desired && normalized === desired);
    if (!normalized.startsWith(desired + ' ')) return false;
    const suffix = normalized.slice(desired.length).trim();
    return suffix === 'pro' || suffix === 'pro 확장' || suffix === '프로 확장';
  };
  const englishPro = /\bpro\b/i;
  const proLabelMatches = label => {
    const normalized = normalizeLabel(label);
    return (englishPro.test(normalized) && (normalized === 'pro' || normalized === 'pro 확장')) || normalized === '프로 확장';
  };
  const isActive = control => {
    const selected = value => ['true', 'active', 'checked', 'on', 'selected'].includes((value || '').toLowerCase());
    return selected(control.getAttribute('aria-pressed')) || selected(control.getAttribute('aria-selected')) || selected(control.getAttribute('aria-checked')) || selected(control.getAttribute('data-state')) || selected(control.getAttribute('data-selected')) || selected(control.getAttribute('data-active')) || selected(control.getAttribute('data-checked'));
  };
  const inspectModel = () => {
    const matching = requeryControls().filter(control => modelLabelMatches(textOf(control)));
    const active = matching.find(isActive);
    return { control: active || matching[0], confirmed: Boolean(active) };
  };
  const inspectPro = () => {
    const matching = requeryControls().filter(control => proLabelMatches(textOf(control)));
    const active = matching.find(isActive);
    return { control: active || matching[0], confirmed: Boolean(active) };
  };
  const waitFor = async inspect => {
    for (let attempt = 0; attempt < 20; attempt++) {
      const state = inspect();
      if (state.confirmed) return state;
      await new Promise(resolve => setTimeout(resolve, 50));
    }
    return inspect();
  };
  let modelState = inspectModel();
  let observedLabel = modelState.control ? textOf(modelState.control) : '';
  if (selection.strategy === 'ignore') {
    return JSON.stringify({ status: 'ignored', observedLabel, proConfirmed: false, confirmed: false, reason: 'model picker skipped' });
  }
  if (selection.strategy === 'current') {
    const proState = inspectPro();
    const proConfirmed = Boolean(modelState.confirmed && proState.confirmed);
    return JSON.stringify({ status: 'current', observedLabel, thinkingTime: proConfirmed ? 'extended' : '', proConfirmed, confirmed: modelState.confirmed, reason: 'current model inspected' });
  }
  if (modelState.control && desired && !modelState.confirmed) {
    modelState.control.click();
    modelState = await waitFor(inspectModel);
  }
  if (!modelState.confirmed) {
    return JSON.stringify({ status: 'not-found', observedLabel, thinkingTime: '', proConfirmed: false, confirmed: false, reason: 'model control did not become active' });
  }
  const requiresPro = selection.model === 'gpt-6-astra-pro';
  let proState = inspectPro();
  if (requiresPro && proState.control && !proState.confirmed) {
    proState.control.click();
    proState = await waitFor(inspectPro);
  }
  const proConfirmed = Boolean(requiresPro && modelState.confirmed && proState.confirmed);
  observedLabel = textOf(modelState.control) || selection.desiredLabel;
  return JSON.stringify({
    status: 'selected',
    observedLabel,
    thinkingTime: proConfirmed ? 'extended' : '',
    proConfirmed,
    confirmed: true,
    reason: proConfirmed || !requiresPro ? 'matched active bounded controls' : 'Pro control did not become active'
  });
})()`
}

func sanitizeModelEvidence(evidence ModelEvidence) ModelEvidence {
	evidence.Status = boundedRedactedText(evidence.Status, 80)
	evidence.ObservedLabel = boundedRedactedText(evidence.ObservedLabel, 160)
	evidence.ThinkingTime = boundedRedactedText(evidence.ThinkingTime, 40)
	evidence.Reason = boundedRedactedText(evidence.Reason, 160)
	return evidence
}

func boundedRedactedText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	replacers := []struct {
		pattern string
		token   string
	}{
		{`(?i)[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}`, "[redacted-email]"},
		{`(?i)\bsk-[a-z0-9_\-]{6,}\b`, "[redacted-token]"},
		{`(?i)\b(?:session|cookie|token)[a-z0-9_\-]*\s*[:=]\s*[^\s,;]+`, "[redacted-secret]"},
		{`(?i)\bcookie\b`, "[redacted]"},
		{`(?i)\bsession[_-]?token\b`, "[redacted-secret]"},
	}
	for _, replacer := range replacers {
		value = regexp.MustCompile(replacer.pattern).ReplaceAllString(value, replacer.token)
	}
	if limit > 0 && len(value) > limit {
		if limit <= 3 {
			return value[:limit]
		}
		return strings.TrimSpace(value[:limit-3]) + "..."
	}
	return value
}

func captureAssistantExpression() string {
	return `(function oracleCaptureLatestAssistantText() {
  const assistantNodes = Array.from(document.querySelectorAll('[data-message-author-role="assistant"], [data-testid*="assistant"]'));
  const turnNodes = Array.from(document.querySelectorAll('[data-testid*="conversation-turn"]')).filter(node => {
    if (node.querySelector('[data-message-author-role="user"]')) return false;
    if (node.querySelector('[data-message-author-role="assistant"]')) return true;
    const label = [node.getAttribute('aria-label'), node.getAttribute('data-testid')].join(' ');
    return /assistant|chatgpt/i.test(label || '');
  });
  const nodes = assistantNodes.concat(turnNodes);
  const texts = nodes.map(node => (node.innerText || node.textContent || '').trim()).filter(Boolean);
  const text = texts.length ? texts[texts.length - 1] : '';
  return JSON.stringify({ text, reason: text ? 'captured' : 'no assistant text found' });
})()`
}
