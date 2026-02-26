package roadmap

import (
	"testing"
)

const sampleRoadmap = `# Roadmap: MyProject

## 1. Auth system
- Status: done
- Branch: codebutler/auth-system
- Depends on: —
- Acceptance criteria: JWT-based auth, login/register endpoints, middleware

## 2. User profile API
- Status: in_progress
- Branch: codebutler/user-profile
- Depends on: 1
- Acceptance criteria: CRUD endpoints, avatar upload, validation

## 3. Profile UI
- Status: pending
- Depends on: 1, 2
- Acceptance criteria: Profile page, edit form, avatar picker

## 4. Notification system
- Status: pending
- Depends on: —
- Acceptance criteria: Email + push, per-user preferences, queue-based
`

func TestParse(t *testing.T) {
	r, err := ParseString(sampleRoadmap)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if r.Title != "MyProject" {
		t.Errorf("title: got %q", r.Title)
	}
	if len(r.Items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(r.Items))
	}
}

func TestParse_ItemDetails(t *testing.T) {
	r, _ := ParseString(sampleRoadmap)

	tests := []struct {
		num        int
		title      string
		status     Status
		branch     string
		depsLen    int
		acceptance string
	}{
		{1, "Auth system", StatusDone, "codebutler/auth-system", 0, "JWT-based auth, login/register endpoints, middleware"},
		{2, "User profile API", StatusInProgress, "codebutler/user-profile", 1, "CRUD endpoints, avatar upload, validation"},
		{3, "Profile UI", StatusPending, "", 2, "Profile page, edit form, avatar picker"},
		{4, "Notification system", StatusPending, "", 0, "Email + push, per-user preferences, queue-based"},
	}

	for _, tt := range tests {
		item := r.GetItem(tt.num)
		if item == nil {
			t.Errorf("item %d not found", tt.num)
			continue
		}
		if item.Title != tt.title {
			t.Errorf("item %d title: got %q", tt.num, item.Title)
		}
		if item.Status != tt.status {
			t.Errorf("item %d status: got %q", tt.num, item.Status)
		}
		if item.Branch != tt.branch {
			t.Errorf("item %d branch: got %q", tt.num, item.Branch)
		}
		if len(item.DependsOn) != tt.depsLen {
			t.Errorf("item %d deps: expected %d, got %d", tt.num, tt.depsLen, len(item.DependsOn))
		}
		if item.Acceptance != tt.acceptance {
			t.Errorf("item %d acceptance: got %q", tt.num, item.Acceptance)
		}
	}
}

func TestParse_Dependencies(t *testing.T) {
	r, _ := ParseString(sampleRoadmap)

	item3 := r.GetItem(3)
	if item3 == nil {
		t.Fatal("item 3 not found")
	}
	if len(item3.DependsOn) != 2 {
		t.Fatalf("item 3 deps: expected 2, got %d", len(item3.DependsOn))
	}
	if item3.DependsOn[0] != 1 || item3.DependsOn[1] != 2 {
		t.Errorf("item 3 deps: expected [1,2], got %v", item3.DependsOn)
	}
}

func TestParse_Empty(t *testing.T) {
	r, err := ParseString("")
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(r.Items) != 0 {
		t.Errorf("expected 0 items, got %d", len(r.Items))
	}
}

func TestParse_BlockedBy(t *testing.T) {
	md := `# Roadmap: Test

## 1. Feature
- Status: blocked
- Depends on: —
- Acceptance criteria: something
- Blocked by: needs user input on auth approach
`
	r, _ := ParseString(md)
	item := r.GetItem(1)
	if item.BlockedBy != "needs user input on auth approach" {
		t.Errorf("blocked by: got %q", item.BlockedBy)
	}
}

func TestFormat(t *testing.T) {
	r, _ := ParseString(sampleRoadmap)
	formatted := Format(r)

	// Re-parse to verify round-trip
	r2, err := ParseString(formatted)
	if err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if r2.Title != r.Title {
		t.Errorf("title mismatch: %q vs %q", r.Title, r2.Title)
	}
	if len(r2.Items) != len(r.Items) {
		t.Errorf("items count: %d vs %d", len(r.Items), len(r2.Items))
	}
	for i := range r.Items {
		if r.Items[i].Number != r2.Items[i].Number {
			t.Errorf("item %d number mismatch", i)
		}
		if r.Items[i].Status != r2.Items[i].Status {
			t.Errorf("item %d status mismatch: %q vs %q", i, r.Items[i].Status, r2.Items[i].Status)
		}
	}
}

func TestSetStatus(t *testing.T) {
	r, _ := ParseString(sampleRoadmap)
	if err := r.SetStatus(3, StatusInProgress); err != nil {
		t.Fatalf("set status: %v", err)
	}
	if r.GetItem(3).Status != StatusInProgress {
		t.Error("item 3 should be in_progress")
	}
}

func TestSetStatus_NotFound(t *testing.T) {
	r, _ := ParseString(sampleRoadmap)
	if err := r.SetStatus(99, StatusDone); err == nil {
		t.Error("expected error for unknown item")
	}
}

func TestSetBranch(t *testing.T) {
	r, _ := ParseString(sampleRoadmap)
	r.SetBranch(3, "codebutler/profile-ui")
	if r.GetItem(3).Branch != "codebutler/profile-ui" {
		t.Error("branch not set")
	}
}

func TestAddItem(t *testing.T) {
	r, _ := ParseString(sampleRoadmap)
	item := r.AddItem("Admin dashboard", "Admin CRUD, user management", []int{1, 2})

	if item.Number != 5 {
		t.Errorf("expected number 5, got %d", item.Number)
	}
	if item.Status != StatusPending {
		t.Errorf("expected pending, got %q", item.Status)
	}
	if len(r.Items) != 5 {
		t.Errorf("expected 5 items, got %d", len(r.Items))
	}
}

func TestGetItem_NotFound(t *testing.T) {
	r, _ := ParseString(sampleRoadmap)
	if r.GetItem(99) != nil {
		t.Error("expected nil for unknown item")
	}
}

func TestStatus_IsValid(t *testing.T) {
	if !StatusPending.IsValid() {
		t.Error("pending should be valid")
	}
	if !StatusBlocked.IsValid() {
		t.Error("blocked should be valid")
	}
	if Status("invalid").IsValid() {
		t.Error("invalid should not be valid")
	}
}
