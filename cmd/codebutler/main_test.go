package main

import "testing"

func TestValidRoles(t *testing.T) {
	expected := []string{"pm", "coder", "reviewer", "researcher", "artist", "lead"}
	for _, role := range expected {
		if !validRoles[role] {
			t.Errorf("expected %q to be a valid role", role)
		}
	}
}

func TestInvalidRoles(t *testing.T) {
	invalid := []string{"", "admin", "manager", "PM", "CODER"}
	for _, role := range invalid {
		if validRoles[role] {
			t.Errorf("expected %q to be an invalid role", role)
		}
	}
}
