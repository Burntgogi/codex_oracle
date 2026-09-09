package browser_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"codex_oracle/internal/browser"
)

func TestFindChromePathHonorsEnvOverride(t *testing.T) {
	chromePath := filepath.Join(t.TempDir(), "chrome.exe")
	if err := os.WriteFile(chromePath, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	found, err := browser.FindChromePath(map[string]string{"ORACLE_BROWSER_CHROME_PATH": chromePath})
	if err != nil {
		t.Fatalf("FindChromePath returned error: %v", err)
	}
	if found != chromePath {
		t.Fatalf("found = %q, want %q", found, chromePath)
	}
}

func TestFindChromePathUsesWindowsEnvironmentKeysCaseInsensitively(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows environment key casing is only relevant on Windows")
	}
	root := t.TempDir()
	chromePath := filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe")
	if err := os.MkdirAll(filepath.Dir(chromePath), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(chromePath, []byte("fake"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	found, err := browser.FindChromePath(map[string]string{"ProgramFiles": root})
	if err != nil {
		t.Fatalf("FindChromePath returned error: %v", err)
	}
	if found != chromePath {
		t.Fatalf("found = %q, want %q", found, chromePath)
	}
}

func TestBuildLaunchArgsUsesPrivateProfileAndChatGPTURL(t *testing.T) {
	args := browser.BuildLaunchArgs(browser.LaunchConfig{
		ProfileDir: "C:/profiles/oracle",
		ChatGPTURL: "https://chatgpt.com/",
	})
	joined := strings.Join(args, " ")

	if !strings.Contains(joined, "--user-data-dir=C:/profiles/oracle") {
		t.Fatalf("args = %v, want user-data-dir", args)
	}
	if !strings.Contains(joined, "--remote-debugging-port=0") {
		t.Fatalf("args = %v, want remote debugging port", args)
	}
	if args[len(args)-1] != "https://chatgpt.com/" {
		t.Fatalf("last arg = %q, want ChatGPT URL", args[len(args)-1])
	}
}

func TestReadDevToolsEndpointFromProfile(t *testing.T) {
	profileDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(profileDir, "DevToolsActivePort"), []byte("9222\n/devtools/browser/abc\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	endpoint, err := browser.ReadDevToolsEndpoint(profileDir)
	if err != nil {
		t.Fatalf("ReadDevToolsEndpoint returned error: %v", err)
	}
	if endpoint != "ws://127.0.0.1:9222/devtools/browser/abc" {
		t.Fatalf("endpoint = %q, want websocket endpoint", endpoint)
	}
}

func TestWaitDevToolsEndpointWaitsForFile(t *testing.T) {
	profileDir := t.TempDir()
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(profileDir, "DevToolsActivePort"), []byte("9223\n/devtools/browser/waited\n"), 0o644)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	endpoint, err := browser.WaitDevToolsEndpoint(ctx, profileDir, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("WaitDevToolsEndpoint returned error: %v", err)
	}
	if endpoint != "ws://127.0.0.1:9223/devtools/browser/waited" {
		t.Fatalf("endpoint = %q, want waited endpoint", endpoint)
	}
}

func TestLaunchChromeAndWaitStartsProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process launch in short mode")
	}
	profileDir := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	fakeChrome := filepath.Join(t.TempDir(), "fake-chrome.cmd")
	script := fmt.Sprintf(`@echo off
set profile=%%ORACLE_BROWSER_PROFILE_DIR%%
if "%%profile%%"=="" exit /b 2
if not exist "%%profile%%" mkdir "%%profile%%"
> "%%profile%%\DevToolsActivePort" echo %d
>> "%%profile%%\DevToolsActivePort" echo /devtools/browser/fake
ping -n 5 127.0.0.1 > nul
`, port)
	if err := os.WriteFile(fakeChrome, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	process, endpoint, err := browser.LaunchChromeAndWait(ctx, fakeChrome, browser.LaunchConfig{
		ProfileDir: profileDir,
		ChatGPTURL: "about:blank",
	}, 5*time.Millisecond)
	if err != nil {
		t.Fatalf("LaunchChromeAndWait returned error: %v", err)
	}
	defer process.Kill()

	wantEndpoint := fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser/fake", port)
	if endpoint != wantEndpoint {
		t.Fatalf("endpoint = %q, want fake endpoint", endpoint)
	}
	if process.PID() <= 0 {
		t.Fatalf("PID = %d, want positive PID", process.PID())
	}
}

func TestLaunchChromeAndWaitRejectsClosedDevToolsPort(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process launch in short mode")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener: %v", err)
	}

	profileDir := t.TempDir()
	fakeChrome := filepath.Join(t.TempDir(), "fake-chrome.cmd")
	script := fmt.Sprintf(`@echo off
set profile=%%ORACLE_BROWSER_PROFILE_DIR%%
if "%%profile%%"=="" exit /b 2
if not exist "%%profile%%" mkdir "%%profile%%"
> "%%profile%%\DevToolsActivePort" echo %d
>> "%%profile%%\DevToolsActivePort" echo /devtools/browser/closed
ping -n 5 127.0.0.1 > nul
`, port)
	if err := os.WriteFile(fakeChrome, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	process, endpoint, err := browser.LaunchChromeAndWait(ctx, fakeChrome, browser.LaunchConfig{
		ProfileDir: profileDir,
		ChatGPTURL: "about:blank",
	}, 10*time.Millisecond)
	if err == nil {
		_ = process.Kill()
		t.Fatalf("LaunchChromeAndWait returned endpoint %q, want closed port error", endpoint)
	}
	if !strings.Contains(err.Error(), "DevTools") {
		t.Fatalf("error = %v, want DevTools readiness error", err)
	}
}

func TestLaunchChromeAndWaitDoesNotTieProcessLifetimeToWaitContext(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process launch in short mode")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	profileDir := t.TempDir()
	markerPath := filepath.Join(t.TempDir(), "still-running.txt")
	fakeChrome := filepath.Join(t.TempDir(), "fake-chrome.cmd")
	script := fmt.Sprintf(`@echo off
set profile=%%ORACLE_BROWSER_PROFILE_DIR%%
if "%%profile%%"=="" exit /b 2
if not exist "%%profile%%" mkdir "%%profile%%"
> "%%profile%%\DevToolsActivePort" echo %d
>> "%%profile%%\DevToolsActivePort" echo /devtools/browser/lifetime
ping -n 2 127.0.0.1 > nul
> "%s" echo alive
ping -n 5 127.0.0.1 > nul
`, port, markerPath)
	if err := os.WriteFile(fakeChrome, []byte(script), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	process, endpoint, err := browser.LaunchChromeAndWait(ctx, fakeChrome, browser.LaunchConfig{
		ProfileDir:  profileDir,
		ChatGPTURL:  "about:blank",
		KeepBrowser: false,
	}, 5*time.Millisecond)
	if err != nil {
		cancel()
		t.Fatalf("LaunchChromeAndWait returned error: %v", err)
	}
	defer process.Kill()
	if endpoint == "" {
		cancel()
		t.Fatal("endpoint is empty")
	}

	cancel()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(markerPath); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("launched process did not survive wait context cancellation long enough to write marker")
}
