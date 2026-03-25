package manage

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/evanmschultz/valv/internal/domain"
)

func TestProfilePickerSelectsProfile(t *testing.T) {
	model := NewProfilePicker(domain.ProviderCodex, []domain.Profile{{Name: "dev", HomePath: "/tmp/dev"}, {Name: "work", HomePath: "/tmp/work"}})
	next, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	picked, ok := next.(ProfilePickerModel)
	if !ok {
		t.Fatalf("Update() returned %T, want ProfilePickerModel", next)
	}
	selected, ok := picked.Selected()
	if !ok || selected != "dev" {
		t.Fatalf("Selected() = %q, %t; want dev, true", selected, ok)
	}
}
