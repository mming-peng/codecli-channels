package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type SessionRecord struct {
	ID              string    `json:"id"`
	OwnerKey        string    `json:"ownerKey"`
	ProjectAlias    string    `json:"projectAlias"`
	ProjectPath     string    `json:"projectPath"`
	Name            string    `json:"name"`
	ThreadID        string    `json:"threadId,omitempty"`
	ClaudeSessionID string    `json:"claudeSessionId,omitempty"`
	DefaultRunMode  string    `json:"defaultRunMode"`
	LastTaskAt      time.Time `json:"lastTaskAt,omitempty"`
	LastTaskMode    string    `json:"lastTaskMode,omitempty"`
	LastTaskStatus  string    `json:"lastTaskStatus,omitempty"`
	LastTaskSummary string    `json:"lastTaskSummary,omitempty"`
	LastBackend     string    `json:"lastBackend,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type SessionTaskSummary struct {
	At      time.Time
	Backend string
	Mode    string
	Status  string
	Summary string
}

type state struct {
	ConversationProjects map[string]string         `json:"conversationProjects"`
	ConversationBackends map[string]string         `json:"conversationBackends"`
	ActiveSessions       map[string]string         `json:"activeSessions"`
	Sessions             map[string]*SessionRecord `json:"sessions"`
	NextSessionID        int64                     `json:"nextSessionId"`
}

type Store struct {
	mu   sync.Mutex
	path string
	data state
}

func New(path string) (*Store, error) {
	s := &Store{path: path}
	s.data = state{
		ConversationProjects: map[string]string{},
		ConversationBackends: map[string]string{},
		ActiveSessions:       map[string]string{},
		Sessions:             map[string]*SessionRecord{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func ConversationKey(accountID, chatType, targetID string) string {
	return accountID + ":" + chatType + ":" + targetID
}

func activeKey(conversationKey, projectAlias string) string {
	return conversationKey + "|" + projectAlias
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var loaded state
	if err := json.Unmarshal(data, &loaded); err != nil {
		return err
	}
	if loaded.ConversationProjects == nil {
		loaded.ConversationProjects = map[string]string{}
	}
	if loaded.ConversationBackends == nil {
		loaded.ConversationBackends = map[string]string{}
	}
	if loaded.ActiveSessions == nil {
		loaded.ActiveSessions = map[string]string{}
	}
	if loaded.Sessions == nil {
		loaded.Sessions = map[string]*SessionRecord{}
	}
	s.data = loaded
	return nil
}

func (s *Store) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) GetProjectAlias(conversationKey, defaultAlias string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	alias := s.data.ConversationProjects[conversationKey]
	if alias == "" {
		alias = defaultAlias
		s.data.ConversationProjects[conversationKey] = alias
		_ = s.saveLocked()
	}
	return alias
}

func (s *Store) GetBackend(conversationKey, defaultBackend string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	backend := strings.ToLower(strings.TrimSpace(s.data.ConversationBackends[conversationKey]))
	if backend == "" {
		backend = strings.ToLower(strings.TrimSpace(defaultBackend))
		if backend == "" {
			backend = "codex"
		}
		s.data.ConversationBackends[conversationKey] = backend
		_ = s.saveLocked()
	}
	return backend
}

func (s *Store) SetBackend(conversationKey, backend string) error {
	backend = strings.ToLower(strings.TrimSpace(backend))
	if backend == "" {
		backend = "codex"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.ConversationBackends[conversationKey] = backend
	return s.saveLocked()
}

func (s *Store) SetProjectAlias(conversationKey, alias string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.ConversationProjects[conversationKey] = alias
	return s.saveLocked()
}

func (s *Store) GetOrCreateActiveSession(conversationKey, projectAlias, projectPath, defaultMode string) (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sessionID := s.data.ActiveSessions[activeKey(conversationKey, projectAlias)]; sessionID != "" {
		if session := s.data.Sessions[sessionID]; session != nil {
			if session.ProjectPath == "" {
				session.ProjectPath = projectPath
			}
			if session.DefaultRunMode == "" {
				session.DefaultRunMode = defaultMode
			}
			return cloneSession(session), nil
		}
	}
	s.data.NextSessionID++
	now := time.Now()
	record := &SessionRecord{
		ID:             fmt.Sprintf("s%d", s.data.NextSessionID),
		OwnerKey:       conversationKey,
		ProjectAlias:   projectAlias,
		ProjectPath:    projectPath,
		Name:           "default",
		DefaultRunMode: defaultMode,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.data.Sessions[record.ID] = record
	s.data.ActiveSessions[activeKey(conversationKey, projectAlias)] = record.ID
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return cloneSession(record), nil
}

func (s *Store) CreateSession(conversationKey, projectAlias, projectPath, name, defaultMode string) (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.NextSessionID++
	now := time.Now()
	record := &SessionRecord{
		ID:             fmt.Sprintf("s%d", s.data.NextSessionID),
		OwnerKey:       conversationKey,
		ProjectAlias:   projectAlias,
		ProjectPath:    projectPath,
		Name:           strings.TrimSpace(name),
		DefaultRunMode: defaultMode,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if record.Name == "" {
		record.Name = "session"
	}
	s.data.Sessions[record.ID] = record
	s.data.ActiveSessions[activeKey(conversationKey, projectAlias)] = record.ID
	if err := s.saveLocked(); err != nil {
		return nil, err
	}
	return cloneSession(record), nil
}

func (s *Store) ListSessions(conversationKey, projectAlias string) []*SessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]*SessionRecord, 0)
	for _, session := range s.data.Sessions {
		if session.OwnerKey == conversationKey && session.ProjectAlias == projectAlias {
			items = append(items, cloneSession(session))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items
}

func (s *Store) SwitchSession(conversationKey, projectAlias, prefix string) (*SessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, session := range s.data.Sessions {
		if session.OwnerKey != conversationKey || session.ProjectAlias != projectAlias {
			continue
		}
		if strings.HasPrefix(session.ID, prefix) || strings.EqualFold(session.Name, prefix) {
			s.data.ActiveSessions[activeKey(conversationKey, projectAlias)] = session.ID
			session.UpdatedAt = time.Now()
			if err := s.saveLocked(); err != nil {
				return nil, err
			}
			return cloneSession(session), nil
		}
	}
	return nil, fmt.Errorf("未找到会话 %s", prefix)
}

func (s *Store) UpdateSession(record *SessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.data.Sessions[record.ID]
	if !ok {
		return fmt.Errorf("会话 %s 不存在", record.ID)
	}
	current.Name = record.Name
	current.ThreadID = record.ThreadID
	current.ClaudeSessionID = record.ClaudeSessionID
	current.ProjectPath = record.ProjectPath
	current.DefaultRunMode = record.DefaultRunMode
	current.LastTaskAt = record.LastTaskAt
	current.LastTaskMode = record.LastTaskMode
	current.LastTaskStatus = record.LastTaskStatus
	current.LastTaskSummary = record.LastTaskSummary
	current.LastBackend = record.LastBackend
	current.UpdatedAt = time.Now()
	return s.saveLocked()
}

func (s *Store) UpdateSessionTaskSummary(sessionID string, summary SessionTaskSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.data.Sessions[sessionID]
	if !ok {
		return fmt.Errorf("会话 %s 不存在", sessionID)
	}
	if !summary.At.IsZero() {
		current.LastTaskAt = summary.At
	}
	current.LastBackend = strings.TrimSpace(summary.Backend)
	current.LastTaskMode = strings.TrimSpace(summary.Mode)
	current.LastTaskStatus = strings.TrimSpace(summary.Status)
	current.LastTaskSummary = strings.TrimSpace(summary.Summary)
	current.UpdatedAt = time.Now()
	return s.saveLocked()
}

func (s *Store) CurrentSession(conversationKey, projectAlias string) *SessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	sessionID := s.data.ActiveSessions[activeKey(conversationKey, projectAlias)]
	if sessionID == "" {
		return nil
	}
	if session := s.data.Sessions[sessionID]; session != nil {
		return cloneSession(session)
	}
	return nil
}

func cloneSession(in *SessionRecord) *SessionRecord {
	if in == nil {
		return nil
	}
	copy := *in
	return &copy
}
