package weixin

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type State struct {
	dir         string
	cursorPath  string
	tokensPath  string
	mu          sync.Mutex
	contexts    map[string]string
	contextLoad bool
}

func NewState(dir string) (*State, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &State{
		dir:        dir,
		cursorPath: filepath.Join(dir, "get_updates.buf"),
		tokensPath: filepath.Join(dir, "context_tokens.json"),
		contexts:   map[string]string{},
	}, nil
}

func (s *State) LoadCursor() (string, error) {
	data, err := os.ReadFile(s.cursorPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

func (s *State) SaveCursor(cursor string) error {
	return os.WriteFile(s.cursorPath, []byte(cursor), 0o600)
}

func (s *State) LoadContextToken(peer string) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadContextsLocked(); err != nil {
		return "", false, err
	}
	value, ok := s.contexts[peer]
	return value, ok, nil
}

func (s *State) SaveContextToken(peer, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadContextsLocked(); err != nil {
		return err
	}
	s.contexts[peer] = token
	data, err := json.MarshalIndent(s.contexts, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.tokensPath, data, 0o600)
}

func (s *State) loadContextsLocked() error {
	if s.contextLoad {
		return nil
	}
	data, err := os.ReadFile(s.tokensPath)
	if err != nil {
		if os.IsNotExist(err) {
			s.contextLoad = true
			return nil
		}
		return err
	}
	if len(data) == 0 {
		s.contextLoad = true
		return nil
	}
	if err := json.Unmarshal(data, &s.contexts); err != nil {
		return err
	}
	s.contextLoad = true
	return nil
}
