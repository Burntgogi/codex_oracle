package oracle_test

import (
	"strings"
	"testing"

	"codex_oracle/internal/oracle"
)

func TestApplyConsultPresetChatGPTProHeavy(t *testing.T) {
	input := oracle.ConsultInput{
		Preset: "chatgpt-pro-heavy",
		Prompt: "review the plan",
	}

	resolved, err := oracle.ApplyConsultPreset(input)
	if err != nil {
		t.Fatalf("ApplyConsultPreset returned error: %v", err)
	}

	if resolved.Engine != "browser" {
		t.Fatalf("Engine = %q, want browser", resolved.Engine)
	}
	if resolved.Model != "gpt-6-astra-pro" {
		t.Fatalf("Model = %q, want gpt-6-astra-pro", resolved.Model)
	}
	if resolved.BrowserThinkingTime != "extended" {
		t.Fatalf("BrowserThinkingTime = %q, want extended", resolved.BrowserThinkingTime)
	}
}

func TestApplyConsultPresetRejectsModels(t *testing.T) {
	input := oracle.ConsultInput{
		Preset: "chatgpt-pro-heavy",
		Prompt: "review the plan",
		Models: []string{"gpt-5.5-pro", "claude-sonnet"},
	}

	_, err := oracle.ApplyConsultPreset(input)
	if err == nil {
		t.Fatal("ApplyConsultPreset returned nil error, want incompatible models error")
	}
}

func TestBuildDryRunResolvedSplitsGPT6AstraModelAndPro(t *testing.T) {
	input := oracle.ConsultInput{
		Preset:              "chatgpt-pro-heavy",
		Prompt:              "review the plan",
		Engine:              "browser",
		Model:               "gpt-6-astra-pro",
		BrowserThinkingTime: "extended",
	}

	resolved := oracle.BuildDryRunResolved(input, []string{"README.md"}, map[string]string{})
	if resolved.Browser == nil {
		t.Fatal("Browser = nil, want browser settings")
	}
	if resolved.Model != "gpt-6-astra-pro" {
		t.Fatalf("Model = %q, want gpt-6-astra-pro", resolved.Model)
	}
	if resolved.Browser.DesiredModel != "GPT-6 Astra" {
		t.Fatalf("DesiredModel = %q, want GPT-6 Astra", resolved.Browser.DesiredModel)
	}
	if resolved.Browser.ThinkingLabel != "Pro" {
		t.Fatalf("ThinkingLabel = %q, want Pro", resolved.Browser.ThinkingLabel)
	}
}

func TestBuildDryRunResolvedNormalizesLegacyBrowserModel(t *testing.T) {
	resolved := oracle.BuildDryRunResolved(oracle.ConsultInput{
		Engine:              "browser",
		Model:               "gpt-5.5-pro",
		BrowserModelLabel:   "GPT-5.5",
		BrowserThinkingTime: "extended",
	}, nil, map[string]string{})
	if resolved.Model != "gpt-6-astra-pro" {
		t.Fatalf("Model = %q, want gpt-6-astra-pro", resolved.Model)
	}
	if resolved.Browser.DesiredModel != "GPT-6 Astra" || resolved.Browser.ThinkingLabel != "Pro" {
		t.Fatalf("Browser = %#v, want GPT-6 Astra + Pro", resolved.Browser)
	}
	if len(resolved.Guidance) == 0 || !strings.Contains(strings.Join(resolved.Guidance, " "), "gpt-5.5-pro") {
		t.Fatalf("Guidance = %#v, want legacy migration message", resolved.Guidance)
	}
}

func TestBuildDryRunResolvedNormalizesExplicitGPT55BrowserModel(t *testing.T) {
	resolved := oracle.BuildDryRunResolved(oracle.ConsultInput{
		Engine:              "browser",
		Model:               " GPT-5.5 ",
		BrowserModelLabel:   "GPT-5.5",
		BrowserThinkingTime: "extended",
	}, nil, map[string]string{})
	if resolved.Model != oracle.DefaultBrowserModel {
		t.Fatalf("Model = %q, want %q", resolved.Model, oracle.DefaultBrowserModel)
	}
	if resolved.Browser.DesiredModel != oracle.DefaultBrowserModelLabel || resolved.Browser.ThinkingLabel != oracle.DefaultBrowserThinkingLabel {
		t.Fatalf("Browser = %#v, want GPT-6 Astra + Pro", resolved.Browser)
	}
	if !strings.Contains(strings.Join(resolved.Guidance, " "), "GPT-5.5") {
		t.Fatalf("Guidance = %#v, want original normalized model input", resolved.Guidance)
	}
}

func TestChatGPTProHeavyExplicitGPT55UsesBrowserCanonicalModel(t *testing.T) {
	input, err := oracle.ApplyConsultPreset(oracle.ConsultInput{
		Preset: "chatgpt-pro-heavy",
		Model:  "gpt-5.5",
	})
	if err != nil {
		t.Fatalf("ApplyConsultPreset returned error: %v", err)
	}
	resolved := oracle.BuildDryRunResolved(input, nil, map[string]string{})
	if resolved.Model != oracle.DefaultBrowserModel {
		t.Fatalf("Model = %q, want %q", resolved.Model, oracle.DefaultBrowserModel)
	}
	if resolved.Browser.DesiredModel != oracle.DefaultBrowserModelLabel || resolved.Browser.ThinkingLabel != oracle.DefaultBrowserThinkingLabel {
		t.Fatalf("Browser = %#v, want GPT-6 Astra + Pro", resolved.Browser)
	}
}

func TestBuildDryRunResolvedKeepsAPIDefaultModel(t *testing.T) {
	resolved := oracle.BuildDryRunResolved(oracle.ConsultInput{Engine: "api"}, nil, map[string]string{})
	if resolved.Model != oracle.DefaultAPIModel {
		t.Fatalf("Model = %q, want unchanged API default %q", resolved.Model, oracle.DefaultAPIModel)
	}
	if resolved.Browser != nil {
		t.Fatalf("Browser = %#v, want nil for API engine", resolved.Browser)
	}
}
