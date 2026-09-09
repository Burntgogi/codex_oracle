package browser

import "testing"

func TestRuntimeMetadataIncludesReattachFields(t *testing.T) {
	runtime := runtimeMetadata(runtimeMetadataInput{
		PID:             1234,
		Endpoint:        "ws://127.0.0.1:9222/devtools/browser/abc",
		TargetID:        "target-1",
		ConversationURL: "https://chatgpt.com/c/abc",
		ProfileDir:      "profile",
		ChatGPTURL:      "https://chatgpt.com/",
		Reattached:      true,
	})

	if runtime["pid"] != 1234 {
		t.Fatalf("pid = %#v, want 1234", runtime["pid"])
	}
	if runtime["endpoint"] != "ws://127.0.0.1:9222/devtools/browser/abc" {
		t.Fatalf("endpoint = %#v, want stored endpoint", runtime["endpoint"])
	}
	if runtime["targetId"] != "target-1" {
		t.Fatalf("targetId = %#v, want target-1", runtime["targetId"])
	}
	if runtime["conversationUrl"] != "https://chatgpt.com/c/abc" {
		t.Fatalf("conversationUrl = %#v, want conversation URL", runtime["conversationUrl"])
	}
	if runtime["profileDir"] != "profile" {
		t.Fatalf("profileDir = %#v, want profile", runtime["profileDir"])
	}
	if runtime["chatgptUrl"] != "https://chatgpt.com/" {
		t.Fatalf("chatgptUrl = %#v, want ChatGPT URL", runtime["chatgptUrl"])
	}
	if runtime["reattached"] != true {
		t.Fatalf("reattached = %#v, want true", runtime["reattached"])
	}
	if runtime["lastActivityAt"] == "" {
		t.Fatal("lastActivityAt is empty")
	}
}
