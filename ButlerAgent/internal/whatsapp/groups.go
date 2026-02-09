package whatsapp

import (
	"context"
	"fmt"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// Group represents a WhatsApp group
type Group struct {
	JID  string
	Name string
}

// GetGroups returns all groups the user is a member of
func (c *Client) GetGroups() ([]Group, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to WhatsApp")
	}

	// Get all groups from WhatsApp
	groups, err := c.wac.GetJoinedGroups(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get groups: %w", err)
	}

	result := make([]Group, 0, len(groups))
	for _, group := range groups {
		result = append(result, Group{
			JID:  group.JID.String(),
			Name: group.Name,
		})
	}

	return result, nil
}

// CreateGroup creates a new WhatsApp group
// participants should be phone numbers in international format (e.g., "1234567890")
func (c *Client) CreateGroup(name string, participants []string) (string, error) {
	if !c.IsConnected() {
		return "", fmt.Errorf("not connected to WhatsApp")
	}

	// Convert phone numbers to JIDs
	participantJIDs := make([]types.JID, 0, len(participants))
	for _, phone := range participants {
		jid := types.NewJID(phone, types.DefaultUserServer)
		participantJIDs = append(participantJIDs, jid)
	}

	// Create group
	resp, err := c.wac.CreateGroup(context.Background(), whatsmeow.ReqCreateGroup{
		Name:         name,
		Participants: participantJIDs,
	})

	if err != nil {
		return "", fmt.Errorf("failed to create group: %w", err)
	}

	return resp.JID.String(), nil
}

// GetGroupInfo returns detailed information about a group
func (c *Client) GetGroupInfo(groupJID string) (*types.GroupInfo, error) {
	if !c.IsConnected() {
		return nil, fmt.Errorf("not connected to WhatsApp")
	}

	// Parse JID
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return nil, fmt.Errorf("invalid group JID: %w", err)
	}

	// Get group info
	info, err := c.wac.GetGroupInfo(context.Background(), jid)
	if err != nil {
		return nil, fmt.Errorf("failed to get group info: %w", err)
	}

	return info, nil
}

// AddParticipants adds members to a group
func (c *Client) AddParticipants(groupJID string, participants []string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to WhatsApp")
	}

	// Parse group JID
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("invalid group JID: %w", err)
	}

	// Convert phone numbers to JIDs
	participantJIDs := make([]types.JID, 0, len(participants))
	for _, phone := range participants {
		participantJID := types.NewJID(phone, types.DefaultUserServer)
		participantJIDs = append(participantJIDs, participantJID)
	}

	// Add participants
	_, err = c.wac.UpdateGroupParticipants(context.Background(), jid, participantJIDs, whatsmeow.ParticipantChangeAdd)
	if err != nil {
		return fmt.Errorf("failed to add participants: %w", err)
	}

	return nil
}

// RemoveParticipants removes members from a group
func (c *Client) RemoveParticipants(groupJID string, participants []string) error {
	if !c.IsConnected() {
		return fmt.Errorf("not connected to WhatsApp")
	}

	// Parse group JID
	jid, err := types.ParseJID(groupJID)
	if err != nil {
		return fmt.Errorf("invalid group JID: %w", err)
	}

	// Convert phone numbers to JIDs
	participantJIDs := make([]types.JID, 0, len(participants))
	for _, phone := range participants {
		participantJID := types.NewJID(phone, types.DefaultUserServer)
		participantJIDs = append(participantJIDs, participantJID)
	}

	// Remove participants
	_, err = c.wac.UpdateGroupParticipants(context.Background(), jid, participantJIDs, whatsmeow.ParticipantChangeRemove)
	if err != nil {
		return fmt.Errorf("failed to remove participants: %w", err)
	}

	return nil
}
