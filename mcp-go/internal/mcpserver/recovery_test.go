package mcpserver_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"codex_oracle/internal/mcpserver"
	"codex_oracle/internal/session"
)

func recoveryCall(t *testing.T, server *mcpserver.Server, name string, args map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	response := server.HandleForTest(raw)
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	return response.Result.(map[string]any)
}

func TestChromeRecoveryPlan(t *testing.T) {
	for _, conversation := range []string{"https://chatgpt.com/c/recovery", "", "https://example.com/c/wrong"} {
		for _, status := range []string{"running", "completed", "failed"} {
			t.Run(status+"/"+conversation, func(t *testing.T) {
				store := session.NewStore(t.TempDir())
				meta, err := store.Create(session.CreateOptions{Mode: "chrome-assisted", Status: status,
					Request: map[string]any{"handoffDigest": "digest", "handoffNonce": "nonce"},
					Runtime: map[string]any{"conversationUrl": conversation}})
				if err != nil {
					t.Fatal(err)
				}
				reconnector := &fakeReattacher{}
				server := mcpserver.New(mcpserver.Options{Store: store, BrowserReattacher: reconnector})
				for _, dryRun := range []bool{true, false} {
					result := recoveryCall(t, server, "reattach", map[string]any{"id": meta.ID, "dryRun": dryRun})
					if result["isError"] == true {
						t.Fatalf("recovery failed: %v", result)
					}
					data := result["structuredContent"].(map[string]any)
					if data["executionMode"] != "chrome-assisted" || data["requiresBrowserCapture"] != true {
						t.Fatalf("missing Chrome recovery: %v", data)
					}
					if data["requiresForce"] != (status != "running") {
						t.Fatalf("incorrect overwrite guidance: %v", data)
					}
					finalize := data["finalize"].(map[string]any)
					if finalize["sessionId"] != meta.ID || finalize["handoffDigest"] != "digest" || finalize["handoffNonce"] != "nonce" {
						t.Fatalf("lost identity: %v", finalize)
					}
					if data["reattachable"] != (conversation == "https://chatgpt.com/c/recovery") {
						t.Fatalf("incorrect URL readiness: %v", data)
					}
					encoded, _ := json.Marshal(data)
					if strings.Contains(string(encoded), "login_setup") || strings.Contains(string(encoded), "example.com") {
						t.Fatalf("unsafe or obsolete guidance: %s", encoded)
					}
				}
				if reconnector.request.Endpoint != "" || reconnector.request.ConversationURL != "" {
					t.Fatal("Chrome recovery invoked CDP")
				}
				after, err := store.Read(meta.ID)
				if err != nil || after.Status != status {
					t.Fatalf("recovery changed session: %v %v", after, err)
				}
			})
		}
	}
}

func TestFinalizeRetriesAfterTranscriptWriteFailure(t *testing.T) {
	t.Setenv("ORACLE_SESSION_CONTENT_MODE", "full")
	store := session.NewStore(t.TempDir())
	server := mcpserver.New(mcpserver.Options{CWD: t.TempDir(), Store: store})
	prepared := recoveryCall(t, server, "consult_prepare", map[string]any{"prompt": "review", "preset": "chatgpt-pro-heavy"})
	data := prepared["structuredContent"].(map[string]any)
	id := data["sessionId"].(string)
	recovery := recoveryCall(t, server, "reattach", map[string]any{"id": id})
	plan := recovery["structuredContent"].(map[string]any)
	args, ok := plan["finalize"].(map[string]any)
	if !ok {
		t.Fatalf("missing recovery identity: %v", plan)
	}
	args["status"], args["answer"], args["transcript"] = "completed", "captured answer", "captured transcript"
	args["runtime"] = map[string]any{"conversationUrl": "https://chatgpt.com/c/recovery"}
	blocker := filepath.Join(store.SessionsDir(), id, "artifacts", "transcript.md")
	if err := os.MkdirAll(blocker, 0o755); err != nil {
		t.Fatal(err)
	}
	result := recoveryCall(t, server, "consult_finalize", args)
	if result["isError"] != true {
		t.Fatal("expected transcript write failure")
	}
	meta, err := store.Read(id)
	if err != nil || meta.Status != "running" {
		t.Fatalf("premature completion: %v %v", meta, err)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	result = recoveryCall(t, server, "consult_finalize", args)
	if result["isError"] == true {
		t.Fatalf("retry failed: %v", result)
	}
	detail, err := store.ReadDetail(id)
	if err != nil || detail.Metadata.Status != "completed" || detail.Log != "captured answer" {
		t.Fatalf("retry not saved: %v %v", detail, err)
	}
}

func TestCheckpointSurvivesRestartAndPreservesContent(t *testing.T) {
	for _, mode := range []string{"metadata", "full"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ORACLE_SESSION_CONTENT_MODE", mode)
			store := session.NewStore(t.TempDir())
			server := mcpserver.New(mcpserver.Options{CWD: t.TempDir(), Store: store})
			prepared := recoveryCall(t, server, "consult_prepare", map[string]any{"prompt": "private checkpoint prompt", "preset": "chatgpt-pro-heavy"})
			data := prepared["structuredContent"].(map[string]any)
			checkpoint := data["checkpoint"].(map[string]any)
			args := checkpoint["arguments"].(map[string]any)
			id := args["sessionId"].(string)
			// A checkpoint must never erase existing captured content, even after partial failure.
			if err := store.UpdateResult(id, session.UpdateOptions{Status: "running", Output: "partial answer", Transcript: "partial transcript"}); err != nil {
				t.Fatal(err)
			}
			before, err := store.ReadDetail(id)
			if err != nil {
				t.Fatal(err)
			}
			for _, bad := range []string{"https://chatgpt.com/", "https://chatgpt.com/c/", "https://example.com/c/test", "http://chatgpt.com/c/test", "https://user:pass@chatgpt.com/c/test", "https://chatgpt.com/c/test?token=private", "https://chatgpt.com/c/test#private"} {
				args["conversationUrl"] = bad
				if recoveryCall(t, server, "consult_checkpoint", args)["isError"] != true {
					t.Fatalf("accepted URL %q", bad)
				}
			}
			args["conversationUrl"] = "https://chatgpt.com/c/WEB:c94248fe-c4fa-4b09-98f2-ec5267717696"
			pending := recoveryCall(t, server, "consult_checkpoint", args)
			if pending["isError"] != true || !strings.Contains(fmt.Sprint(pending), "retry without resubmitting") {
				t.Fatalf("missing temporary URL recovery guidance: %v", pending)
			}
			unchanged, err := store.ReadDetail(id)
			if err != nil || unchanged.Metadata.Runtime["conversationUrl"] != before.Metadata.Runtime["conversationUrl"] || unchanged.Log != before.Log {
				t.Fatalf("temporary URL changed session: %v %v", unchanged, err)
			}
			url := "https://chatgpt.com/c/checkpoint-test"
			args["conversationUrl"] = url
			nonce := args["handoffNonce"]
			args["handoffNonce"] = "wrong"
			if recoveryCall(t, server, "consult_checkpoint", args)["isError"] != true {
				t.Fatal("accepted wrong identity")
			}
			args["handoffNonce"] = nonce
			for i := 0; i < 2; i++ {
				if result := recoveryCall(t, server, "consult_checkpoint", args); result["isError"] == true {
					t.Fatalf("checkpoint/retry failed: %v", result)
				}
			}
			args["conversationUrl"] = "https://chatgpt.com/c/wrong"
			if recoveryCall(t, server, "consult_checkpoint", args)["isError"] != true {
				t.Fatal("rebound conversation")
			}
			server = mcpserver.New(mcpserver.Options{Store: session.NewStore(store.HomeDir())})
			plan := recoveryCall(t, server, "reattach", map[string]any{"id": id})["structuredContent"].(map[string]any)
			if plan["reattachable"] != true || plan["runtime"].(map[string]any)["conversationUrl"] != url {
				t.Fatalf("lost checkpoint on restart: %v", plan)
			}
			after, err := store.ReadDetail(id)
			if err != nil || after.Log != before.Log || after.Metadata.Status != "running" {
				t.Fatalf("checkpoint changed content/status: %v %v", after, err)
			}
			finalize := plan["finalize"].(map[string]any)
			finalize["status"], finalize["answer"] = "completed", "final answer"
			finalize["runtime"] = map[string]any{"conversationUrl": "https://chatgpt.com/c/wrong"}
			if recoveryCall(t, server, "consult_finalize", finalize)["isError"] != true {
				t.Fatal("accepted answer from another conversation")
			}
			finalize["runtime"] = map[string]any{"conversationUrl": url}
			if result := recoveryCall(t, server, "consult_finalize", finalize); result["isError"] == true {
				t.Fatalf("finalize failed: %v", result)
			}
			args["conversationUrl"] = url
			if recoveryCall(t, server, "consult_checkpoint", args)["isError"] != true {
				t.Fatal("checkpoint changed terminal session")
			}
			if mode == "metadata" {
				if err := filepath.WalkDir(filepath.Join(store.SessionsDir(), id), func(path string, entry os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if entry.IsDir() {
						return nil
					}
					body, err := os.ReadFile(path)
					if strings.Contains(string(body), "private checkpoint prompt") || strings.Contains(string(body), "partial answer") || strings.Contains(string(body), "final answer") {
						t.Fatalf("content persisted: %s", path)
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
