package claude

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"codecli-channels/internal/codex"
)

type Runner struct {
	Binary string
}

func NewRunner(binary string) *Runner {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		binary = "claude"
	}
	return &Runner{Binary: binary}
}

func (r *Runner) RunTurn(ctx context.Context, opts codex.TurnOptions) (codex.TurnResult, error) {
	var result codex.TurnResult
	prompt := buildPrompt(opts)
	if prompt == "" {
		return result, fmt.Errorf("empty prompt")
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	configDir, err := prepareScopedClaudeConfigDir(opts.ProjectPath, opts.SessionID)
	if err != nil {
		return result, err
	}

	mode := strings.ToLower(strings.TrimSpace(opts.SandboxMode))
	allowedTools := allowedToolsForMode(mode)
	bridgeSettingsPath, err := writeBridgeSettings(configDir, mode)
	if err != nil {
		return result, err
	}

	args := []string{
		"-p", prompt,
		"--output-format", "json",
		"--permission-mode", "dontAsk",
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if strings.TrimSpace(opts.ThreadID) != "" {
		args = append(args, "--resume", opts.ThreadID)
	}
	if allowedTools != "" {
		args = append(args, "--allowedTools", allowedTools)
	}
	if bridgeSettingsPath != "" {
		args = append(args, "--settings", bridgeSettingsPath)
	}

	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Dir = opts.ProjectPath
	cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+configDir)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()

	result.CombinedText = strings.TrimSpace(stdout.String() + "\n" + stderr.String())
	result.ExitCode = exitCodeForErr(err)

	var payload headlessPayload
	if parseErr := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &payload); parseErr != nil {
		if err != nil {
			return result, fmt.Errorf("claude failed: %w", err)
		}
		return result, fmt.Errorf("parse claude json output: %w", parseErr)
	}
	if strings.TrimSpace(payload.SessionID) == "" {
		// Keep the existing thread id if present; it may still be valid even if
		// Claude omitted it from the response for some reason.
		payload.SessionID = strings.TrimSpace(opts.ThreadID)
	}
	result.ThreadID = payload.SessionID
	result.ResponseText = strings.TrimSpace(payload.Result)
	if err != nil {
		return result, fmt.Errorf("claude failed: %w", err)
	}
	return result, nil
}

type headlessPayload struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
}

func buildPrompt(opts codex.TurnOptions) string {
	prompt := strings.TrimSpace(opts.Prompt)
	if opts.MaxPromptChars > 0 && len([]rune(prompt)) > opts.MaxPromptChars {
		prompt = string([]rune(prompt)[:opts.MaxPromptChars])
	}
	return prompt
}

func allowedToolsForMode(mode string) string {
	if mode == "read" {
		return "Read,Glob,Grep"
	}
	return "Bash,Read,Edit,Glob,Grep"
}

func writeBridgeSettings(configDir, mode string) (string, error) {
	if mode != "write" {
		return "", nil
	}
	// Keep this intentionally small and conservative. The bridge already has a
	// separate /confirm gate for risky user requests; these are last-resort
	// hard blocks to avoid the most common destructive commands.
	payload := map[string]any{
		"permissions": map[string]any{
			"deny": []string{
				"Bash(rm -rf *)",
				"Bash(sudo *)",
				"Bash(git reset --hard*)",
				"Bash(git clean -fd*)",
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	path := filepath.Join(configDir, "settings.bridge.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func prepareScopedClaudeConfigDir(projectPath, scope string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sourceDir := os.Getenv("CLAUDE_CONFIG_DIR")
	if strings.TrimSpace(sourceDir) == "" {
		sourceDir = filepath.Join(homeDir, ".claude")
	}
	hashInput := projectPath
	if strings.TrimSpace(scope) != "" {
		hashInput = projectPath + "\n" + scope
	}
	hash := sha1.Sum([]byte(hashInput))
	runtimeDir := filepath.Join(os.TempDir(), "codecli-channels-claude-config", hex.EncodeToString(hash[:8]))
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		return "", err
	}

	for _, name := range []string{"settings.json", "settings.local.json", "CLAUDE.md"} {
		if err := copyFileIfExists(filepath.Join(sourceDir, name), filepath.Join(runtimeDir, name)); err != nil {
			return "", err
		}
	}
	// Some installations keep a nested ".claude/settings.local.json". Copy it if present.
	if err := copyFileIfExists(filepath.Join(sourceDir, ".claude", "settings.local.json"), filepath.Join(runtimeDir, ".claude", "settings.local.json")); err != nil {
		return "", err
	}
	for _, name := range []string{"rules", "skills", "plugins"} {
		if err := symlinkDirIfPossible(filepath.Join(sourceDir, name), filepath.Join(runtimeDir, name)); err != nil {
			return "", err
		}
	}
	for _, name := range []string{"sessions", "projects", "cache", "debug", "file-history", "session-env", "shell-snapshots", "tasks", "todos", "telemetry", "backups"} {
		if err := os.MkdirAll(filepath.Join(runtimeDir, name), 0o755); err != nil {
			return "", err
		}
	}
	return runtimeDir, nil
}

func copyFileIfExists(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, info.Mode())
}

func symlinkDirIfPossible(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !info.IsDir() {
		return nil
	}
	if current, err := os.Lstat(dst); err == nil {
		if current.Mode()&os.ModeSymlink != 0 {
			if target, err := os.Readlink(dst); err == nil && target == src {
				return nil
			}
		}
		if current.IsDir() || current.Mode()&os.ModeSymlink != 0 {
			_ = os.RemoveAll(dst)
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.Symlink(src, dst); err != nil {
		if os.IsExist(err) {
			return nil
		}
		return err
	}
	return nil
}

func exitCodeForErr(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr != nil {
		return exitErr.ExitCode()
	}
	return -1
}
