package codex

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

var (
	ansiCSI = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	ansiOSC = regexp.MustCompile(`\x1b\][^\x07]*(\x07|\x1b\\)`)
	ctrlSeq = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f\x7f]`)
)

type sessionMeta struct {
	ID     string
	Cwd    string
	Source string
}

type sessionWatchResult struct {
	ThreadID     string
	ResponseText string
}

type sessionWatchUpdate struct {
	ThreadID         string
	LastAgentText    string
	TaskComplete     bool
	FinalAnswerText  string
	ApprovalRequests []ApprovalRequest
}

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

	codexHome, err := prepareCodexRuntimeHome(opts.ProjectPath)
	if err != nil {
		return result, err
	}
	effectiveThreadID := resolveRunnableThreadID(codexHome, opts.ThreadID)

	args := []string{"--no-alt-screen", "-a", "on-request"}
	if opts.Model != "" {
		args = append(args, "-m", opts.Model)
	}
	if opts.SandboxMode != "" {
		args = append(args, "-s", opts.SandboxMode)
	}
	args = append(args, "-C", opts.ProjectPath)
	if effectiveThreadID != "" {
		args = append(args, "resume", effectiveThreadID, prompt)
	} else {
		args = append(args, prompt)
	}

	cmd := exec.CommandContext(ctx, r.Binary, args...)
	cmd.Dir = opts.ProjectPath
	cmd.Env = buildCleanCodexEnv(codexHome)

	startedAt := time.Now()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return result, err
	}
	defer func() { _ = ptmx.Close() }()

	var plainBuilder strings.Builder
	progress := newProgressAccumulator()
	watchDone := make(chan sessionWatchResult, 1)
	watchErr := make(chan error, 1)
	processDone := make(chan error, 1)
	approvalEvents := make(chan ApprovalRequest, 4)

	go func() {
		processDone <- cmd.Wait()
	}()
	go func() {
		res, watchErrValue := waitForTurnResult(ctx, codexHome, opts.ProjectPath, opts.ThreadID, startedAt, approvalEvents)
		if watchErrValue != nil {
			watchErr <- watchErrValue
			return
		}
		watchDone <- res
	}()

	approvalDenied := false
	readDone := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(ptmx)
		for {
			chunk := make([]byte, 4096)
			n, readErr := reader.Read(chunk)
			if n > 0 {
				text := string(chunk[:n])
				respondToTerminalQueries(ptmx, text)
				plain := sanitizeTTY(text)
				if strings.Contains(plain, "Continue anyway? [y/N]:") {
					_, _ = ptmx.Write([]byte("y\r"))
				}
				if strings.TrimSpace(plain) != "" {
					plainBuilder.WriteString(plain)
					plainBuilder.WriteByte('\n')
					if opts.ProgressHandler != nil {
						for _, flush := range progress.AddPlain(plain) {
							opts.ProgressHandler(ProgressEvent{Text: flush})
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

	for {
		select {
		case req := <-approvalEvents:
			if handleApprovalRequest(ptmx, opts, progress, req) {
				approvalDenied = true
				_ = cmd.Process.Kill()
			}
		case watch := <-watchDone:
			result.ThreadID = watch.ThreadID
			result.ResponseText = strings.TrimSpace(watch.ResponseText)
			result.CombinedText = strings.TrimSpace(plainBuilder.String())
			_ = cmd.Process.Kill()
			<-processDone
			<-readDone
			if pending := progress.Flush(); pending != "" && opts.ProgressHandler != nil {
				opts.ProgressHandler(ProgressEvent{Text: pending})
			}
			if result.ThreadID == "" {
				result.ThreadID = findLatestThreadID(codexHome, opts.ProjectPath, startedAt)
			}
			if result.ResponseText == "" && approvalDenied {
				result.ResponseText = "本次原生审批已拒绝，Codex 未执行该命令。"
			}
			return result, nil
		case waitErr := <-processDone:
			_ = <-readDone
			if pending := progress.Flush(); pending != "" && opts.ProgressHandler != nil {
				opts.ProgressHandler(ProgressEvent{Text: pending})
			}
			result.CombinedText = strings.TrimSpace(plainBuilder.String())
			if result.ThreadID == "" {
				result.ThreadID = findLatestThreadID(codexHome, opts.ProjectPath, startedAt)
			}
			if approvalDenied {
				result.ResponseText = "本次原生审批已拒绝，Codex 未执行该命令。"
				return result, nil
			}
			if errorsIsDeadline(ctx) {
				return result, fmt.Errorf("Codex 执行超时（>%s）", opts.Timeout)
			}
			if waitErr != nil {
				if exitErr, ok := waitErr.(*exec.ExitError); ok {
					result.ExitCode = exitErr.ExitCode()
				}
				if result.ResponseText == "" {
					result.ResponseText = extractLastAgentMessage(result.CombinedText)
				}
				return result, fmt.Errorf("Codex 执行失败：%w", waitErr)
			}
			return result, nil
		case err := <-watchErr:
			if approvalDenied {
				result.ResponseText = "本次原生审批已拒绝，Codex 未执行该命令。"
				result.CombinedText = strings.TrimSpace(plainBuilder.String())
				_ = cmd.Process.Kill()
				<-processDone
				<-readDone
				return result, nil
			}
			_ = cmd.Process.Kill()
			<-processDone
			<-readDone
			result.CombinedText = strings.TrimSpace(plainBuilder.String())
			return result, err
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			<-processDone
			<-readDone
			result.CombinedText = strings.TrimSpace(plainBuilder.String())
			return result, fmt.Errorf("Codex 执行超时（>%s）", opts.Timeout)
		}
	}
}

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

func waitForTurnResult(ctx context.Context, codexHome, projectPath, threadID string, startedAt time.Time, approvalEvents chan<- ApprovalRequest) (sessionWatchResult, error) {
	sessionPath := ""
	resultThreadID := threadID
	if threadID != "" {
		sessionPath = findSessionFileByID(codexHome, threadID)
	}
	for sessionPath == "" {
		path, id := findLatestSessionFile(codexHome, projectPath, startedAt, "cli")
		if path != "" {
			sessionPath = path
			if resultThreadID == "" {
				resultThreadID = id
			}
			break
		}
		select {
		case <-ctx.Done():
			return sessionWatchResult{}, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}

	offset := int64(0)
	if threadID != "" {
		offset = fileSize(sessionPath)
	}
	for {
		update, nextOffset, err := readSessionUpdates(sessionPath, offset)
		if err == nil {
			offset = nextOffset
			if update.ThreadID != "" {
				resultThreadID = update.ThreadID
			}
			for _, approval := range update.ApprovalRequests {
				if approvalEvents == nil {
					continue
				}
				select {
				case approvalEvents <- approval:
				case <-ctx.Done():
					return sessionWatchResult{}, ctx.Err()
				}
			}
			if update.TaskComplete {
				response := strings.TrimSpace(update.LastAgentText)
				if response == "" {
					response = strings.TrimSpace(update.FinalAnswerText)
				}
				return sessionWatchResult{ThreadID: resultThreadID, ResponseText: response}, nil
			}
		}
		select {
		case <-ctx.Done():
			return sessionWatchResult{}, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func readSessionUpdates(path string, offset int64) (sessionWatchUpdate, int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionWatchUpdate{}, offset, err
	}
	if offset < 0 || offset > int64(len(data)) {
		offset = 0
	}
	chunk := data[offset:]
	update := sessionWatchUpdate{}
	scanner := bufio.NewScanner(bytes.NewReader(chunk))
	scanner.Buffer(make([]byte, 1024*1024), 8*1024*1024)
	for scanner.Scan() {
		parseSessionLine(scanner.Text(), &update)
	}
	if err := scanner.Err(); err != nil {
		return update, offset, err
	}
	return update, int64(len(data)), nil
}

func parseSessionLine(line string, update *sessionWatchUpdate) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	var entry struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal([]byte(line), &entry); err != nil {
		return
	}
	switch entry.Type {
	case "session_meta":
		var meta struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(entry.Payload, &meta) == nil && meta.ID != "" {
			update.ThreadID = meta.ID
		}
	case "event_msg":
		var payload struct {
			Type             string `json:"type"`
			LastAgentMessage string `json:"last_agent_message"`
		}
		if json.Unmarshal(entry.Payload, &payload) == nil && payload.Type == "task_complete" {
			update.TaskComplete = true
			update.LastAgentText = payload.LastAgentMessage
		}
	case "response_item":
		var head struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(entry.Payload, &head) != nil {
			return
		}
		switch head.Type {
		case "message":
			var payload struct {
				Type    string `json:"type"`
				Role    string `json:"role"`
				Phase   string `json:"phase"`
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			}
			if json.Unmarshal(entry.Payload, &payload) == nil && payload.Role == "assistant" {
				parts := make([]string, 0, len(payload.Content))
				for _, item := range payload.Content {
					if item.Type == "output_text" && strings.TrimSpace(item.Text) != "" {
						parts = append(parts, item.Text)
					}
				}
				if len(parts) > 0 {
					update.FinalAnswerText = strings.Join(parts, "")
				}
			}
		case "function_call":
			var payload struct {
				Type      string `json:"type"`
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
				CallID    string `json:"call_id"`
			}
			if json.Unmarshal(entry.Payload, &payload) == nil {
				if approval, ok := parseApprovalRequestFromFunctionCall(payload.Name, payload.Arguments); ok {
					update.ApprovalRequests = append(update.ApprovalRequests, approval)
				}
			}
		}
	}
}

func handleApprovalRequest(ptmx *os.File, opts TurnOptions, progress *progressAccumulator, req ApprovalRequest) bool {
	if pending := progress.Flush(); pending != "" && opts.ProgressHandler != nil {
		opts.ProgressHandler(ProgressEvent{Text: pending})
	}
	decision := ApprovalDeny
	if opts.ApprovalHandler != nil {
		decisionValue, handlerErr := opts.ApprovalHandler(req)
		if handlerErr == nil {
			decision = decisionValue
		}
	}
	if decision == ApprovalAllow || decision == ApprovalAllowForSession {
		_, _ = ptmx.Write([]byte("\r"))
		return false
	}
	_, _ = ptmx.Write([]byte("\x1b"))
	return true
}

func parseApprovalRequestFromFunctionCall(name, arguments string) (ApprovalRequest, bool) {
	var payload struct {
		Cmd                string `json:"cmd"`
		Justification      string `json:"justification"`
		SandboxPermissions string `json:"sandbox_permissions"`
	}
	if json.Unmarshal([]byte(arguments), &payload) != nil {
		return ApprovalRequest{}, false
	}
	if payload.SandboxPermissions != "require_escalated" {
		return ApprovalRequest{}, false
	}
	request := ApprovalRequest{
		Title:   "Codex 原生审批请求",
		Reason:  strings.TrimSpace(payload.Justification),
		Command: strings.TrimSpace(payload.Cmd),
		RawText: strings.TrimSpace(arguments),
	}
	if request.Reason == "" {
		request.Reason = "Codex 请求执行受限操作，需要人工确认。"
	}
	if request.Command == "" {
		request.Command = strings.TrimSpace(name)
	}
	return request, true
}

func respondToTerminalQueries(ptmx *os.File, text string) {
	if strings.Contains(text, "\x1b[6n") {
		_, _ = ptmx.Write([]byte("\x1b[1;1R"))
	}
}

func prepareCodexRuntimeHome(projectPath string) (string, error) {
	return prepareScopedCodexRuntimeHome(projectPath, "")
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
	runtimeHome := filepath.Join(os.TempDir(), "qq-codex-go-codex-home", hex.EncodeToString(hash[:8]))
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

func copyStream(dst io.Writer, src io.Reader) error {
	_, err := io.Copy(dst, src)
	return err
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

func findLatestThreadID(codexHome, projectPath string, startedAt time.Time) string {
	_, id := findLatestSessionFile(codexHome, projectPath, startedAt, "")
	return id
}

func findLatestSessionFile(codexHome, projectPath string, startedAt time.Time, source string) (string, string) {
	sessionsDir := codexSessionsDir(codexHome)
	absProject, _ := filepath.Abs(projectPath)
	type candidate struct {
		path    string
		id      string
		modTime time.Time
	}
	items := make([]candidate, 0)
	_ = filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		if info.ModTime().Before(startedAt) {
			return nil
		}
		meta := parseSessionMeta(path)
		if meta.ID == "" || meta.Cwd == "" {
			return nil
		}
		if source != "" && meta.Source != source {
			return nil
		}
		if absCwd, err := filepath.Abs(meta.Cwd); err == nil && absCwd == absProject {
			items = append(items, candidate{path: path, id: meta.ID, modTime: info.ModTime()})
		}
		return nil
	})
	if len(items) == 0 {
		return "", ""
	}
	sort.Slice(items, func(i, j int) bool { return items[i].modTime.After(items[j].modTime) })
	return items[0].path, items[0].id
}

func findSessionFileByID(codexHome, threadID string) string {
	sessionsDir := codexSessionsDir(codexHome)
	found := ""
	_ = filepath.Walk(sessionsDir, func(path string, info os.FileInfo, err error) error {
		if found != "" {
			return filepath.SkipAll
		}
		if err != nil || info == nil || info.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		meta := parseSessionMeta(path)
		if meta.ID == threadID {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func resolveRunnableThreadID(codexHome, threadID string) string {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return ""
	}
	if findSessionFileByID(codexHome, threadID) == "" {
		return ""
	}
	return threadID
}

func parseSessionMeta(path string) sessionMeta {
	file, err := os.Open(path)
	if err != nil {
		return sessionMeta{}
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 256*1024), 1024*1024)
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
			ID     string `json:"id"`
			Cwd    string `json:"cwd"`
			Source string `json:"source"`
		}
		if json.Unmarshal(entry.Payload, &meta) == nil {
			return sessionMeta{ID: meta.ID, Cwd: meta.Cwd, Source: meta.Source}
		}
	}
	return sessionMeta{}
}

func codexSessionsDir(codexHome string) string {
	if strings.TrimSpace(codexHome) == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		codexHome = filepath.Join(homeDir, ".codex")
	}
	return filepath.Join(codexHome, "sessions")
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func extractLastAgentMessage(text string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if len(lines) == 0 {
		return ""
	}
	return lines[len(lines)-1]
}
