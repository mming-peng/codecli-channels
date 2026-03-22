package bridge

import (
	"fmt"
	"strings"

	cfgpkg "codecli-channels/internal/config"
)

var (
	defaultReadOnlyPrefixes = []string{"/ask", "/read", "问问"}
	defaultWritePrefixes    = []string{"/run", "/exec", "执行"}
	defaultConfirmPrefixes  = []string{"/confirm", "/确认"}
)

type ParsedCommand struct {
	Type             string
	Mode             string
	Body             string
	Prefix           string
	ReadOnlyPrefixes []string
	WritePrefixes    []string
	ConfirmPrefixes  []string
}

type DangerousMatch struct {
	Matched bool
	Reason  string
}

type dangerRule struct {
	Needle string
	Reason string
}

var dangerRules = []dangerRule{
	{Needle: "rm -rf", Reason: "检测到 rm -rf 风险指令"},
	{Needle: "git reset --hard", Reason: "检测到 git reset --hard 风险指令"},
	{Needle: "drop database", Reason: "检测到删除数据库风险指令"},
	{Needle: "truncate table", Reason: "检测到清空数据表风险指令"},
	{Needle: "sudo ", Reason: "检测到 sudo 提权操作"},
	{Needle: "清空仓库", Reason: "检测到大范围清理风险描述"},
	{Needle: "清空目录", Reason: "检测到大范围清理风险描述"},
}

func normalizePrefixes(values, fallbacks []string) []string {
	source := values
	if len(source) == 0 {
		source = fallbacks
	}
	out := make([]string, 0, len(source))
	for _, item := range source {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func matchPrefix(text string, prefixes []string) (string, string, bool) {
	for _, prefix := range prefixes {
		if text == prefix {
			return prefix, "", true
		}
		if strings.HasPrefix(text, prefix+" ") {
			return prefix, strings.TrimSpace(text[len(prefix):]), true
		}
	}
	return "", "", false
}

func ParseBridgeCommand(text string, cfg cfgpkg.BridgeConfig) ParsedCommand {
	normalized := strings.TrimSpace(text)
	readPrefixes := normalizePrefixes(cfg.ReadOnlyPrefixes, defaultReadOnlyPrefixes)
	writePrefixes := normalizePrefixes(cfg.WritePrefixes, defaultWritePrefixes)
	confirmPrefixes := normalizePrefixes(cfg.ConfirmPrefixes, defaultConfirmPrefixes)

	if !cfg.RequireCommandPrefix {
		return ParsedCommand{Type: "execute", Mode: "write", Body: normalized, ReadOnlyPrefixes: readPrefixes, WritePrefixes: writePrefixes, ConfirmPrefixes: confirmPrefixes}
	}
	if prefix, body, ok := matchPrefix(normalized, confirmPrefixes); ok {
		return ParsedCommand{Type: "confirm", Mode: "write", Body: body, Prefix: prefix, ReadOnlyPrefixes: readPrefixes, WritePrefixes: writePrefixes, ConfirmPrefixes: confirmPrefixes}
	}
	if prefix, body, ok := matchPrefix(normalized, readPrefixes); ok {
		return ParsedCommand{Type: "execute", Mode: "read", Body: body, Prefix: prefix, ReadOnlyPrefixes: readPrefixes, WritePrefixes: writePrefixes, ConfirmPrefixes: confirmPrefixes}
	}
	if prefix, body, ok := matchPrefix(normalized, writePrefixes); ok {
		return ParsedCommand{Type: "execute", Mode: "write", Body: body, Prefix: prefix, ReadOnlyPrefixes: readPrefixes, WritePrefixes: writePrefixes, ConfirmPrefixes: confirmPrefixes}
	}
	return ParsedCommand{Type: "unmatched", Body: normalized, ReadOnlyPrefixes: readPrefixes, WritePrefixes: writePrefixes, ConfirmPrefixes: confirmPrefixes}
}

func DetectDangerousTask(text string) DangerousMatch {
	normalized := strings.ToLower(strings.TrimSpace(text))
	for _, rule := range dangerRules {
		if strings.Contains(normalized, strings.ToLower(rule.Needle)) {
			return DangerousMatch{Matched: true, Reason: rule.Reason}
		}
	}
	return DangerousMatch{}
}

func BuildHelpText(cfg cfgpkg.BridgeConfig) string {
	readPrefixes := normalizePrefixes(cfg.ReadOnlyPrefixes, defaultReadOnlyPrefixes)
	writePrefixes := normalizePrefixes(cfg.WritePrefixes, defaultWritePrefixes)
	confirmPrefixes := normalizePrefixes(cfg.ConfirmPrefixes, defaultConfirmPrefixes)
	defaultText := "普通消息 - 直接发给当前后端（默认按当前会话模式执行）"
	if cfg.ImplicitMessageMode == "read" {
		defaultText = "普通消息 - 直接发给当前后端（默认只读分析）"
	}
	return strings.Join([]string{
		"常用操作：",
		"/status - 看当前项目、会话、模式、审批和运行状态",
		fmt.Sprintf("%s 你的问题 - 只读分析，不改文件", readPrefixes[0]),
		fmt.Sprintf("%s 你的需求 - 在当前项目执行后端任务", writePrefixes[0]),
		"/stop - 停止当前正在执行的任务",
		"/history - 回看当前项目最近任务",
		"",
		"详细命令：",
		defaultText,
		"/ping - 健康检查",
		"/help - 查看帮助",
		"/backend current - 查看当前后端（codex/claude）",
		"/backend use <codex|claude> - 切换后端",
		"/status - 查看当前状态总览",
		"/stop - 停止当前任务",
		"/history - 查看当前项目最近任务",
		"/project list - 查看项目列表",
		"/project current - 查看当前项目",
		"/project use <别名> - 切换项目",
		"/clear - 立即开启当前项目的新会话",
		"/session list - 查看当前项目下的本地会话",
		"/session current - 查看当前会话",
		"/session new [名称] - 新建会话",
		"/session switch <id> - 切换会话",
		"/mode - 查看当前默认执行模式",
		"/mode write|read - 设置普通消息和 /run 的默认模式",
		fmt.Sprintf("%s - 确认执行高风险写操作", confirmPrefixes[0]),
		"/approve [session] - 同意当前 Codex 原生审批（Codex 后端），可选本会话记忆",
		"/deny - 拒绝当前 Codex 原生审批（Codex 后端）",
	}, "\n")
}
