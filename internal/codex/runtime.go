package codex

import (
	"crypto/sha1"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

func buildCleanCodexEnv(codexHome string) []string {
	keys := []string{
		"HOME", "PATH", "SHELL", "USER", "LOGNAME", "LANG", "LC_ALL", "LC_CTYPE", "TMPDIR",
		"SSH_AUTH_SOCK", "TERM_PROGRAM", "TERM_PROGRAM_VERSION",
		"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy",
		"OPENAI_API_KEY", "OPENAI_BASE_URL",
	}
	env := []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"CODEX_DISABLE_TELEMETRY=1",
		"CODEX_HOME=" + codexHome,
	}
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok && strings.TrimSpace(value) != "" {
			if key == "TERM" || key == "COLORTERM" || key == "CODEX_DISABLE_TELEMETRY" {
				continue
			}
			env = append(env, key+"="+value)
		}
	}
	return env
}

func prepareScopedCodexRuntimeHome(projectPath, scope string) (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	sourceHome := os.Getenv("CODEX_HOME")
	if strings.TrimSpace(sourceHome) == "" {
		sourceHome = filepath.Join(homeDir, ".codex")
	}
	hashInput := projectPath
	if strings.TrimSpace(scope) != "" {
		hashInput = projectPath + "\n" + scope
	}
	hash := sha1.Sum([]byte(hashInput))
	runtimeHome := filepath.Join(os.TempDir(), "codecli-channels-codex-home", hex.EncodeToString(hash[:8]))
	if err := os.MkdirAll(runtimeHome, 0o755); err != nil {
		return "", err
	}
	files := []string{"config.toml", "auth.json", ".codex-global-state.json", "models_cache.json", "version.json", "state_5.sqlite", "state_5.sqlite-shm", "state_5.sqlite-wal"}
	for _, name := range files {
		if err := copyFileIfExists(filepath.Join(sourceHome, name), filepath.Join(runtimeHome, name)); err != nil {
			return "", err
		}
	}
	for _, name := range []string{"prompts", "rules", "policy", "skills", "vendor_imports"} {
		if err := symlinkDirIfPossible(filepath.Join(sourceHome, name), filepath.Join(runtimeHome, name)); err != nil {
			return "", err
		}
	}
	for _, name := range []string{"sessions", "archived_sessions", "log", "shell_snapshots"} {
		if err := os.MkdirAll(filepath.Join(runtimeHome, name), 0o755); err != nil {
			return "", err
		}
	}
	return runtimeHome, nil
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
