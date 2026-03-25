package manage

import "strings"

// Action identifies a top-level operator action in the Valv manage screen.
type Action string

const (
	ActionStatus       Action = "status"
	ActionProfiles     Action = "profiles"
	ActionUpdate       Action = "update"
	ActionCleanup      Action = "cleanup"
	ActionGlobalSwitch Action = "global-switch"
)

func (a Action) String() string {
	return string(a)
}

// Item is one entry in the manage TUI list.
type Item struct {
	action      Action
	title       string
	description string
}

// NewItem constructs a manage list item.
func NewItem(action Action, title, description string) Item {
	return Item{
		action:      action,
		title:       title,
		description: description,
	}
}

// Action returns the item's action identifier.
func (i Item) Action() Action {
	return i.action
}

// FilterValue implements list.Item.
func (i Item) FilterValue() string {
	return strings.Join([]string{i.action.String(), i.title, i.description}, " ")
}

// Title implements list.DefaultItem.
func (i Item) Title() string {
	return i.title
}

// Description implements list.DefaultItem.
func (i Item) Description() string {
	return i.description
}

// ActionSelectedMsg is emitted when the user chooses a list entry.
type ActionSelectedMsg struct {
	Action Action
	Item   Item
}

// DefaultItems returns the canonical manage home actions.
func DefaultItems() []Item {
	return []Item{
		NewItem(ActionStatus, "Status", "Inspect the current project binding and runtime state."),
		NewItem(ActionProfiles, "Profiles", "Select and bind an existing provider profile."),
		NewItem(ActionUpdate, "Update", "Rebuild provider client images and rotate runtimes."),
		NewItem(ActionCleanup, "Cleanup", "Prune stale containers, caches, and local state."),
		NewItem(ActionGlobalSwitch, "Global switch", "Convenience host-global auth switcher."),
	}
}
