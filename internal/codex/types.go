package codex

import (
	"context"
	"strings"
	"time"
)

type TurnRunner interface {
	RunTurn(ctx context.Context, opts TurnOptions) (TurnResult, error)
}

type TurnOptions struct {
	Prompt          string
	SessionID       string
	ProjectAlias    string
	ProjectPath     string
	TargetType      string
	SenderID        string
	TargetID        string
	ThreadID        string
	SandboxMode     string
	Model           string
	MaxPromptChars  int
	Timeout         time.Duration
	ApprovalHandler func(ApprovalRequest) (ApprovalDecision, error)
	ProgressHandler func(ProgressEvent)
}

type TurnResult struct {
	ThreadID     string
	ResponseText string
	CombinedText string
	ExitCode     int
}

type ApprovalRequest struct {
	Title   string
	Reason  string
	Command string
	RawText string
}

type ProgressEvent struct {
	Text string
}

type ApprovalDecision int

const (
	ApprovalDeny ApprovalDecision = iota
	ApprovalAllow
	ApprovalAllowForSession
	ApprovalCancel
)

func buildPrompt(opts TurnOptions) string {
	prompt := strings.TrimSpace(opts.Prompt)
	if opts.MaxPromptChars > 0 && len([]rune(prompt)) > opts.MaxPromptChars {
		prompt = string([]rune(prompt)[:opts.MaxPromptChars])
	}
	return prompt
}
