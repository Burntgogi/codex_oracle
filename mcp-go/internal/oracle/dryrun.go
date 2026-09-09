package oracle

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func BuildDryRunResolved(input ConsultInput, files []string, env map[string]string) DryRunResolved {
	engine := resolveEngine(input, env)
	model := resolveModel(input, engine)
	browserModelMigrated := false
	originalBrowserModel := ""
	if engine == "browser" {
		originalBrowserModel = strings.TrimSpace(model)
		model, browserModelMigrated = NormalizeBrowserModel(model)
	}
	guidance := []string{}
	var browser *BrowserResolved
	if engine == "api" {
		guidance = append(guidance, `API engine requires provider credentials. If the operator has ChatGPT Pro but no API key, retry with engine:"browser" or preset:"chatgpt-pro-heavy".`)
	}
	if engine == "browser" {
		browser = buildBrowserResolved(input, model, env)
		guidance = append(guidance, "Browser engine uses a signed-in ChatGPT profile; run dryRun:true before live use.")
		if browserModelMigrated {
			guidance = append(guidance, fmt.Sprintf(`Browser model %q was normalized to "gpt-6-astra-pro" (GPT-6 Astra + Pro).`, originalBrowserModel))
		}
		if browser.ManualLogin {
			guidance = append(guidance, "Manual-login browser mode uses a private automation profile separate from the normal Chrome profile.")
			guidance = append(guidance, "If this profile is not signed in, live browser runs must fail fast with login guidance.")
		}
	}
	followUps := 0
	for _, followUp := range input.BrowserFollowUps {
		if strings.TrimSpace(followUp) != "" {
			followUps++
		}
	}
	if followUps > 0 {
		guidance = append(guidance, "This is a multi-turn browser consult; all follow-ups stay in one ChatGPT conversation.")
	}
	return DryRunResolved{
		ResolvedEngine: engine,
		Model:          model,
		Models:         input.Models,
		Files:          files,
		FollowUpCount:  followUps,
		Browser:        browser,
		Guidance:       guidance,
	}
}

func FormatDryRun(resolved DryRunResolved) string {
	lines := []string{
		"[dry-run] MCP resolved request:",
		"  engine: " + resolved.ResolvedEngine,
		"  model: " + resolved.Model,
		"  files: " + itoa(len(resolved.Files)),
	}
	if len(resolved.Models) > 0 {
		lines = append(lines, "  models: "+strings.Join(resolved.Models, ", "))
	}
	if resolved.Browser != nil {
		browser := resolved.Browser
		lines = append(lines, "  browser desired model: "+defaultText(browser.DesiredModel, "(default)"))
		lines = append(lines, "  browser thinking time: "+defaultText(browser.ThinkingTime, "(default)"))
		lines = append(lines, "  browser thinking label: "+defaultText(browser.ThinkingLabel, "(default)"))
		lines = append(lines, "  browser model strategy: "+defaultText(browser.ModelStrategy, "(default)"))
		lines = append(lines, "  browser research mode: "+defaultText(browser.ResearchMode, "off"))
		lines = append(lines, "  browser attachments: "+defaultText(browser.Attachments, "auto"))
		lines = append(lines, "  browser bundle files: "+yesNo(browser.BundleFiles))
		lines = append(lines, "  browser bundle format: "+defaultText(browser.BundleFormat, "auto"))
		lines = append(lines, "  browser keep browser: "+yesNo(browser.KeepBrowser))
		lines = append(lines, "  browser manual login: "+yesNo(browser.ManualLogin))
		if browser.ProfileDir != "" {
			lines = append(lines, "  browser profile: "+browser.ProfileDir)
		}
		if browser.ChatGPTURL != "" {
			lines = append(lines, "  ChatGPT URL: "+browser.ChatGPTURL)
		}
	}
	lines = append(lines, "  follow-ups: "+itoa(resolved.FollowUpCount))
	for _, guidance := range resolved.Guidance {
		lines = append(lines, "  guidance: "+guidance)
	}
	return strings.Join(lines, "\n")
}

func resolveEngine(input ConsultInput, env map[string]string) string {
	if input.Engine != "" {
		return input.Engine
	}
	if configured := strings.TrimSpace(env["ORACLE_ENGINE"]); configured != "" {
		return configured
	}
	if strings.TrimSpace(env["OPENAI_API_KEY"]) != "" {
		return "api"
	}
	return "browser"
}

func resolveModel(input ConsultInput, engine string) string {
	if input.Model != "" {
		return input.Model
	}
	if len(input.Models) > 0 {
		return input.Models[0]
	}
	if engine == "api" {
		return DefaultAPIModel
	}
	return DefaultBrowserModel
}

func buildBrowserResolved(input ConsultInput, model string, env map[string]string) *BrowserResolved {
	if model == DefaultBrowserModel && strings.TrimSpace(input.BrowserThinkingTime) == "" {
		input.BrowserThinkingTime = "extended"
	}
	profileDir := strings.TrimSpace(env["ORACLE_BROWSER_PROFILE_DIR"])
	manualLogin := profileDir != "" || runtime.GOOS == "windows"
	if profileDir == "" {
		profileDir = filepath.Join(defaultHome(), "browser-profile")
	}
	chatgptURL := strings.TrimSpace(env["ORACLE_CHATGPT_URL"])
	if chatgptURL == "" {
		chatgptURL = DefaultChatGPTURL
	}
	imageOutputPath := input.GenerateImage
	if imageOutputPath == "" {
		imageOutputPath = input.OutputPath
	}
	return &BrowserResolved{
		DesiredModel:    browserLabel(input, model),
		ThinkingTime:    input.BrowserThinkingTime,
		ThinkingLabel:   thinkingLabel(input.BrowserThinkingTime),
		ModelStrategy:   input.BrowserModelStrategy,
		ResearchMode:    input.BrowserResearchMode,
		Attachments:     input.BrowserAttachments,
		BundleFiles:     input.BrowserBundleFiles,
		BundleFormat:    input.BrowserBundleFormat,
		KeepBrowser:     input.BrowserKeepBrowser,
		ManualLogin:     manualLogin,
		ProfileDir:      profileDir,
		ChatGPTURL:      chatgptURL,
		ImageOutputPath: imageOutputPath,
	}
}

func browserLabel(input ConsultInput, model string) string {
	if model == DefaultBrowserModel || strings.EqualFold(strings.TrimSpace(model), LegacyBrowserModel) {
		return DefaultBrowserModelLabel
	}
	if input.BrowserModelLabel != "" {
		return input.BrowserModelLabel
	}
	return model
}

func thinkingLabel(thinkingTime string) string {
	switch strings.ToLower(strings.TrimSpace(thinkingTime)) {
	case "extended":
		return DefaultBrowserThinkingLabel
	default:
		return strings.TrimSpace(thinkingTime)
	}
}

func defaultHome() string {
	if configured := strings.TrimSpace(os.Getenv("ORACLE_HOME_DIR")); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".codex_oracle"
	}
	return filepath.Join(home, ".codex_oracle")
}

func defaultText(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[i:])
}
