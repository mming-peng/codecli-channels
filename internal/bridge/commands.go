package bridge

import (
	"strings"

	cfgpkg "codecli-channels/internal/config"
)

type DangerousMatch struct {
	Matched bool
	Reason  string
}

type dangerRule struct {
	Needle string
	Reason string
}

type removedCommand struct {
	Prefixes []string
	Reply    string
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

var removedCommands = []removedCommand{
	{
		Prefixes: []string{"/ask", "/read", "问问", "/run", "/exec", "执行"},
		Reply:    "这些命令已移除。直接发普通消息即可，我会把内容交给当前后端继续处理。",
	},
	{
		Prefixes: []string{"/ping", "/status", "/mode"},
		Reply:    "这个命令已移除。需要查看或切换环境时，请使用 /help 里的 /project、/session、/backend、/history、/stop。",
	},
	{
		Prefixes: []string{"/clear"},
		Reply:    "这个命令已移除。需要结束当前上下文时，请使用 `/session new [名称]`。",
	},
	{
		Prefixes: []string{"/confirm", "/确认"},
		Reply:    "`/confirm` 已移除。遇到待审批或高风险任务时，请直接回复 `/approve` 或 `/deny`。",
	},
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

func MatchRemovedCommand(text string) (string, string, bool) {
	normalized := strings.TrimSpace(text)
	for _, spec := range removedCommands {
		if prefix, _, ok := matchPrefix(normalized, spec.Prefixes); ok {
			return prefix, spec.Reply, true
		}
	}
	return "", "", false
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
	defaultText := "普通消息 - 直接发给当前后端，在当前项目和会话里继续工作"
	if cfg.ImplicitMessageMode == "read" {
		defaultText = "普通消息 - 直接发给当前后端，默认按只读方式分析"
	}
	return strings.Join([]string{
		"默认交互：",
		defaultText,
		"",
		"环境控制：",
		"/help - 查看帮助",
		"/history - 查看当前项目最近任务",
		"/stop - 停止当前正在执行的任务",
		"/project list - 查看项目列表",
		"/project current - 查看当前项目",
		"/project use <别名> - 切换项目",
		"/session list - 查看当前项目下的会话",
		"/session current - 查看当前会话",
		"/session new [名称] - 新建会话并切走当前上下文",
		"/session switch <id> - 切换会话",
		"/backend current - 查看当前后端（codex/claude）",
		"/backend list - 查看可用后端",
		"/backend use <codex|claude> - 切换后端",
		"/approve [session] - 同意当前待处理审批，可选本会话记忆",
		"/deny - 拒绝当前待处理审批",
	}, "\n")
}
