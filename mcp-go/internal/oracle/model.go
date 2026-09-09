package oracle

import "strings"

func NormalizeBrowserModel(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	switch strings.ToLower(trimmed) {
	case DefaultBrowserModel:
		return DefaultBrowserModel, false
	case DefaultAPIModel, LegacyBrowserModel, "gpt-5.6-sol-pro", "gpt-5.6-sol", "gpt-6-astra", "astra 6 pro":
		return DefaultBrowserModel, true
	default:
		return trimmed, false
	}
}
