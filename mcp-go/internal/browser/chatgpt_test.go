package browser_test

import (
	"context"
	"strings"
	"testing"

	"codex_oracle/internal/browser"
)

type fakeEvaluator struct {
	expressions []string
	next        []string
}

func (f *fakeEvaluator) EvaluateString(_ context.Context, expression string) (string, error) {
	f.expressions = append(f.expressions, expression)
	if len(f.next) == 0 {
		return "", nil
	}
	value := f.next[0]
	f.next = f.next[1:]
	return value, nil
}

type fakeUploadPage struct {
	fakeEvaluator
	uploaded []string
}

func (f *fakeUploadPage) UploadFiles(_ context.Context, paths []string) error {
	f.uploaded = append([]string{}, paths...)
	return nil
}

func TestProbeAuthAcceptsAuthenticatedSession(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{"authenticated":true,"composerVisible":true}`}}
	client := browser.NewChatGPTClient(evaluator)

	status, err := client.ProbeAuth(context.Background())
	if err != nil {
		t.Fatalf("ProbeAuth returned error: %v", err)
	}
	if !status.Authenticated {
		t.Fatal("Authenticated = false, want true")
	}
	if len(evaluator.expressions) != 1 || !strings.Contains(evaluator.expressions[0], "oracleAuthProbe") {
		t.Fatalf("probe expression = %#v, want oracleAuthProbe marker", evaluator.expressions)
	}
}

func TestSubmitPromptEscapesPromptAndRequiresSubmittedResult(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{"submitted":true}`}}
	client := browser.NewChatGPTClient(evaluator)

	err := client.SubmitPrompt(context.Background(), "hello `world`\n</script>")
	if err != nil {
		t.Fatalf("SubmitPrompt returned error: %v", err)
	}
	expression := evaluator.expressions[0]
	if !strings.Contains(expression, `hello `) || strings.Contains(expression, "\n</script>") {
		t.Fatalf("submit expression did not JSON-escape prompt safely:\n%s", expression)
	}
	if !strings.Contains(expression, "oracleSubmitPrompt") {
		t.Fatalf("submit expression missing marker:\n%s", expression)
	}
}

func TestSubmitPromptExpressionUsesStableComposerSelectors(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{"submitted":true}`}}
	client := browser.NewChatGPTClient(evaluator)

	err := client.SubmitPrompt(context.Background(), "hello")
	if err != nil {
		t.Fatalf("SubmitPrompt returned error: %v", err)
	}
	expression := evaluator.expressions[0]
	for _, want := range []string{`#prompt-textarea`, `[data-testid="composer-text-input"]`, `[data-testid="send-button"]`, `[aria-label*="Send"]`} {
		if !strings.Contains(expression, want) {
			t.Fatalf("submit expression missing selector %s:\n%s", want, expression)
		}
	}
}

func TestUploadFilesPreparesInputSetsFilesAndWaitsForAcceptance(t *testing.T) {
	page := &fakeUploadPage{
		fakeEvaluator: fakeEvaluator{
			next: []string{
				`{"ready":true}`,
				`{"accepted":true,"fileCount":1}`,
			},
		},
	}
	client := browser.NewChatGPTClient(page)

	err := client.UploadFiles(context.Background(), []string{"C:\\tmp\\context.zip"})
	if err != nil {
		t.Fatalf("UploadFiles returned error: %v", err)
	}
	if len(page.uploaded) != 1 || page.uploaded[0] != "C:\\tmp\\context.zip" {
		t.Fatalf("uploaded = %#v, want context.zip", page.uploaded)
	}
	if len(page.expressions) != 2 {
		t.Fatalf("expressions = %#v, want prepare and wait expressions", page.expressions)
	}
	if !strings.Contains(page.expressions[0], "oraclePrepareFileUpload") {
		t.Fatalf("first expression = %q, want prepare marker", page.expressions[0])
	}
	if !strings.Contains(page.expressions[1], "oracleWaitForFileUpload") {
		t.Fatalf("second expression = %q, want wait marker", page.expressions[1])
	}
}

func TestCaptureLatestAssistantTextReturnsText(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{"text":"final answer"}`}}
	client := browser.NewChatGPTClient(evaluator)

	text, err := client.CaptureLatestAssistantText(context.Background())
	if err != nil {
		t.Fatalf("CaptureLatestAssistantText returned error: %v", err)
	}
	if text != "final answer" {
		t.Fatalf("text = %q, want final answer", text)
	}
}

func TestCaptureAssistantExpressionFiltersUserTurns(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{"text":"final answer"}`}}
	client := browser.NewChatGPTClient(evaluator)

	_, err := client.CaptureLatestAssistantText(context.Background())
	if err != nil {
		t.Fatalf("CaptureLatestAssistantText returned error: %v", err)
	}
	expression := evaluator.expressions[0]
	for _, want := range []string{`[data-message-author-role="assistant"]`, `[data-message-author-role="user"]`, `conversation-turn`} {
		if !strings.Contains(expression, want) {
			t.Fatalf("capture expression missing %s:\n%s", want, expression)
		}
	}
	if strings.Contains(expression, "'article'") {
		t.Fatalf("capture expression still uses generic article fallback:\n%s", expression)
	}
}

func TestProbeCompletionReturnsStatus(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{"inProgress":false,"ready":true,"reason":"send-button-ready"}`}}
	client := browser.NewChatGPTClient(evaluator)

	status, err := client.ProbeCompletion(context.Background())
	if err != nil {
		t.Fatalf("ProbeCompletion returned error: %v", err)
	}
	if status.InProgress {
		t.Fatal("InProgress = true, want false")
	}
	if !status.Ready {
		t.Fatal("Ready = false, want true")
	}
	if status.Reason != "send-button-ready" {
		t.Fatalf("Reason = %q, want send-button-ready", status.Reason)
	}
	expression := evaluator.expressions[0]
	for _, want := range []string{`oracleCompletionStatus`, `[data-testid*="stop"]`, `[aria-label*="Stop"]`, `[data-testid="send-button"]`} {
		if !strings.Contains(expression, want) {
			t.Fatalf("completion expression missing %s:\n%s", want, expression)
		}
	}
	if strings.Contains(expression, "document.body.innerText") || strings.Contains(expression, "document.body.textContent") {
		t.Fatalf("completion expression reads full body text, want bounded controls only:\n%s", expression)
	}
}

func TestConfigureModelReturnsEvidence(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{"status":"selected","observedLabel":"GPT-6 Astra Pro","thinkingTime":"extended","proConfirmed":true,"confirmed":true}`}}
	client := browser.NewChatGPTClient(evaluator)

	evidence, err := client.ConfigureModel(context.Background(), browser.ModelSelection{
		Model:        "gpt-6-astra-pro",
		DesiredLabel: "GPT-6 Astra",
		Strategy:     "select",
		ThinkingTime: "extended",
	})
	if err != nil {
		t.Fatalf("ConfigureModel returned error: %v", err)
	}
	if evidence.ObservedLabel != "GPT-6 Astra Pro" {
		t.Fatalf("ObservedLabel = %q, want GPT-6 Astra Pro", evidence.ObservedLabel)
	}
	if evidence.ThinkingTime != "extended" {
		t.Fatalf("ThinkingTime = %q, want extended", evidence.ThinkingTime)
	}
	if !evidence.ProConfirmed {
		t.Fatal("ProConfirmed = false, want true")
	}
	if !strings.Contains(evaluator.expressions[0], "oracleConfigureModel") {
		t.Fatalf("model expression = %q, want oracleConfigureModel marker", evaluator.expressions[0])
	}
}

func TestConfigureModelExpressionUsesBoundedControlSelectors(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{"status":"selected","observedLabel":"GPT-6 Astra Pro","thinkingTime":"extended","proConfirmed":true,"confirmed":true}`}}
	client := browser.NewChatGPTClient(evaluator)

	_, err := client.ConfigureModel(context.Background(), browser.ModelSelection{
		Model:        "gpt-6-astra-pro",
		DesiredLabel: "GPT-6 Astra",
	})
	if err != nil {
		t.Fatalf("ConfigureModel returned error: %v", err)
	}
	expression := evaluator.expressions[0]
	for _, want := range []string{`async function oracleConfigureModel`, `[data-testid*="model"]`, `[aria-label*="model"]`, `[role="menuitem"]`, `[role="option"]`, `gpt-6-astra-pro`, `\bpro\b`, `pro 확장`, `프로 확장`, `proConfirmed`, `aria-pressed`, `aria-selected`, `aria-checked`, `data-state`, `requeryControls`, `for (let attempt = 0; attempt < 20; attempt++)`, `await new Promise`, `modelState.control.click()`, `modelState = await waitFor(inspectModel)`, `proState.control.click()`, `proState = await waitFor(inspectPro)`} {
		if !strings.Contains(expression, want) {
			t.Fatalf("model expression missing selector %s:\n%s", want, expression)
		}
	}
	for _, forbidden := range []string{`modelConfirmed = true;`, `proConfirmed = true;`} {
		if strings.Contains(expression, forbidden) {
			t.Fatalf("model expression treats click as confirmation (%s):\n%s", forbidden, expression)
		}
	}
	modelWaitIndex := strings.Index(expression, `modelState = await waitFor(inspectModel);`)
	proQueryIndex := strings.Index(expression, `let proState = inspectPro();`)
	if modelWaitIndex < 0 || proQueryIndex < 0 || proQueryIndex <= modelWaitIndex {
		t.Fatalf("model expression reuses Pro state before model selection completes:\n%s", expression)
	}
	if strings.Contains(expression, "document.body.innerText") || strings.Contains(expression, "document.body.textContent") {
		t.Fatalf("model expression reads full body text, want bounded controls only:\n%s", expression)
	}
}

func TestConfigureModelRedactsSensitiveEvidence(t *testing.T) {
	evaluator := &fakeEvaluator{next: []string{`{
  "status":"selected",
  "observedLabel":"GPT-6 Astra Pro user@example.com sk-proj-secret session_token=abc123",
  "thinkingTime":"extended",
  "proConfirmed":true,
  "confirmed":true,
  "reason":"matched button for user@example.com with cookie session_token=abc123 and sk-proj-secret plus a long diagnostic payload that should be bounded before it can be returned to callers"
}`}}
	client := browser.NewChatGPTClient(evaluator)

	evidence, err := client.ConfigureModel(context.Background(), browser.ModelSelection{
		Model:        "gpt-6-astra-pro",
		DesiredLabel: "GPT-6 Astra",
		Strategy:     "select",
		ThinkingTime: "extended",
	})
	if err != nil {
		t.Fatalf("ConfigureModel returned error: %v", err)
	}
	combined := strings.Join([]string{evidence.ObservedLabel, evidence.Reason}, " ")
	for _, forbidden := range []string{"user@example.com", "sk-proj-secret", "session_token", "cookie"} {
		if strings.Contains(strings.ToLower(combined), strings.ToLower(forbidden)) {
			t.Fatalf("evidence leaked %q: %#v", forbidden, evidence)
		}
	}
	if !strings.Contains(evidence.ObservedLabel, "GPT-6 Astra") {
		t.Fatalf("ObservedLabel = %q, want model label preserved", evidence.ObservedLabel)
	}
	if len(evidence.Reason) > 160 {
		t.Fatalf("Reason length = %d, want bounded reason", len(evidence.Reason))
	}
}

func TestVerifyModelEvidenceFailsWhenProExtendedUnconfirmed(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model:        "gpt-6-astra-pro",
		DesiredLabel: "GPT-6 Astra",
		ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status:        "current",
		ObservedLabel: "GPT-6 Astra",
		ThinkingTime:  "standard",
		Confirmed:     true,
	})
	if err == nil {
		t.Fatal("VerifyModelEvidence returned nil error, want extended verification error")
	}
}

func TestVerifyModelEvidenceAcceptsGPT6AstraWithProExtensionLabel(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model:        "gpt-6-astra-pro",
		DesiredLabel: "GPT-6 Astra",
		ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status:        "current",
		ObservedLabel: "GPT-6 Astra Pro 확장",
		ThinkingTime:  "",
		ProConfirmed:  true,
		Confirmed:     true,
	})
	if err != nil {
		t.Fatalf("VerifyModelEvidence returned error: %v", err)
	}
}

func TestVerifyModelEvidenceAcceptsUnicodeHyphenAndProLabel(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model: "gpt-6-astra-pro", DesiredLabel: "GPT-6 Astra", ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status: "current", ObservedLabel: "GPT‑6   Astra  Pro", ProConfirmed: true, Confirmed: true,
	})
	if err != nil {
		t.Fatalf("VerifyModelEvidence returned error: %v", err)
	}
}

func TestVerifyModelEvidenceRejectsGPT55ForGPT6Astra(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model: "gpt-6-astra-pro", DesiredLabel: "GPT-6 Astra", ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status: "current", ObservedLabel: "GPT-5.5 Pro", ProConfirmed: true, Confirmed: true,
	})
	if err == nil {
		t.Fatal("VerifyModelEvidence returned nil, want model mismatch")
	}
}

func TestVerifyModelEvidenceCanonicalizesExplicitGPT55BrowserSelection(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model: " GPT-5.5 ", DesiredLabel: "GPT-5.5", ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status: "current", ObservedLabel: "GPT-6 Astra Pro", ProConfirmed: true, Confirmed: true,
	})
	if err != nil {
		t.Fatalf("VerifyModelEvidence returned error: %v", err)
	}
}

func TestVerifyModelEvidenceRejectsMissingPro(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model: "gpt-6-astra-pro", DesiredLabel: "GPT-6 Astra", ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status: "current", ObservedLabel: "GPT-6 Astra", Confirmed: true,
	})
	if err == nil || !strings.Contains(err.Error(), "Pro") {
		t.Fatalf("error = %v, want explicit Pro verification failure", err)
	}
}

func TestVerifyModelEvidenceRejectsStaleDesiredLabelBypass(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model: "gpt-6-astra-pro", DesiredLabel: "GPT-5.5", ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status: "current", ObservedLabel: "GPT-5.5 Pro", ProConfirmed: true, Confirmed: true,
	})
	if err == nil {
		t.Fatal("VerifyModelEvidence returned nil, want stale desired-label rejection")
	}
}

func TestVerifyModelEvidenceRejectsOtherGPT6AstraTiers(t *testing.T) {
	for _, observedLabel := range []string{"GPT-6 Astra Lite Pro", "GPT-6 Astra Terra Pro", "GPT-6 Astra Luna Pro"} {
		t.Run(observedLabel, func(t *testing.T) {
			err := browser.VerifyModelEvidence(browser.ModelSelection{
				Model: "gpt-6-astra-pro", DesiredLabel: "GPT-6 Astra", ThinkingTime: "extended",
			}, browser.ModelEvidence{
				Status: "current", ObservedLabel: observedLabel, ProConfirmed: true, Confirmed: true,
			})
			if err == nil {
				t.Fatalf("VerifyModelEvidence(%q) returned nil, want tier mismatch", observedLabel)
			}
		})
	}
}

func TestVerifyModelEvidenceRejectsVisibleButUnselectedPro(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model: "gpt-6-astra-pro", DesiredLabel: "GPT-6 Astra", ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status: "current", ObservedLabel: "GPT-6 Astra Pro", ThinkingTime: "extended", Confirmed: true,
	})
	if err == nil || !strings.Contains(err.Error(), "Pro") {
		t.Fatalf("error = %v, want explicit unselected Pro failure", err)
	}
}

func TestVerifyModelEvidenceRejectsProLabelFallbackWithoutExplicitConfirmation(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model: "gpt-6-astra-pro", DesiredLabel: "GPT-6 Astra", ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status: "current", ObservedLabel: "GPT-6 Astra Pro 확장", Confirmed: true,
	})
	if err == nil || !strings.Contains(err.Error(), "Pro") {
		t.Fatalf("error = %v, want explicit label-fallback Pro failure", err)
	}
}

func TestVerifyModelEvidenceRejectsProfileAsPro(t *testing.T) {
	err := browser.VerifyModelEvidence(browser.ModelSelection{
		Model: "gpt-6-astra-pro", DesiredLabel: "GPT-6 Astra", ThinkingTime: "extended",
	}, browser.ModelEvidence{
		Status: "current", ObservedLabel: "GPT-6 Astra profile", Confirmed: true,
	})
	if err == nil {
		t.Fatalf("error = %v, want profile label rejected", err)
	}
}
