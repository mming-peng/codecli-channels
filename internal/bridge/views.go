package bridge

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	cfgpkg "codecli-channels/internal/config"
	"codecli-channels/internal/store"
)

type runningTaskState struct {
	Cancel        context.CancelFunc
	StartedAt     time.Time
	SessionID     string
	SessionName   string
	ProjectAlias  string
	Backend       string
	Mode          string
	Summary       string
	ProgressText  string
	StopRequested bool
}

type statusView struct {
	ProjectAlias         string
	ProjectDescription   string
	Backend              string
	SessionID            string
	SessionName          string
	Mode                 string
	Busy                 bool
	ProgressText         string
	PendingConfirmReason string
	PendingApprovalTitle string
	LastTaskStatus       string
	LastTaskSummary      string
}

func (s *Service) setRunningTask(key string, state *runningTaskState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runningTasks[key] = state
}

func (s *Service) clearRunningTask(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.runningTasks, key)
}

func (s *Service) currentRunningTask(key string) *runningTaskState {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.runningTasks[key]
	if state == nil {
		return nil
	}
	copy := *state
	return &copy
}

func (s *Service) updateRunningTaskProgress(key, progress string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.runningTasks[key]
	if state == nil {
		return
	}
	state.ProgressText = strings.TrimSpace(progress)
}

func (s *Service) stopRunningTask(key string) (*runningTaskState, bool) {
	s.mu.Lock()
	state := s.runningTasks[key]
	if state == nil {
		s.mu.Unlock()
		return nil, false
	}
	state.StopRequested = true
	copy := *state
	cancel := state.Cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return &copy, true
}

func (s *Service) currentNativeApprovalTitle(key string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.nativeApprovals[key]
	if state == nil || time.Now().After(state.ExpiresAt) {
		return ""
	}
	if title := strings.TrimSpace(state.Request.Title); title != "" {
		return title
	}
	return strings.TrimSpace(state.Request.Reason)
}

func summarizeTaskText(text string) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= 48 {
		return text
	}
	return string(runes[:48]) + "…"
}

func formatDisplayTime(ts time.Time) string {
	if ts.IsZero() {
		return "-"
	}
	return ts.Format("2006-01-02 15:04")
}

func buildStatusText(view statusView) string {
	lines := []string{"当前状态"}
	projectLine := "项目：" + view.ProjectAlias
	if strings.TrimSpace(view.ProjectDescription) != "" {
		projectLine += " - " + view.ProjectDescription
	}
	lines = append(lines, projectLine)
	if strings.TrimSpace(view.Backend) != "" {
		lines = append(lines, "后端："+view.Backend)
	}
	if view.SessionID != "" || view.SessionName != "" {
		lines = append(lines, fmt.Sprintf("会话：%s | %s", fallbackText(view.SessionID, "-"), fallbackText(view.SessionName, "-")))
	}
	lines = append(lines, "默认模式："+fallbackText(view.Mode, "-"))
	lines = append(lines, fmt.Sprintf("运行中：%s", yesNo(view.Busy)))
	if strings.TrimSpace(view.ProgressText) != "" {
		lines = append(lines, "最近进度："+view.ProgressText)
	}
	lines = append(lines, "高风险确认："+fallbackText(view.PendingConfirmReason, "无"))
	lines = append(lines, "原生审批："+fallbackText(view.PendingApprovalTitle, "无"))
	if view.LastTaskStatus != "" || view.LastTaskSummary != "" {
		lines = append(lines, fmt.Sprintf("最近任务：%s | %s", fallbackText(view.LastTaskStatus, "-"), fallbackText(view.LastTaskSummary, "-")))
	} else {
		lines = append(lines, "最近任务：无")
	}
	return strings.Join(lines, "\n")
}

func buildProjectListText(projects []cfgpkg.ProjectConfig, currentAlias string) string {
	items := append([]cfgpkg.ProjectConfig(nil), projects...)
	sort.Slice(items, func(i, j int) bool { return items[i].Alias < items[j].Alias })
	lines := []string{"可用项目："}
	for _, project := range items {
		marker := "-"
		if currentAlias == project.Alias {
			marker = "*"
		}
		line := fmt.Sprintf("%s %s", marker, project.Alias)
		if strings.TrimSpace(project.Description) != "" {
			line += " - " + project.Description
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func buildSessionListText(projectAlias string, items []*store.SessionRecord, currentID string) string {
	if len(items) == 0 {
		return fmt.Sprintf("项目 %s 下还没有会话，先发一条普通消息或 /run 即可自动创建。", projectAlias)
	}
	lines := []string{fmt.Sprintf("项目 %s 的会话：", projectAlias)}
	for _, item := range items {
		marker := "-"
		if item != nil && item.ID == currentID {
			marker = "*"
		}
		if item == nil {
			continue
		}
		parts := []string{
			fmt.Sprintf("%s %s", marker, item.ID),
			fallbackText(item.Name, "-"),
			"mode:" + fallbackText(item.DefaultRunMode, "-"),
		}
		if item.LastTaskStatus != "" || item.LastTaskSummary != "" {
			parts = append(parts, "最近:"+fallbackText(item.LastTaskStatus, "-"))
			parts = append(parts, fallbackText(item.LastTaskSummary, "-"))
		}
		if !item.LastTaskAt.IsZero() {
			parts = append(parts, formatDisplayTime(item.LastTaskAt))
		}
		lines = append(lines, strings.Join(parts, " | "))
	}
	return strings.Join(lines, "\n")
}

func buildHistoryText(projectAlias string, events []AuditEvent) string {
	if len(events) == 0 {
		return fmt.Sprintf("项目 %s 暂无最近任务记录。", projectAlias)
	}
	items := append([]AuditEvent(nil), events...)
	sort.Slice(items, func(i, j int) bool { return items[i].Time.After(items[j].Time) })
	lines := []string{fmt.Sprintf("最近任务（项目：%s）：", projectAlias)}
	for _, event := range items {
		summary := summarizeTaskText(event.Text)
		if summary == "" {
			summary = summarizeTaskText(event.Reason)
		}
		lines = append(lines, fmt.Sprintf("- %s | %s | %s | %s | %s",
			formatDisplayTime(event.Time),
			fallbackText(event.SessionID, "-"),
			fallbackText(event.Mode, "-"),
			fallbackText(event.Status, "-"),
			fallbackText(summary, "-"),
		))
	}
	return strings.Join(lines, "\n")
}

func (s *Service) buildConversationStatus(_ context.Context, conversationKey string) string {
	if s == nil || s.cfg == nil || s.store == nil {
		return "当前状态不可用。"
	}
	project := s.resolveCurrentProject(conversationKey)
	session, err := s.store.GetOrCreateActiveSession(conversationKey, project.Alias, project.Path, s.cfg.Bridge.DefaultRunMode)
	if err != nil {
		return fmt.Sprintf("读取当前状态失败：%v", err)
	}
	view := statusView{
		ProjectAlias:       project.Alias,
		ProjectDescription: project.Description,
		Backend:            backendLabel(s.resolveBackend(conversationKey)),
	}
	if session != nil {
		view.SessionID = session.ID
		view.SessionName = session.Name
		view.Mode = session.DefaultRunMode
		view.LastTaskStatus = session.LastTaskStatus
		view.LastTaskSummary = session.LastTaskSummary
		if strings.TrimSpace(session.LastBackend) != "" {
			view.Backend = backendLabel(session.LastBackend)
		}
	}
	if running := s.currentRunningTask(conversationKey); running != nil {
		view.Busy = true
		if running.SessionID != "" {
			view.SessionID = running.SessionID
		}
		if running.SessionName != "" {
			view.SessionName = running.SessionName
		}
		if running.Mode != "" {
			view.Mode = running.Mode
		}
		if running.Backend != "" {
			view.Backend = backendLabel(running.Backend)
		}
		if running.ProgressText != "" {
			view.ProgressText = running.ProgressText
		}
	}
	if pending, ok := s.getPendingConfirmation(conversationKey); ok {
		view.PendingConfirmReason = pending.Reason
	}
	view.PendingApprovalTitle = s.currentNativeApprovalTitle(conversationKey)
	return buildStatusText(view)
}

func yesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
