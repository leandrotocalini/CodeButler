package store

// Sessions tracks Claude conversation sessions per chat JID.
// session_id references Claude's internal session for --resume.
// context_summary holds a compacted summary that gets prepended to the
// next prompt after the session is cleared, giving Claude prior context
// without loading the full conversation history.

func (s *Store) initSessions() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			chat_jid        TEXT PRIMARY KEY,
			session_id      TEXT NOT NULL DEFAULT '',
			context_summary TEXT NOT NULL DEFAULT '',
			updated_at      TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return err
	}

	// Migrate: add context_summary for existing databases
	s.db.Exec(`ALTER TABLE sessions ADD COLUMN context_summary TEXT NOT NULL DEFAULT ''`)

	return nil
}

func (s *Store) GetSession(chatJID string) (string, error) {
	var sessionID string
	err := s.db.QueryRow(`SELECT session_id FROM sessions WHERE chat_jid = ?`, chatJID).Scan(&sessionID)
	if err != nil {
		return "", nil // no session yet, not an error
	}
	return sessionID, nil
}

func (s *Store) SetSession(chatJID, sessionID string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (chat_jid, session_id, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(chat_jid) DO UPDATE SET session_id = ?, updated_at = datetime('now')`,
		chatJID, sessionID, sessionID,
	)
	return err
}

func (s *Store) GetSummary(chatJID string) string {
	var summary string
	err := s.db.QueryRow(`SELECT context_summary FROM sessions WHERE chat_jid = ?`, chatJID).Scan(&summary)
	if err != nil {
		return ""
	}
	return summary
}

func (s *Store) SetSummary(chatJID, summary string) error {
	_, err := s.db.Exec(
		`INSERT INTO sessions (chat_jid, context_summary, updated_at) VALUES (?, ?, datetime('now'))
		 ON CONFLICT(chat_jid) DO UPDATE SET context_summary = ?, updated_at = datetime('now')`,
		chatJID, summary, summary,
	)
	return err
}

// ResetSession clears only the session ID, keeping the context summary.
// Used after compaction so the next message starts a fresh session with summary.
func (s *Store) ResetSession(chatJID string) error {
	_, err := s.db.Exec(
		`UPDATE sessions SET session_id = '', updated_at = datetime('now') WHERE chat_jid = ?`,
		chatJID,
	)
	return err
}

// ClearSession removes the session ID and context summary for a chat,
// so the next message starts completely fresh.
func (s *Store) ClearSession(chatJID string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE chat_jid = ?`, chatJID)
	return err
}
