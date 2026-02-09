package protocol

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

const ProtocolDir = "/tmp/codebutler"

// Message types
type IncomingMessage struct {
	Type        string    `json:"type"`
	Timestamp   string    `json:"timestamp"`
	MessageID   string    `json:"message_id"`
	From        Contact   `json:"from"`
	Chat        Contact   `json:"chat"`
	Content     string    `json:"content"`
	IsVoice     bool      `json:"is_voice"`
	Transcript  *string   `json:"transcript"`
}

type OutgoingResponse struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	ReplyTo   string `json:"reply_to"`
	ChatJID   string `json:"chat_jid"`
	Content   string `json:"content"`
}

type Question struct {
	Type       string   `json:"type"`
	Timestamp  string   `json:"timestamp"`
	QuestionID string   `json:"question_id"`
	ChatJID    string   `json:"chat_jid"`
	Text       string   `json:"text"`
	Options    []string `json:"options"`
	Timeout    int      `json:"timeout"`
}

type Answer struct {
	Type       string `json:"type"`
	Timestamp  string `json:"timestamp"`
	QuestionID string `json:"question_id"`
	Selected   int    `json:"selected"`
	Text       string `json:"text"`
}

type Contact struct {
	JID  string `json:"jid"`
	Name string `json:"name"`
}

// File paths
var (
	IncomingPath = ProtocolDir + "/incoming.json"
	OutgoingPath = ProtocolDir + "/outgoing.json"
	QuestionPath = ProtocolDir + "/question.json"
	AnswerPath   = ProtocolDir + "/answer.json"
)

// Initialize creates the protocol directory
func Initialize() error {
	return os.MkdirAll(ProtocolDir, 0755)
}

// WriteIncoming writes an incoming message
func WriteIncoming(msg *IncomingMessage) error {
	msg.Type = "message"
	msg.Timestamp = time.Now().Format(time.RFC3339)
	return writeJSON(IncomingPath, msg)
}

// ReadIncoming reads an incoming message (blocks if not exists)
func ReadIncoming() (*IncomingMessage, error) {
	var msg IncomingMessage
	if err := readJSON(IncomingPath, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// WriteOutgoing writes an outgoing response
func WriteOutgoing(resp *OutgoingResponse) error {
	resp.Type = "response"
	resp.Timestamp = time.Now().Format(time.RFC3339)
	return writeJSON(OutgoingPath, resp)
}

// ReadOutgoing reads an outgoing response
func ReadOutgoing() (*OutgoingResponse, error) {
	var resp OutgoingResponse
	if err := readJSON(OutgoingPath, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// WriteQuestion writes a question
func WriteQuestion(q *Question) error {
	q.Type = "question"
	q.Timestamp = time.Now().Format(time.RFC3339)
	return writeJSON(QuestionPath, q)
}

// ReadQuestion reads a question
func ReadQuestion() (*Question, error) {
	var q Question
	if err := readJSON(QuestionPath, &q); err != nil {
		return nil, err
	}
	return &q, nil
}

// WriteAnswer writes an answer
func WriteAnswer(ans *Answer) error {
	ans.Type = "answer"
	ans.Timestamp = time.Now().Format(time.RFC3339)
	return writeJSON(AnswerPath, ans)
}

// ReadAnswer reads an answer
func ReadAnswer() (*Answer, error) {
	var ans Answer
	if err := readJSON(AnswerPath, &ans); err != nil {
		return nil, err
	}
	return &ans, nil
}

// FileExists checks if a protocol file exists
func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// DeleteFile removes a protocol file
func DeleteFile(path string) error {
	if !FileExists(path) {
		return nil
	}
	return os.Remove(path)
}

// Helper functions
func writeJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write error: %w", err)
	}

	return nil
}

func readJSON(path string, v interface{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read error: %w", err)
	}

	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("unmarshal error: %w", err)
	}

	return nil
}
