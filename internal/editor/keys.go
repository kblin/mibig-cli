package editor

import "charm.land/bubbles/v2/key"

type keyMap struct {
	Up       key.Binding
	Down     key.Binding
	PageUp   key.Binding
	PageDown key.Binding
	Expand   key.Binding
	Collapse key.Binding
	Edit     key.Binding
	Add      key.Binding
	Remove   key.Binding
	MoveUp   key.Binding
	MoveDown key.Binding
	Raw      key.Binding
	RawAll   key.Binding
	Undo     key.Binding
	Redo     key.Binding
	Focus    key.Binding
	Next     key.Binding
	Prev     key.Binding
	Quality  key.Binding
	Save     key.Binding
	Help     key.Binding
	Quit     key.Binding
}

func defaultKeys() keyMap {
	return keyMap{
		Up:       key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "up")),
		Down:     key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "down")),
		PageUp:   key.NewBinding(key.WithKeys("pgup", "ctrl+u"), key.WithHelp("pgup", "page up")),
		PageDown: key.NewBinding(key.WithKeys("pgdown", "ctrl+d"), key.WithHelp("pgdn", "page down")),
		Expand:   key.NewBinding(key.WithKeys("right", "l"), key.WithHelp("→/l", "expand")),
		Collapse: key.NewBinding(key.WithKeys("left", "h"), key.WithHelp("←/h", "collapse")),
		Edit:     key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "edit/toggle")),
		Add:      key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add/create")),
		Remove:   key.NewBinding(key.WithKeys("d", "delete"), key.WithHelp("d", "delete/clear")),
		MoveUp:   key.NewBinding(key.WithKeys("K", "shift+up"), key.WithHelp("K", "move up")),
		MoveDown: key.NewBinding(key.WithKeys("J", "shift+down"), key.WithHelp("J", "move down")),
		Raw:      key.NewBinding(key.WithKeys("e"), key.WithHelp("e", "edit JSON in $EDITOR")),
		RawAll:   key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "edit entry JSON")),
		Undo:     key.NewBinding(key.WithKeys("u", "ctrl+z"), key.WithHelp("u", "undo")),
		Redo:     key.NewBinding(key.WithKeys("ctrl+r", "U"), key.WithHelp("ctrl+r", "redo")),
		Focus:    key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "tree/issues")),
		Next:     key.NewBinding(key.WithKeys("n"), key.WithHelp("n/N", "next/prev issue")),
		Prev:     key.NewBinding(key.WithKeys("N")),
		Quality:  key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "validate at level")),
		Save:     key.NewBinding(key.WithKeys("ctrl+s", "w"), key.WithHelp("w", "save")),
		Help:     key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "more keys")),
		Quit:     key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
	}
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Edit, k.Add, k.Remove, k.Raw, k.Next, k.Save, k.Help, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.PageUp, k.PageDown, k.Expand, k.Collapse},
		{k.Edit, k.Add, k.Remove, k.MoveUp, k.MoveDown, k.Raw, k.RawAll},
		{k.Undo, k.Redo, k.Focus, k.Next, k.Quality},
		{k.Save, k.Help, k.Quit},
	}
}
