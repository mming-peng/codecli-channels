package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LoadRaw loads config JSON without any normalization.
//
// This is used by editable config flows where we must preserve the user's raw
// JSON values (for example relative paths like "./data").
func LoadRaw(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	// Populate alias fields (they are not part of JSON) and ensure maps are non-nil
	// for safe editing.
	if cfg.Channels == nil {
		cfg.Channels = map[string]ChannelConfig{}
	}
	for alias, ch := range cfg.Channels {
		ch.Alias = alias
		if ch.Options == nil {
			ch.Options = map[string]any{}
		}
		cfg.Channels[alias] = ch
	}
	if cfg.Bridge.Projects == nil {
		cfg.Bridge.Projects = map[string]ProjectConfig{}
	}
	for alias, project := range cfg.Bridge.Projects {
		project.Alias = alias
		cfg.Bridge.Projects[alias] = project
	}

	return &cfg, nil
}

// SaveRaw saves config JSON as-is (no normalization, no path rewriting).
func SaveRaw(path string, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("cfg 不能为空")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

type ChannelPatch struct {
	// Type is required when creating a new channel.
	Type string
	// Enabled is optional; when nil it leaves current value unchanged.
	Enabled *bool
	// Options overwrites keys into the channel's options.
	Options map[string]any
	// SetOptionsIfEmpty sets options only when the key doesn't exist (or is nil).
	SetOptionsIfEmpty map[string]any
}

// UpsertChannel creates or updates a channel config by alias.
func (c *Config) UpsertChannel(alias string, patch ChannelPatch) (ChannelConfig, error) {
	if c == nil {
		return ChannelConfig{}, fmt.Errorf("config 不能为空")
	}
	c.normalizeLegacyChannels()
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ChannelConfig{}, fmt.Errorf("channel alias 不能为空")
	}
	if c.Channels == nil {
		c.Channels = map[string]ChannelConfig{}
	}

	normalizeType := func(value string) string {
		return strings.ToLower(strings.TrimSpace(value))
	}

	ch, exists := c.Channels[alias]
	ch.Alias = alias
	if ch.Options == nil {
		ch.Options = map[string]any{}
	}

	if exists {
		if strings.TrimSpace(patch.Type) != "" {
			wantType := normalizeType(patch.Type)
			gotType := normalizeType(ch.Type)
			if gotType != "" && gotType != wantType {
				return ChannelConfig{}, fmt.Errorf("channel %s type 冲突：当前=%s 目标=%s", alias, gotType, wantType)
			}
			ch.Type = wantType
		}
		if patch.Enabled != nil {
			ch.Enabled = *patch.Enabled
		}
		for k, v := range patch.Options {
			ch.Options[k] = v
		}
		for k, v := range patch.SetOptionsIfEmpty {
			if cur, ok := ch.Options[k]; !ok || cur == nil {
				ch.Options[k] = v
			}
		}
		c.Channels[alias] = ch
		return ch, nil
	}

	typ := normalizeType(patch.Type)
	if typ == "" {
		return ChannelConfig{}, fmt.Errorf("channel %s 缺少 type", alias)
	}
	ch.Type = typ
	if patch.Enabled != nil {
		ch.Enabled = *patch.Enabled
	}
	for k, v := range patch.Options {
		ch.Options[k] = v
	}
	for k, v := range patch.SetOptionsIfEmpty {
		if cur, ok := ch.Options[k]; !ok || cur == nil {
			ch.Options[k] = v
		}
	}
	c.Channels[alias] = ch
	return ch, nil
}

func (c *Config) EnsureBridgeChannelID(channelID string) {
	if c == nil {
		return
	}
	c.seedBridgeChannelIDsFromLegacy()
	channelID = strings.TrimSpace(channelID)
	if channelID == "" {
		return
	}
	for _, id := range c.Bridge.ChannelIDs {
		if id == channelID {
			return
		}
	}
	c.Bridge.ChannelIDs = append(c.Bridge.ChannelIDs, channelID)
}

func (c *Config) EnsureAllowedScope(scope string) {
	if c == nil {
		return
	}
	c.seedAllowedScopesFromLegacy()
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return
	}
	for _, s := range c.Bridge.AllowedScopes {
		if s == scope {
			return
		}
	}
	c.Bridge.AllowedScopes = append(c.Bridge.AllowedScopes, scope)
}

func (c *Config) seedBridgeChannelIDsFromLegacy() {
	if c == nil || len(c.Bridge.ChannelIDs) > 0 || len(c.Bridge.AccountIDs) == 0 {
		return
	}
	for _, id := range c.Bridge.AccountIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		c.Bridge.ChannelIDs = append(c.Bridge.ChannelIDs, id)
	}
}

func (c *Config) seedAllowedScopesFromLegacy() {
	if c == nil || len(c.Bridge.AllowedScopes) > 0 || len(c.Bridge.AllowedTargets) == 0 {
		return
	}
	for _, scope := range c.Bridge.AllowedTargets {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			continue
		}
		c.Bridge.AllowedScopes = append(c.Bridge.AllowedScopes, scope)
	}
}
