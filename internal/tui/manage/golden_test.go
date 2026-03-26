package manage

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	teatest "github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/evanmschultz/valv/internal/domain"
)

func TestManageHomeGolden(t *testing.T) {
	tm := teatest.NewTestModel(t, New(), teatest.WithInitialTermSize(96, 24))
	t.Cleanup(func() { _ = tm.Quit() })
	tm.Send(tea.WindowSizeMsg{Width: 96, Height: 24})

	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
	final := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second)).(Model)
	teatest.RequireEqualOutput(t, []byte(final.View().Content))
}

func TestProfilePickerGolden(t *testing.T) {
	model := NewProfilePicker(domain.ProviderCodex, []domain.Profile{
		{Name: "alpha-profile", HomePath: "/tmp/alpha-profile"},
		{Name: "beta-profile", HomePath: "/tmp/beta-profile"},
	})
	tm := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(96, 24))
	t.Cleanup(func() { _ = tm.Quit() })
	tm.Send(tea.WindowSizeMsg{Width: 96, Height: 24})

	if err := tm.Quit(); err != nil {
		t.Fatal(err)
	}
	final := tm.FinalModel(t, teatest.WithFinalTimeout(2*time.Second)).(ProfilePickerModel)
	teatest.RequireEqualOutput(t, []byte(final.View().Content))
}
