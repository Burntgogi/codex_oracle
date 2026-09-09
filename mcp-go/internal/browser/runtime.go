package browser

import "time"

type runtimeMetadataInput struct {
	PID             int
	Endpoint        string
	TargetID        string
	ConversationURL string
	ProfileDir      string
	ChatGPTURL      string
	Reattached      bool
}

func runtimeMetadata(input runtimeMetadataInput) map[string]any {
	runtime := map[string]any{
		"endpoint":       input.Endpoint,
		"targetId":       input.TargetID,
		"profileDir":     input.ProfileDir,
		"chatgptUrl":     input.ChatGPTURL,
		"lastActivityAt": time.Now().UTC().Format(time.RFC3339),
	}
	if input.PID > 0 {
		runtime["pid"] = input.PID
	}
	if input.ConversationURL != "" {
		runtime["conversationUrl"] = input.ConversationURL
	}
	if input.Reattached {
		runtime["reattached"] = true
	}
	return runtime
}
