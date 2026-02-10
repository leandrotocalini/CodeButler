package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

type Message struct {
	ID          string `json:"id"`
	WhatsAppID  string `json:"wa_msg_id"`
	From        string `json:"from"`
	Chat        string `json:"chat"`
	Content     string `json:"content"`
	Timestamp   string `json:"timestamp"`
	IsVoice     bool   `json:"is_voice"`
}

type Store struct {
	db *sql.DB
}

// New opens (or creates) a SQLite store at the given path.
func New(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create store dir: %w", err)
	}

	db, err := sql.Open("sqlite3", "file:"+dbPath+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open store db: %w", err)
	}

	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id        TEXT PRIMARY KEY,
			from_jid  TEXT NOT NULL,
			chat      TEXT NOT NULL,
			content   TEXT NOT NULL,
			timestamp TEXT NOT NULL,
			is_voice  INTEGER DEFAULT 0,
			acked     INTEGER DEFAULT 0,
			wa_msg_id TEXT DEFAULT ''
		)
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create messages table: %w", err)
	}

	// Migrate: add wa_msg_id for existing databases
	db.Exec(`ALTER TABLE messages ADD COLUMN wa_msg_id TEXT DEFAULT ''`)

	st := &Store{db: db}

	if err := st.initSessions(); err != nil {
		db.Close()
		return nil, fmt.Errorf("create sessions table: %w", err)
	}

	return st, nil
}

func (s *Store) Insert(msg Message) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO messages (id, from_jid, chat, content, timestamp, is_voice, acked, wa_msg_id) VALUES (?, ?, ?, ?, ?, ?, 0, ?)`,
		msg.ID, msg.From, msg.Chat, msg.Content, msg.Timestamp, boolToInt(msg.IsVoice), msg.WhatsAppID,
	)
	return err
}

func (s *Store) GetPending() ([]Message, error) {
	rows, err := s.db.Query(
		`SELECT id, from_jid, chat, content, timestamp, is_voice, wa_msg_id FROM messages WHERE acked = 0 ORDER BY timestamp ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var m Message
		var isVoice int
		if err := rows.Scan(&m.ID, &m.From, &m.Chat, &m.Content, &m.Timestamp, &isVoice, &m.WhatsAppID); err != nil {
			return nil, err
		}
		m.IsVoice = isVoice != 0
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}

func (s *Store) Ack(id string) (bool, error) {
	res, err := s.db.Exec(`UPDATE messages SET acked = 1 WHERE id = ? AND acked = 0`, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
