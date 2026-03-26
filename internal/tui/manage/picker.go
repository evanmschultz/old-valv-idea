package manage

import (
	"fmt"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/evanmschultz/valv/internal/domain"
)

type profileItem struct {
	name string
	home string
}

func (i profileItem) FilterValue() string { return i.name + " " + i.home }
func (i profileItem) Title() string       { return i.name }
func (i profileItem) Description() string { return i.home }

type ProfilePickerModel struct {
	list      list.Model
	provider  domain.Provider
	styles    Styles
	selected  string
	confirmed bool
}

func NewProfilePicker(provider domain.Provider, profiles []domain.Profile) ProfilePickerModel {
	styles := DefaultStyles()
	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(lipgloss.Color("#EAF6FF"))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Foreground(lipgloss.Color("#93A8B7"))
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.Foreground(lipgloss.Color("#A2F2D9")).BorderLeftForeground(lipgloss.Color("#A2F2D9"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("#CFEFFF"))

	items := make([]list.Item, 0, len(profiles))
	for _, profile := range profiles {
		items = append(items, profileItem{name: profile.Name, home: profile.HomePath})
	}
	menu := list.New(
		items,
		delegate,
		max(1, defaultWidth-styles.Frame.GetHorizontalFrameSize()),
		max(1, defaultHeight-layoutMargin-styles.Frame.GetVerticalFrameSize()),
	)
	menu.Title = fmt.Sprintf("%s profiles", provider)
	menu.SetShowFilter(false)
	menu.SetShowPagination(false)
	menu.SetShowStatusBar(true)
	menu.SetShowHelp(true)
	menu.SetStatusBarItemName("profile", "profiles")

	return ProfilePickerModel{list: menu, provider: provider, styles: styles}
}

func (m ProfilePickerModel) Init() tea.Cmd { return nil }

func (m ProfilePickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(
			max(1, msg.Width-m.styles.Frame.GetHorizontalFrameSize()),
			max(1, msg.Height-layoutMargin-m.styles.Frame.GetVerticalFrameSize()),
		)
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "enter" && !m.list.SettingFilter() {
			if item, ok := m.list.SelectedItem().(profileItem); ok {
				m.selected = item.name
				m.confirmed = true
				return m, tea.Quit
			}
		}
	}
	nextList, cmd := m.list.Update(msg)
	m.list = nextList
	return m, cmd
}

func (m ProfilePickerModel) View() tea.View {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.header(),
		m.styles.Frame.Render(m.list.View()),
		m.styles.footer("Select a profile and press enter, or press esc to leave without changing anything."),
	)
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Valv Profile Picker"
	return view
}

func (m ProfilePickerModel) Selected() (string, bool) {
	if !m.confirmed {
		return "", false
	}
	return m.selected, true
}
