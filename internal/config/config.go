package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultAccountID      = "default"
	DefaultTimezone       = "Asia/Shanghai"
	DefaultReadSandbox    = "read-only"
	DefaultWriteSandbox   = "workspace-write"
	DangerSandbox         = "danger-full-access"
	DefaultQQMaxReply     = 1500
	DefaultMaxPromptChars = 4000
	DefaultTimeoutMs      = 600000
	DefaultConfirmTTLMS   = 600000
)

type Config struct {
	DefaultAccountID string             `json:"defaultAccountId"`
	DefaultTimezone  string             `json:"defaultTimezone"`
	Accounts         map[string]Account `json:"accounts"`
	Bridge           BridgeConfig       `json:"bridge"`
}

type Account struct {
	Enabled      bool   `json:"enabled"`
	AppID        string `json:"appId"`
	ClientSecret string `json:"clientSecret"`
}

type BridgeConfig struct {
	Enabled              bool                     `json:"enabled"`
	Backend              string                   `json:"backend"`
	AccountIDs           []string                 `json:"accountIds"`
	AllowAllTargets      bool                     `json:"allowAllTargets"`
	AllowedTargets       []string                 `json:"allowedTargets"`
	RequireCommandPrefix bool                     `json:"requireCommandPrefix"`
	ReadOnlyPrefixes     []string                 `json:"readOnlyPrefixes"`
	WritePrefixes        []string                 `json:"writePrefixes"`
	ConfirmPrefixes      []string                 `json:"confirmPrefixes"`
	Projects             map[string]ProjectConfig `json:"projects"`
	DefaultProject       string                   `json:"defaultProject"`
	CodexTimeoutMs       int                      `json:"codexTimeoutMs"`
	QQMaxReplyChars      int                      `json:"qqMaxReplyChars"`
	MaxPromptChars       int                      `json:"maxPromptChars"`
	ReadOnlyCodexSandbox string                   `json:"readOnlyCodexSandbox"`
	WriteCodexSandbox    string                   `json:"writeCodexSandbox"`
	CodexModel           string                   `json:"codexModel"`
	ClaudeBinary         string                   `json:"claudeBinary"`
	ClaudeModel          string                   `json:"claudeModel"`
	ConfirmationTTLMS    int                      `json:"confirmationTtlMs"`
	AuditEnabled         bool                     `json:"auditEnabled"`
	DefaultRunMode       string                   `json:"defaultRunMode"`
	ImplicitMessageMode  string                   `json:"implicitMessageMode"`
	DataDir              string                   `json:"dataDir"`
	StateFile            string                   `json:"stateFile"`
	AuditFile            string                   `json:"auditFile"`
}

type ProjectConfig struct {
	Alias       string `json:"-"`
	Path        string `json:"path"`
	Description string `json:"description"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if err := cfg.Normalize(filepath.Dir(path)); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Normalize(baseDir string) error {
	if c.DefaultAccountID == "" {
		c.DefaultAccountID = DefaultAccountID
	}
	if c.DefaultTimezone == "" {
		c.DefaultTimezone = DefaultTimezone
	}
	if len(c.Accounts) == 0 {
		return fmt.Errorf("accounts 不能为空")
	}
	if strings.TrimSpace(c.Bridge.Backend) == "" {
		c.Bridge.Backend = "codex"
	}
	c.Bridge.Backend = strings.ToLower(strings.TrimSpace(c.Bridge.Backend))
	switch c.Bridge.Backend {
	case "codex", "claude":
	default:
		return fmt.Errorf("bridge.backend=%s 不支持（仅支持 codex/claude）", c.Bridge.Backend)
	}
	if c.Bridge.QQMaxReplyChars <= 0 {
		c.Bridge.QQMaxReplyChars = DefaultQQMaxReply
	}
	if c.Bridge.MaxPromptChars <= 0 {
		c.Bridge.MaxPromptChars = DefaultMaxPromptChars
	}
	if c.Bridge.CodexTimeoutMs <= 0 {
		c.Bridge.CodexTimeoutMs = DefaultTimeoutMs
	}
	if c.Bridge.ConfirmationTTLMS <= 0 {
		c.Bridge.ConfirmationTTLMS = DefaultConfirmTTLMS
	}
	if c.Bridge.ReadOnlyCodexSandbox == "" {
		c.Bridge.ReadOnlyCodexSandbox = DefaultReadSandbox
	}
	if c.Bridge.WriteCodexSandbox == "" {
		c.Bridge.WriteCodexSandbox = DefaultWriteSandbox
	}
	if strings.TrimSpace(c.Bridge.ClaudeBinary) == "" {
		c.Bridge.ClaudeBinary = "claude"
	}
	if c.Bridge.DefaultRunMode == "" {
		c.Bridge.DefaultRunMode = "write"
	}
	if c.Bridge.ImplicitMessageMode == "" {
		c.Bridge.ImplicitMessageMode = "write"
	}
	if c.Bridge.DataDir == "" {
		c.Bridge.DataDir = filepath.Join(baseDir, "..", "data")
	}
	c.Bridge.DataDir = absPath(baseDir, c.Bridge.DataDir)
	if c.Bridge.StateFile == "" {
		c.Bridge.StateFile = filepath.Join(c.Bridge.DataDir, "state.json")
	}
	if c.Bridge.AuditFile == "" {
		c.Bridge.AuditFile = filepath.Join(c.Bridge.DataDir, "bridge-audit.jsonl")
	}
	c.Bridge.StateFile = absPath(baseDir, c.Bridge.StateFile)
	c.Bridge.AuditFile = absPath(baseDir, c.Bridge.AuditFile)
	if len(c.Bridge.AccountIDs) == 0 {
		c.Bridge.AccountIDs = []string{c.DefaultAccountID}
	}
	for alias, project := range c.Bridge.Projects {
		project.Alias = alias
		project.Path = absPath(baseDir, project.Path)
		c.Bridge.Projects[alias] = project
	}
	if c.Bridge.DefaultProject == "" {
		for alias := range c.Bridge.Projects {
			c.Bridge.DefaultProject = alias
			break
		}
	}
	if c.Bridge.DefaultProject == "" {
		return fmt.Errorf("bridge.projects 不能为空")
	}
	if _, ok := c.Bridge.Projects[c.Bridge.DefaultProject]; !ok {
		return fmt.Errorf("defaultProject=%s 不存在", c.Bridge.DefaultProject)
	}
	if err := os.MkdirAll(c.Bridge.DataDir, 0o755); err != nil {
		return err
	}
	return nil
}

func absPath(baseDir, value string) string {
	if value == "" {
		return value
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	return filepath.Clean(filepath.Join(baseDir, value))
}

func (c *Config) ResolveAccount(accountID string) (Account, error) {
	if accountID == "" {
		accountID = c.DefaultAccountID
	}
	account, ok := c.Accounts[accountID]
	if !ok {
		return Account{}, fmt.Errorf("未找到账号 %s", accountID)
	}
	if strings.TrimSpace(account.AppID) == "" || strings.TrimSpace(account.ClientSecret) == "" {
		return Account{}, fmt.Errorf("账号 %s 缺少 appId/clientSecret", accountID)
	}
	return account, nil
}

func (c *Config) ProjectList() []ProjectConfig {
	items := make([]ProjectConfig, 0, len(c.Bridge.Projects))
	for _, project := range c.Bridge.Projects {
		items = append(items, project)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Alias < items[j].Alias })
	return items
}

func (c *Config) Project(alias string) (ProjectConfig, bool) {
	project, ok := c.Bridge.Projects[alias]
	return project, ok
}
