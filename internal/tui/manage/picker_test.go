package manage

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/evanmschultz/valv/internal/domain"
)

func TestProfilePickerSelectsProfile(t *testing.T) {
	profiles := []domain.Profile{
		{Name: "dev", HomePath: "/tmp/dev", Provider: domain.ProviderCodex},
		{Name: "work", HomePath: "/tmp/work", Provider: domain.ProviderCodex},
	}
	model := NewProfilePicker(domain.ProviderCodex, profiles)
	next, _ := model.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEnter}))
	picked, ok := next.(ProfilePickerModel)
	if !ok {
		t.Fatalf("Update() returned %T, want ProfilePickerModel", next)
	}
	selected, ok := picked.Selected()
	if !ok || selected.Name != "dev" {
		t.Fatalf("Selected() = %q, %t; want dev, true", selected.Name, ok)
	}
	if selected.Provider != domain.ProviderCodex {
		t.Fatalf("Selected().Provider = %q, want codex", selected.Provider)
	}
}
