package oracle

import "fmt"

func ApplyConsultPreset(input ConsultInput) (ConsultInput, error) {
	if input.Preset == "" {
		return input, nil
	}
	if input.Preset != PresetChatGPTProHeavy {
		return input, fmt.Errorf("unknown consult preset %q", input.Preset)
	}
	if len(input.Models) > 0 {
		return input, fmt.Errorf("MCP consult preset %q cannot be combined with models", PresetChatGPTProHeavy)
	}
	if input.Engine == "" {
		input.Engine = "browser"
	}
	if input.Model == "" {
		input.Model = DefaultBrowserModel
	}
	if input.BrowserThinkingTime == "" {
		input.BrowserThinkingTime = "extended"
	}
	return input, nil
}
