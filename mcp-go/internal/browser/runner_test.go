package browser_test

import (
	"context"
	"testing"

	"codex_oracle/internal/browser"
)

func TestNotImplementedRunnerFailsClosed(t *testing.T) {
	runner := browser.NotImplementedRunner{}

	_, err := runner.Run(context.Background(), browser.Request{Prompt: "review"})
	if err == nil {
		t.Fatal("Run returned nil error, want fail-closed not implemented error")
	}
}
