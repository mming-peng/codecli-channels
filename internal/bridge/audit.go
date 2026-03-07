package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type AuditEvent struct {
	Time         time.Time `json:"time"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
	AccountID    string    `json:"accountId,omitempty"`
	ChatType     string    `json:"chatType,omitempty"`
	TargetID     string    `json:"targetId,omitempty"`
	SenderID     string    `json:"senderId,omitempty"`
	ProjectAlias string    `json:"projectAlias,omitempty"`
	ProjectPath  string    `json:"projectPath,omitempty"`
	SessionID    string    `json:"sessionId,omitempty"`
	Mode         string    `json:"mode,omitempty"`
	Text         string    `json:"text,omitempty"`
}

type AuditLogger struct {
	mu   sync.Mutex
	path string
}

func NewAuditLogger(path string) *AuditLogger {
	return &AuditLogger{path: path}
}

func (a *AuditLogger) Write(event AuditEvent) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(a.path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if event.Time.IsZero() {
		event.Time = time.Now()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = file.Write(append(data, '\n'))
	return err
}
