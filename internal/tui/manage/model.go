package manage

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/list"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	defaultWidth  = 80
	defaultHeight = 24
	layoutMargin  = 6
)

// Model is the Bubble Tea home screen for Valv operator workflows.
type Model struct {
	list      list.Model
	styles    Styles
	status    string
	lastFocus Action
	confirmed bool
}

// New returns a model seeded with the canonical Valv manage actions.
func New() Model {
	return NewWithItems(DefaultItems())
}

// NewWithItems returns a model seeded with the provided items.
func NewWithItems(items []Item) Model {
	if len(items) == 0 {
		items = DefaultItems()
	}

	delegate := list.NewDefaultDelegate()
	delegate.ShowDescription = true
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(lipgloss.Color("#EAF6FF"))
	delegate.Styles.NormalDesc = delegate.Styles.NormalDesc.Foreground(lipgloss.Color("#93A8B7"))
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(lipgloss.Color("#9DDCFF")).
		BorderLeftForeground(lipgloss.Color("#9DDCFF"))
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Foreground(lipgloss.Color("#CFEFFF"))
	delegate.Styles.DimmedTitle = delegate.Styles.DimmedTitle.Foreground(lipgloss.Color("#6E8291"))
	delegate.Styles.DimmedDesc = delegate.Styles.DimmedDesc.Foreground(lipgloss.Color("#6E8291"))

	menu := list.New(itemsAsList(items), delegate, defaultWidth, defaultHeight)
	menu.Title = "Actions"
	menu.SetShowFilter(false)
	menu.SetShowStatusBar(true)
	menu.SetShowPagination(false)
	menu.SetShowHelp(true)
	menu.SetFilteringEnabled(false)
	menu.SetStatusBarItemName("action", "actions")

	return Model{
		list:   menu,
		styles: DefaultStyles(),
	}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update handles Bubble Tea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.list.SetSize(msg.Width, max(1, msg.Height-layoutMargin))
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "enter" && !m.list.SettingFilter() {
			if item, ok := m.focusedItem(); ok {
				m.lastFocus = item.Action()
				m.confirmed = true
				m.status = fmt.Sprintf("%s selected", strings.ToLower(item.Title()))
				return m, tea.Quit
			}
		}
	}

	nextList, cmd := m.list.Update(msg)
	m.list = nextList
	return m, cmd
}

// View renders the current screen.
func (m Model) View() tea.View {
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		m.styles.header(),
		m.styles.Frame.Render(m.list.View()),
		m.styles.footer(m.footerStatus()),
	)
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "Valv Manage"
	return view
}

// CurrentAction returns the action currently focused in the list.
func (m Model) CurrentAction() (Action, bool) {
	item, ok := m.focusedItem()
	if !ok {
		return "", false
	}
	return item.Action(), true
}

// Notice returns the last rendered status hint.
func (m Model) Notice() string {
	return m.status
}

// Selected returns the action confirmed by the user, if any.
func (m Model) Selected() (Action, bool) {
	if !m.confirmed {
		return "", false
	}
	return m.lastFocus, true
}

func (m Model) focusedItem() (Item, bool) {
	selected := m.list.SelectedItem()
	if selected == nil {
		return Item{}, false
	}
	item, ok := selected.(Item)
	return item, ok
}

func (m Model) footerStatus() string {
	if strings.TrimSpace(m.status) != "" {
		return m.status
	}
	if action, ok := m.CurrentAction(); ok {
		return fmt.Sprintf("Focused action: %s", strings.ToLower(action.String()))
	}
	return ""
}

func itemsAsList(items []Item) []list.Item {
	out := make([]list.Item, 0, len(items))
	for _, item := range items {
		out = append(out, item)
	}
	return out
}
