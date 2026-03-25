package manage

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestDefaultItems(t *testing.T) {
	items := DefaultItems()
	if got, want := len(items), 5; got != want {
		t.Fatalf("len(DefaultItems()) = %d, want %d", got, want)
	}

	gotTitles := []string{
		items[0].Title(),
		items[1].Title(),
		items[2].Title(),
		items[3].Title(),
		items[4].Title(),
	}
	wantTitles := []string{"Status", "Profiles", "Update", "Cleanup", "Global switch"}
	for i := range wantTitles {
		if gotTitles[i] != wantTitles[i] {
			t.Fatalf("DefaultItems()[%d].Title() = %q, want %q", i, gotTitles[i], wantTitles[i])
		}
	}
}

func TestNewSeedsFocusedActionAndView(t *testing.T) {
	model := New()

	action, ok := model.CurrentAction()
	if !ok {
		t.Fatal("CurrentAction() = not ok")
	}
	if action != ActionStatus {
		t.Fatalf("CurrentAction() = %q, want %q", action, ActionStatus)
	}

	view := model.View()
	for _, want := range []string{"Valv Manage", "Operator surface", "Actions", "Status"} {
		if !strings.Contains(view.Content, want) {
			t.Fatalf("View() missing %q in %q", want, view.Content)
		}
	}
}

func TestUpdateSelectsFocusedAction(t *testing.T) {
	model := New()

	next, cmd := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want manage.Model", next)
	}

	if updated.Notice() != "status selected" {
		t.Fatalf("Notice() = %q, want %q", updated.Notice(), "status selected")
	}
	if selected, ok := updated.Selected(); !ok || selected != ActionStatus {
		t.Fatalf("Selected() = %q, %t; want %q, true", selected, ok, ActionStatus)
	}

	if cmd == nil {
		t.Fatal("Update() returned nil command")
	}
	msg := cmd()
	if msg != tea.Quit() {
		t.Fatalf("command() = %#v, want tea.Quit", msg)
	}
}

func TestUpdateMovesSelectionBeforeEntering(t *testing.T) {
	model := New()

	next, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
	updated, ok := next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want manage.Model", next)
	}

	action, ok := updated.CurrentAction()
	if !ok {
		t.Fatal("CurrentAction() = not ok after KeyDown")
	}
	if action != ActionProfiles {
		t.Fatalf("CurrentAction() = %q, want %q", action, ActionProfiles)
	}

	next, cmd := updated.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	updated, ok = next.(Model)
	if !ok {
		t.Fatalf("Update() returned %T, want manage.Model", next)
	}
	if cmd == nil {
		t.Fatal("Update() returned nil command for enter")
	}
	if selected, ok := updated.Selected(); !ok || selected != ActionProfiles {
		t.Fatalf("Selected() = %q, %t; want %q, true", selected, ok, ActionProfiles)
	}
	if updated.Notice() != "profiles selected" {
		t.Fatalf("Notice() = %q, want %q", updated.Notice(), "profiles selected")
	}
}
