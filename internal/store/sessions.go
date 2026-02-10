package store

// Sessions tracks Claude conversation sessions per chat JID.
// This allows resuming conversations when new messages arrive from the same chat.

func (s *Store) initSessions() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			chat_jid   TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)
	`)
	return err
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
