package session

import (
	"fmt"
	"sync"
)

type Session struct {
	ChatID          string
	ActiveRepo      string
	ActiveRepoPath  string
	LastCommandTime int64
}

type Manager struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

func NewManager() *Manager {
	return &Manager{
		sessions: make(map[string]*Session),
	}
}

func (m *Manager) GetSession(chatID string) *Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if session, exists := m.sessions[chatID]; exists {
		return session
	}

	return nil
}

func (m *Manager) SetActiveRepo(chatID, repoName, repoPath string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[chatID]
	if !exists {
		session = &Session{
			ChatID: chatID,
		}
		m.sessions[chatID] = session
	}

	session.ActiveRepo = repoName
	session.ActiveRepoPath = repoPath
}

func (m *Manager) GetActiveRepo(chatID string) (string, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	session, exists := m.sessions[chatID]
	if !exists || session.ActiveRepo == "" {
		return "", "", fmt.Errorf("no active repository")
	}

	return session.ActiveRepo, session.ActiveRepoPath, nil
}

func (m *Manager) ClearSession(chatID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.sessions, chatID)
}

func (m *Manager) ListSessions() []Session {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sessions := make([]Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		sessions = append(sessions, *session)
	}

	return sessions
}
