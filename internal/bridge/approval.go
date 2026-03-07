package bridge

import "strings"

type ApprovalAnalysis struct {
	NeedsApproval bool
	Reason        string
}

var approvalMatchers = []struct {
	Reason   string
	Patterns []string
}{
	{Reason: "当前任务被沙箱拦截，需要升级到更高执行权限", Patterns: []string{"当前沙箱拒绝", "sandbox", "operation not permitted", "permission denied", "writable directories", "可写目录"}},
	{Reason: "当前任务可能需要网络或更宽松的系统权限", Patterns: []string{"getaddrinfo enotfound", "network error", "econnrefused", "enotfound"}},
	{Reason: "当前任务需要从受限模式升级后重试", Patterns: []string{"不能发起越权审批请求", "approval", "需要审批", "审批策略是"}},
}

func AnalyzePermissionRequirement(text string) *ApprovalAnalysis {
	source := strings.ToLower(strings.TrimSpace(text))
	if source == "" {
		return nil
	}
	for _, matcher := range approvalMatchers {
		for _, pattern := range matcher.Patterns {
			if strings.Contains(source, strings.ToLower(pattern)) {
				return &ApprovalAnalysis{NeedsApproval: true, Reason: matcher.Reason}
			}
		}
	}
	return nil
}
