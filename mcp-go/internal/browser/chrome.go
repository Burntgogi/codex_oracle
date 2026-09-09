package browser

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type LaunchConfig struct {
	ProfileDir  string
	ChatGPTURL  string
	Headless    bool
	KeepBrowser bool
}

type ChromeProcess struct {
	cmd         *exec.Cmd
	keepBrowser bool
}

func FindChromePath(env map[string]string) (string, error) {
	if override := strings.TrimSpace(envValue(env, "ORACLE_BROWSER_CHROME_PATH")); override != "" {
		if fileExists(override) {
			return override, nil
		}
		return "", fmt.Errorf("ORACLE_BROWSER_CHROME_PATH does not exist: %s", override)
	}
	for _, candidate := range chromeCandidates(env) {
		if fileExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Chrome executable not found; set ORACLE_BROWSER_CHROME_PATH")
}

func BuildLaunchArgs(config LaunchConfig) []string {
	chatgptURL := strings.TrimSpace(config.ChatGPTURL)
	if chatgptURL == "" {
		chatgptURL = "https://chatgpt.com/"
	}
	args := []string{
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
	}
	if config.ProfileDir != "" {
		args = append(args, "--user-data-dir="+filepath.ToSlash(config.ProfileDir))
	}
	if config.Headless {
		args = append(args, "--headless=new")
	}
	args = append(args, chatgptURL)
	return args
}

func ReadDevToolsEndpoint(profileDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(profileDir, "DevToolsActivePort"))
	if err != nil {
		return "", err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimSpace(lines[1]) == "" {
		return "", fmt.Errorf("DevToolsActivePort is incomplete")
	}
	port := strings.TrimSpace(lines[0])
	path := strings.TrimSpace(lines[1])
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return "ws://127.0.0.1:" + port + path, nil
}

func WaitDevToolsEndpoint(ctx context.Context, profileDir string, interval time.Duration) (string, error) {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		endpoint, err := ReadDevToolsEndpoint(profileDir)
		if err == nil {
			return endpoint, nil
		}
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timed out waiting for DevToolsActivePort in %s: %w", profileDir, ctx.Err())
		case <-ticker.C:
		}
	}
}

func WaitReachableDevToolsEndpoint(ctx context.Context, profileDir string, interval time.Duration) (string, error) {
	if interval <= 0 {
		interval = 100 * time.Millisecond
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var lastErr error
	for {
		endpoint, err := ReadDevToolsEndpoint(profileDir)
		if err == nil {
			if err := endpointReachable(ctx, endpoint, interval); err == nil {
				return endpoint, nil
			} else {
				lastErr = err
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return "", fmt.Errorf("timed out waiting for reachable DevTools endpoint in %s: %w", profileDir, lastErr)
			}
			return "", fmt.Errorf("timed out waiting for reachable DevTools endpoint in %s: %w", profileDir, ctx.Err())
		case <-ticker.C:
		}
	}
}

func endpointReachable(ctx context.Context, rawURL string, timeout time.Duration) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if parsed.Scheme != "ws" {
		return fmt.Errorf("unsupported DevTools websocket scheme %q", parsed.Scheme)
	}
	if timeout <= 0 {
		timeout = 100 * time.Millisecond
	}
	if timeout > time.Second {
		timeout = time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", parsed.Host)
	if err != nil {
		return err
	}
	return conn.Close()
}

func LaunchChromeAndWait(ctx context.Context, chromePath string, config LaunchConfig, interval time.Duration) (*ChromeProcess, string, error) {
	if strings.TrimSpace(chromePath) == "" {
		return nil, "", fmt.Errorf("chrome path is required")
	}
	if strings.TrimSpace(config.ProfileDir) == "" {
		return nil, "", fmt.Errorf("profile dir is required")
	}
	if err := os.MkdirAll(config.ProfileDir, 0o755); err != nil {
		return nil, "", err
	}
	cmd := exec.Command(chromePath, BuildLaunchArgs(config)...)
	cmd.Env = append(os.Environ(), "ORACLE_BROWSER_PROFILE_DIR="+config.ProfileDir)
	cmd.Stdout = ioDiscard{}
	cmd.Stderr = ioDiscard{}
	if err := cmd.Start(); err != nil {
		return nil, "", err
	}
	process := &ChromeProcess{cmd: cmd, keepBrowser: config.KeepBrowser}
	endpoint, err := WaitReachableDevToolsEndpoint(ctx, config.ProfileDir, interval)
	if err != nil {
		_ = process.Kill()
		return nil, "", err
	}
	return process, endpoint, nil
}

func (p *ChromeProcess) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

func (p *ChromeProcess) Kill() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil || p.keepBrowser {
		return nil
	}
	err := p.cmd.Process.Kill()
	_, _ = p.cmd.Process.Wait()
	return err
}

func chromeCandidates(env map[string]string) []string {
	if runtime.GOOS != "windows" {
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/usr/bin/google-chrome",
			"/usr/bin/google-chrome-stable",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
		}
	}
	candidates := []string{}
	for _, rootKey := range []string{"LOCALAPPDATA", "PROGRAMFILES", "PROGRAMFILES(X86)"} {
		if root := strings.TrimSpace(envValue(env, rootKey)); root != "" {
			candidates = append(candidates,
				filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
				filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
			)
		}
	}
	return candidates
}

func envValue(env map[string]string, key string) string {
	if env == nil {
		return ""
	}
	if value, ok := env[key]; ok {
		return value
	}
	for candidateKey, value := range env {
		if strings.EqualFold(candidateKey, key) {
			return value
		}
	}
	return ""
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}
