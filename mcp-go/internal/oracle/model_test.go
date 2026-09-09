package oracle_test

import (
	"testing"

	"codex_oracle/internal/oracle"
)

func TestNormalizeBrowserModel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		want     string
		migrated bool
	}{
		{name: "current", input: "gpt-6-astra-pro", want: "gpt-6-astra-pro"},
		{name: "astra API name on browser path", input: "gpt-6-astra", want: "gpt-6-astra-pro", migrated: true},
		{name: "requested name", input: "Astra 6 Pro", want: "gpt-6-astra-pro", migrated: true},
		{name: "previous Sol preset", input: "gpt-5.6-sol-pro", want: "gpt-6-astra-pro", migrated: true},
		{name: "api model on browser path", input: " GPT-5.5 ", want: "gpt-6-astra-pro", migrated: true},
		{name: "legacy", input: "gpt-5.5-pro", want: "gpt-6-astra-pro", migrated: true},
		{name: "legacy case and space", input: " GPT-5.5-PRO ", want: "gpt-6-astra-pro", migrated: true},
		{name: "custom", input: "custom-browser-model", want: "custom-browser-model"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, migrated := oracle.NormalizeBrowserModel(tt.input)
			if got != tt.want || migrated != tt.migrated {
				t.Fatalf("NormalizeBrowserModel(%q) = (%q, %v), want (%q, %v)", tt.input, got, migrated, tt.want, tt.migrated)
			}
		})
	}
}

func TestAstraProWithoutPreset(t *testing.T) {
	resolved := oracle.BuildDryRunResolved(oracle.ConsultInput{Engine: "browser", Model: "gpt-6-astra"}, nil, nil)
	if resolved.Model != "gpt-6-astra-pro" || resolved.Browser.ThinkingLabel != "Pro" {
		t.Fatalf("missing Pro contract: %#v", resolved)
	}
	handoff, err := oracle.BuildChromeHandoff(resolved, "review", nil)
	if err != nil || handoff.ModelLabel != "GPT-6 Astra" || handoff.ThinkingLabel != "Pro" {
		t.Fatalf("handoff: %#v %v", handoff, err)
	}
}
