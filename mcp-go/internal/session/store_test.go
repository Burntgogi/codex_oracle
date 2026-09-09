package session_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"codex_oracle/internal/session"
)

func TestStoreListsSessionsNewestFirst(t *testing.T) {
	home := t.TempDir()
	store := session.NewStore(home)
	writeSession(t, home, "older", "2026-07-05T01:00:00Z", "completed")
	writeSession(t, home, "newer", "2026-07-05T02:00:00Z", "running")

	entries, err := store.List(session.ListOptions{IncludeAll: true, Limit: 10})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2", len(entries))
	}
	if entries[0].ID != "newer" || entries[1].ID != "older" {
		t.Fatalf("entries order = %#v, want newer then older", entries)
	}
}

func TestStoreReadsSessionDetail(t *testing.T) {
	home := t.TempDir()
	store := session.NewStore(home)
	writeSession(t, home, "abc", "2026-07-05T02:00:00Z", "completed")
	sessionDir := filepath.Join(home, "sessions", "abc")
	mustWrite(t, filepath.Join(sessionDir, "request.json"), `{"prompt":"hello"}`)
	mustWrite(t, filepath.Join(sessionDir, "output.log"), "answer")

	detail, err := store.ReadDetail("abc")
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}

	if detail.Metadata.ID != "abc" {
		t.Fatalf("detail.Metadata.ID = %q, want abc", detail.Metadata.ID)
	}
	if detail.Request["prompt"] != "hello" {
		t.Fatalf("detail.Request = %#v, want prompt hello", detail.Request)
	}
	if detail.Log != "answer" {
		t.Fatalf("detail.Log = %q, want answer", detail.Log)
	}
}

func TestStoreCreateWritesTranscriptArtifact(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	home := t.TempDir()
	store := session.NewStore(home)

	meta, err := store.Create(session.CreateOptions{
		Prompt:     "hello",
		Model:      "gpt-5.5-pro",
		Mode:       "browser",
		Status:     "completed",
		Output:     "answer",
		Transcript: "# Transcript\n\nanswer\n",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	transcriptPath := filepath.Join(home, "sessions", meta.ID, "artifacts", "transcript.md")
	data, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("ReadFile transcript: %v", err)
	}
	if string(data) != "# Transcript\n\nanswer\n" {
		t.Fatalf("transcript = %q, want provided transcript", string(data))
	}
}

func TestStoreDefaultMetadataModeDoesNotPersistPromptOutputOrTranscript(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "")
	home := t.TempDir()
	store := session.NewStore(home)

	meta, err := store.Create(session.CreateOptions{
		Prompt:     "sensitive prompt",
		Model:      "gpt-5.5-pro",
		Mode:       "chrome-assisted",
		Status:     "completed",
		Output:     "sensitive answer",
		Transcript: "# Transcript\n\nsensitive answer\n",
		Request: map[string]any{
			"bundleText":       "sensitive bundle",
			"handoff":          map[string]any{"prompt": "sensitive handoff prompt"},
			"browserFollowUps": []string{"sensitive browser follow-up"},
			"followUps":        []string{"sensitive follow-up"},
			"files":            []string{"README.md"},
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	sessionDir := filepath.Join(home, "sessions", meta.ID)
	if data, err := os.ReadFile(filepath.Join(sessionDir, "output.log")); err != nil {
		t.Fatalf("ReadFile output.log: %v", err)
	} else if string(data) != "" {
		t.Fatalf("output.log = %q, want empty in metadata mode", string(data))
	}
	if _, err := os.Stat(filepath.Join(sessionDir, "artifacts", "transcript.md")); !os.IsNotExist(err) {
		t.Fatalf("transcript exists in metadata mode, err=%v", err)
	}
	detail, err := store.ReadDetail(meta.ID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if _, ok := detail.Request["bundleText"]; ok {
		t.Fatalf("request persisted bundleText in metadata mode: %#v", detail.Request)
	}
	if _, ok := detail.Request["prompt"]; ok {
		t.Fatalf("request persisted prompt in metadata mode: %#v", detail.Request)
	}
	if _, ok := detail.Request["handoff"]; ok {
		t.Fatalf("request persisted handoff in metadata mode: %#v", detail.Request)
	}
	if _, ok := detail.Request["browserFollowUps"]; ok {
		t.Fatalf("request persisted browserFollowUps in metadata mode: %#v", detail.Request)
	}
	if _, ok := detail.Request["followUps"]; ok {
		t.Fatalf("request persisted followUps in metadata mode: %#v", detail.Request)
	}
}

func TestStoreCreatePersistsRuntimeMetadata(t *testing.T) {
	home := t.TempDir()
	store := session.NewStore(home)

	meta, err := store.Create(session.CreateOptions{
		Prompt: "hello",
		Model:  "gpt-5.5-pro",
		Mode:   "browser",
		Status: "completed",
		Output: "answer",
		Runtime: map[string]any{
			"pid":             1234,
			"endpoint":        "ws://127.0.0.1:9222/devtools/browser/abc",
			"targetId":        "target-1",
			"conversationUrl": "https://chatgpt.com/c/abc",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	detail, err := store.ReadDetail(meta.ID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if detail.Metadata.Runtime["targetId"] != "target-1" {
		t.Fatalf("Runtime = %#v, want targetId", detail.Metadata.Runtime)
	}
	if detail.Metadata.Runtime["conversationUrl"] != "https://chatgpt.com/c/abc" {
		t.Fatalf("Runtime = %#v, want conversationUrl", detail.Metadata.Runtime)
	}
}

func TestStoreWriteArtifactWritesInsideSession(t *testing.T) {
	home := t.TempDir()
	store := session.NewStore(home)

	meta, err := store.Create(session.CreateOptions{
		Prompt: "hello",
		Model:  "gpt-5.5-pro",
		Mode:   "chrome-assisted",
		Status: "running",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := store.WriteArtifact(meta.ID, "artifacts/handoff.json", []byte(`{"model":"gpt-5.5-pro"}`)); err != nil {
		t.Fatalf("WriteArtifact returned error: %v", err)
	}

	artifactPath := filepath.Join(home, "sessions", meta.ID, "artifacts", "handoff.json")
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("ReadFile artifact: %v", err)
	}
	if string(data) != `{"model":"gpt-5.5-pro"}` {
		t.Fatalf("artifact = %q, want handoff json", string(data))
	}
}

func TestStoreWriteArtifactRejectsPathTraversal(t *testing.T) {
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{
		Prompt: "hello",
		Status: "running",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := store.WriteArtifact(meta.ID, "../escape.json", []byte("escape")); err == nil {
		t.Fatal("WriteArtifact returned nil error for traversal path")
	}
}

func TestStoreUpdateResultUpdatesExistingSession(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	home := t.TempDir()
	store := session.NewStore(home)

	meta, err := store.Create(session.CreateOptions{
		Prompt: "hello",
		Model:  "gpt-5.5-pro",
		Mode:   "browser",
		Status: "running",
		Output: "partial",
		Runtime: map[string]any{
			"endpoint": "ws://127.0.0.1:9222/devtools/browser/abc",
		},
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := store.UpdateResult(meta.ID, session.UpdateOptions{
		Status:     "completed",
		Output:     "final answer",
		Transcript: "# Transcript\n\nfinal answer\n",
		Runtime: map[string]any{
			"targetId":        "target-1",
			"conversationUrl": "https://chatgpt.com/c/abc",
		},
	}); err != nil {
		t.Fatalf("UpdateResult returned error: %v", err)
	}

	detail, err := store.ReadDetail(meta.ID)
	if err != nil {
		t.Fatalf("ReadDetail returned error: %v", err)
	}
	if detail.Metadata.Status != "completed" {
		t.Fatalf("status = %q, want completed", detail.Metadata.Status)
	}
	if detail.Log != "final answer" {
		t.Fatalf("log = %q, want final answer", detail.Log)
	}
	if detail.Metadata.Runtime["endpoint"] != "ws://127.0.0.1:9222/devtools/browser/abc" {
		t.Fatalf("Runtime = %#v, want original endpoint", detail.Metadata.Runtime)
	}
	if detail.Metadata.Runtime["targetId"] != "target-1" {
		t.Fatalf("Runtime = %#v, want targetId", detail.Metadata.Runtime)
	}
	transcriptPath := filepath.Join(home, "sessions", meta.ID, "artifacts", "transcript.md")
	transcript, err := os.ReadFile(transcriptPath)
	if err != nil {
		t.Fatalf("ReadFile transcript: %v", err)
	}
	if string(transcript) != "# Transcript\n\nfinal answer\n" {
		t.Fatalf("transcript = %q, want updated transcript", string(transcript))
	}
}

func TestStoreUpdateHeartbeatPreservesStatusAndWritesTimestamp(t *testing.T) {
	home := t.TempDir()
	store := session.NewStore(home)

	meta, err := store.Create(session.CreateOptions{
		Prompt: "hello",
		Model:  "gpt-5.5-pro",
		Mode:   "browser",
		Status: "running",
		Output: "",
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	if err := store.UpdateHeartbeat(meta.ID); err != nil {
		t.Fatalf("UpdateHeartbeat returned error: %v", err)
	}

	updated, err := store.Read(meta.ID)
	if err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if updated.Status != "running" {
		t.Fatalf("status = %q, want running", updated.Status)
	}
	if updated.LastHeartbeatAt == "" {
		t.Fatal("LastHeartbeatAt is empty")
	}
	if _, err := time.Parse(time.RFC3339, updated.LastHeartbeatAt); err != nil {
		t.Fatalf("LastHeartbeatAt = %q is not RFC3339: %v", updated.LastHeartbeatAt, err)
	}
}

func TestStoreFailedResultWriteRemainsRetryable(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	for _, blocked := range []string{"output.log", "artifacts/transcript.md"} {
		t.Run(blocked, func(t *testing.T) {
			store := session.NewStore(t.TempDir())
			meta, err := store.Create(session.CreateOptions{Status: "running"})
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(store.SessionsDir(), meta.ID, blocked)
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatal(err)
			}
			result := session.UpdateOptions{Status: "completed", Output: "answer", Transcript: "transcript", Runtime: map[string]any{"conversationUrl": "https://chatgpt.com/c/test"}}
			if err := store.UpdateResult(meta.ID, result); err == nil {
				t.Fatal("blocked result write succeeded")
			}
			current, err := store.Read(meta.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Status != "running" || len(current.Runtime) != 0 {
				t.Fatalf("failed write committed metadata: %#v", current)
			}
			leftovers, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".oracle-write-*"))
			if err != nil || len(leftovers) != 0 {
				t.Fatalf("temporary files remain: %v (%v)", leftovers, err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateResult(meta.ID, result); err != nil {
				t.Fatal(err)
			}
			detail, err := store.ReadDetail(meta.ID)
			if err != nil {
				t.Fatal(err)
			}
			if detail.Metadata.Status != "completed" || detail.Log != "answer" {
				t.Fatalf("retry did not finish: %#v", detail)
			}
			transcript, err := os.ReadFile(filepath.Join(store.SessionsDir(), meta.ID, "artifacts", "transcript.md"))
			if err != nil || string(transcript) != "transcript" {
				t.Fatalf("transcript = %q, err = %v", transcript, err)
			}
		})
	}
}

func TestStoreResultAndHeartbeatPreserveCompletion(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "")
	store := session.NewStore(t.TempDir())
	meta, err := store.Create(session.CreateOptions{Status: "running"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		if err := store.UpdateResult(meta.ID, session.UpdateOptions{Status: "running"}); err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		results := make(chan error, 2)
		go func() { <-start; results <- store.UpdateHeartbeat(meta.ID) }()
		go func() {
			<-start
			results <- store.UpdateResult(meta.ID, session.UpdateOptions{Status: "completed", Output: "private answer", Transcript: "private transcript"})
		}()
		close(start)
		for j := 0; j < 2; j++ {
			if err := <-results; err != nil {
				t.Fatal(err)
			}
		}
		detail, err := store.ReadDetail(meta.ID)
		if err != nil {
			t.Fatal(err)
		}
		if detail.Metadata.Status != "completed" || detail.Metadata.LastHeartbeatAt == "" || detail.Log != "" {
			t.Fatalf("completion/heartbeat/privacy lost: %#v", detail)
		}
	}
	if _, err := os.Stat(filepath.Join(store.SessionsDir(), meta.ID, "artifacts", "transcript.md")); !os.IsNotExist(err) {
		t.Fatalf("metadata-only transcript exists: %v", err)
	}
}

func writeSession(t *testing.T, home, id, createdAt, status string) {
	t.Helper()
	sessionDir := filepath.Join(home, "sessions", id)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	mustWrite(t, filepath.Join(sessionDir, "meta.json"), `{
  "id": "`+id+`",
  "createdAt": "`+createdAt+`",
  "status": "`+status+`",
  "model": "gpt-5.5-pro",
  "mode": "browser"
}`)
}

func mustWrite(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
