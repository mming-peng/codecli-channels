package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/creack/pty"
)

type Runner struct {
	Binary string
}

type TurnOptions struct {
	Prompt          string
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
)

var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	ansiOSC = regexp.MustCompile(`\x1b\][^\x07]*(\x07|\x1b\\)`)
	ctrlSeq = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

func NewRunner() *Runner {
	return &Runner{Binary: "codex"}
}

func (r *Runner) RunTurn(ctx context.Context, opts TurnOptions) (TurnResult, error) {
	var result TurnResult
	prompt := buildPrompt(opts)
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	outputFile, err := os.CreateTemp("", "qq-codex-last-*.txt")
	if err != nil {
		return result, err
	}
	outputPath := outputFile.Name()
	_ = outputFile.Close()
	defer os.Remove(outputPath)

	args := []string{"--no-alt-screen"}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	if opts.SandboxMode != "" {
		args = append(args, "-s", opts.SandboxMode)
	}
	args = append(args,
		"exec",
		"--skip-git-repo-check",
		"-o", outputPath,
	)
	if opts.ThreadID != "" {
		args = append(args, "resume", opts.ThreadID, prompt)
	} else {
		args = append(args, "-C", opts.ProjectPath, prompt)
	}

	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Dir = opts.ProjectPath
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"CODEX_DISABLE_TELEMETRY=1",
	)

	startedAt := time.Now()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return result, err
	}
	defer func() { _ = ptmx.Close() }()

	var plainBuilder strings.Builder
	progress := newProgressAccumulator()
	approvalHandled := false
	approvalResolved := false
	decisionApplied := false

	readDone := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(ptmx)
		for {
			chunk := make([]byte, 4096)
			n, readErr := reader.Read(chunk)
			if n > 0 {
				text := string(chunk[:n])
				plain := sanitizeTTY(text)
				if strings.TrimSpace(plain) != "" {
					plainBuilder.WriteString(plain)
					plainBuilder.WriteByte('\n')
					if opts.ProgressHandler != nil {
						for _, flush := range progress.AddPlain(plain) {
							opts.ProgressHandler(ProgressEvent{Text: flush})
						}
					}
				}
				if !approvalHandled {
					recent := tailString(plainBuilder.String(), 6000)
					if req, ok := detectApprovalRequest(recent); ok {
						approvalHandled = true
						if pending := progress.Flush(); pending != "" && opts.ProgressHandler != nil {
							opts.ProgressHandler(ProgressEvent{Text: pending})
						}
						if opts.ApprovalHandler != nil {
							decision, handlerErr := opts.ApprovalHandler(req)
							approvalResolved = true
							if handlerErr == nil {
								switch decision {
								case ApprovalAllow:
									_, _ = ptmx.Write([]byte("\r"))
									decisionApplied = true
								default:
									_, _ = ptmx.Write([]byte("\x1b"))
									decisionApplied = true
								}
							} else {
								_, _ = ptmx.Write([]byte("\x1b"))
								decisionApplied = true
								plainBuilder.WriteString("[approval handler error] ")
								plainBuilder.WriteString(handlerErr.Error())
								plainBuilder.WriteByte('\n')
							}
						} else {
							_, _ = ptmx.Write([]byte("\x1b"))
							approvalResolved = true
							decisionApplied = true
						}
					}
				}
			}
			if readErr != nil {
				readDone <- readErr
				return
			}
		}
	}()

	waitErr := cmd.Wait()
	<-readDone
	if pending := progress.Flush(); pending != "" && opts.ProgressHandler != nil {
		opts.ProgressHandler(ProgressEvent{Text: pending})
	}

	if errorsIsDeadline(ctx) {
		return finalizeTurnResult(result, outputPath, plainBuilder.String(), opts.ProjectPath, startedAt), fmt.Errorf("Codex 执行超时（>%s）", opts.Timeout)
	}
	if waitErr != nil {
		if exitErr, ok := waitErr.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		}
		res := finalizeTurnResult(result, outputPath, plainBuilder.String(), opts.ProjectPath, startedAt)
		if approvalHandled && approvalResolved && decisionApplied {
			if strings.TrimSpace(res.ResponseText) == "" {
				res.ResponseText = "本次原生审批已拒绝，Codex 未执行该命令。"
			}
		}
		return res, fmt.Errorf("Codex 执行失败：%w", waitErr)
	}
	return finalizeTurnResult(result, outputPath, plainBuilder.String(), opts.ProjectPath, startedAt), nil
}

func finalizeTurnResult(result TurnResult, outputPath, plainText, projectPath string, startedAt time.Time) TurnResult {
	if data, err := os.ReadFile(outputPath); err == nil {
		result.ResponseText = strings.TrimSpace(string(data))
	}
	result.CombinedText = strings.TrimSpace(plainText)
	if result.ThreadID == "" {
		result.ThreadID = findLatestThreadID(projectPath, startedAt)
	}
	return result
}

func errorsIsDeadline(ctx context.Context) bool {
	return ctx.Err() == context.DeadlineExceeded
}

func sanitizeTTY(text string) string {
	cleaned := ansiOSC.ReplaceAllString(text, "")
	cleaned = ansiCSI.ReplaceAllString(cleaned, "")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")
	cleaned = ctrlSeq.ReplaceAllString(cleaned, "")
	lines := strings.Split(cleaned, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

func tailString(text string, max int) string {
	runes := []rune(text)
	if len(runes) <= max {
		return string(runes)
	}
	return string(runes[len(runes)-max:])
}

func detectApprovalRequest(text string) (ApprovalRequest, bool) {
	if !strings.Contains(text, "Would you like to run the following command?") {
		return ApprovalRequest{}, false
	}
	if !strings.Contains(text, "Press enter to confirm or esc to cancel") {
		return ApprovalRequest{}, false
	}
	request := ApprovalRequest{
		Title:   "Codex 原生审批请求",
		RawText: tailString(text, 2500),
	}
	if idx := strings.Index(text, "Reason:"); idx >= 0 {
		reasonBlock := text[idx+len("Reason:"):]
		reasonBlock = strings.TrimSpace(reasonBlock)
		if dollar := strings.Index(reasonBlock, "$ "); dollar >= 0 {
			request.Reason = strings.TrimSpace(reasonBlock[:dollar])
			reasonBlock = reasonBlock[dollar+2:]
			lines := strings.Split(reasonBlock, "\n")
			if len(lines) > 0 {
				request.Command = strings.TrimSpace(lines[0])
			}
		}
	}
	if request.Reason == "" {
		request.Reason = "Codex 请求执行命令，需要人工确认。"
	}
	return request, true
}

type progressAccumulator struct {
	recent    []string
	pending   []string
	lastFlush time.Time
}

func newProgressAccumulator() *progressAccumulator {
	return &progressAccumulator{recent: make([]string, 0, 32), pending: make([]string, 0, 8)}
}

func (p *progressAccumulator) AddPlain(plain string) []string {
	if strings.TrimSpace(plain) == "" {
		return nil
	}
	now := time.Now()
	flushes := make([]string, 0)
	for _, line := range strings.Split(plain, "\n") {
		line = normalizeProgressLine(line)
		if line == "" || isProgressNoise(line) || p.seen(line) {
			continue
		}
		p.pending = append(p.pending, line)
		if p.shouldFlush(now) {
			flushes = append(flushes, p.flush(now))
		}
	}
	return flushes
}

func (p *progressAccumulator) Flush() string {
	if len(p.pending) == 0 {
		return ""
	}
	return p.flush(time.Now())
}

func (p *progressAccumulator) shouldFlush(now time.Time) bool {
	if len(p.pending) == 0 {
		return false
	}
	if p.lastFlush.IsZero() {
		return true
	}
	chars := 0
	for _, item := range p.pending {
		chars += len([]rune(item))
	}
	return len(p.pending) >= 3 || chars >= 220 || now.Sub(p.lastFlush) >= 2*time.Second
}

func (p *progressAccumulator) flush(now time.Time) string {
	text := strings.Join(p.pending, "\n")
	p.pending = p.pending[:0]
	p.lastFlush = now
	return text
}

func (p *progressAccumulator) seen(line string) bool {
	for _, item := range p.recent {
		if item == line {
			return true
		}
	}
	p.recent = append(p.recent, line)
	if len(p.recent) > 48 {
		p.recent = append([]string(nil), p.recent[len(p.recent)-48:]...)
	}
	return false
}

func normalizeProgressLine(line string) string {
	line = strings.TrimSpace(strings.ReplaceAll(line, "\t", " "))
	line = regexp.MustCompile(`\s+`).ReplaceAllString(line, " ")
	return line
}

func isProgressNoise(line string) bool {
	if line == "" {
		return true
	}
	noiseContains := []string{
		"OpenAI Codex",
		"directory:",
		"model:",
		"Use /mcp",
		"? for shortcuts",
		"context left",
		"esc to interrupt",
		"Starting MCP servers",
		"Under-development features enabled",
		"suppress_unstable_features_warning",
		"Would you like to run the following command?",
		"Press enter to confirm or esc to cancel",
		"Conversation interrupted",
		"Something went wrong? Hit /feedback",
	}
	for _, item := range noiseContains {
		if strings.Contains(line, item) {
			return true
		}
	}
	noisePrefixes := []string{
		"Tip:",
		">",
		"›",
		"1. Yes, proceed",
		"2. Yes, and don't ask again",
		"3. No, and tell Codex",
		"$ ",
	}
	for _, prefix := range noisePrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	if strings.Trim(line, "─│╭╮╰╯•◦ ") == "" {
		return true
	}
	return false
}

func buildPrompt(opts TurnOptions) string {
	prompt := strings.TrimSpace(opts.Prompt)
	if opts.MaxPromptChars > 0 && len([]rune(prompt)) > opts.MaxPromptChars {
		prompt = string([]rune(prompt)[:opts.MaxPromptChars])
	}
	return prompt
}

func findLatestThreadID(projectPath string, startedAt time.Time) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	codexHome := os.Getenv("CODEX_HOME")
	if codexHome == "" {
		codexHome = filepath.Join(homeDir, ".codex")
	}
	sessionsDir := filepath.Join(codexHome, "sessions")
	absProject, _ := filepath.Abs(projectPath)
	type candidate struct {
		id      string
		modTime time.Time
	}
	items := make([]candidate, 0)
	_ = filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if info.ModTime().Before(startedAt.Add(-2 * time.Minute)) {
			return nil
		}
		id, cwd := parseSessionMeta(path)
		if id == "" || cwd == "" {
			return nil
		}
		if absCwd, err := filepath.Abs(cwd); err == nil && absCwd == absProject {
			items = append(items, candidate{id: id, modTime: info.ModTime()})
		}
		return nil
	})
	if len(items) == 0 {
		return ""
	}
	sort.Slice(items, func(i, j int) bool { return items[i].modTime.After(items[j].modTime) })
	return items[0].id
}

func parseSessionMeta(path string) (string, string) {
	file, err := os.Open(path)
	if err != nil {
		return "", ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 256*1024), 256*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry struct {
			Type    string          `json:"type"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		if entry.Type != "session_meta" {
			continue
		}
		var meta struct {
			ID  string `json:"id"`
			Cwd string `json:"cwd"`
		}
		if json.Unmarshal(entry.Payload, &meta) == nil {
			return meta.ID, meta.Cwd
		}
	}
	return "", ""
}
