package editor

import (
	"reflect"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"go.kblin.dev/mibig-cli/internal/tree"
)

// editState is an open edit form in the detail pane.
type editState struct {
	title string
	form  *huh.Form
	// sel is the select field of the form, if any, to tell whether esc
	// clears its filter or cancels the edit.
	sel   *huh.Select[string]
	value string
	// gen identifies the form, see formMsg.
	gen   int
	apply func(value string)
}

const (
	unsetOption = "\x00unset"
)

func (m *Model) newForm(field huh.Field) *huh.Form {
	keys := huh.NewDefaultKeyMap()
	// esc and ctrl+c are handled by the editor
	keys.Quit.SetEnabled(false)
	// the text field's own editor bypasses the configured one and does not
	// handle errors (Text.ExternalEditor(false) alone does not disable the
	// key in huh v2.0.3); e edits values as JSON instead
	keys.Text.Editor.SetEnabled(false)
	form := huh.NewForm(huh.NewGroup(field)).
		WithShowHelp(false).
		WithKeyMap(keys).
		WithTheme(huh.ThemeFunc(huh.ThemeCharm))
	return form
}

func (e *editState) resize(l layout) {
	e.form.WithWidth(l.detailWidth)
	// leave room for the error message below the fields
	e.form.WithHeight(max(l.mainHeight-formErrorLines, 1))
}

// formErrorLines are kept free for validation errors of forms.
const formErrorLines = 2

// startEdit opens the form for a scalar value: a selection for controlled
// fields and booleans, a text input otherwise.
func (m *Model) startEdit(n *tree.Node) tea.Cmd {
	path := n.Path
	// titles and descriptions are kept to one line each, so that the
	// input stays visible in small panes
	width := m.layout().detailWidth
	e := &editState{title: ansi.Truncate(oneLine(n.Label()), width-2, "…"), value: n.Text()}
	where := "in " + oneLine(displayPath(n.Parent.Path)) + " · "
	if !n.IsSet() {
		e.value = ""
	}
	original, wasSet := e.value, n.IsSet()
	// unset is the selection for "not set" in optional boolean forms
	unset := false
	e.apply = func(value string) {
		if unset && value == unsetOption {
			value = ""
		}
		if value == original && (wasSet || value == "") {
			m.setStatus(statusInfo, "unchanged")
			return
		}
		m.mutate(func() (string, error) {
			return path, m.tree.Node(path).SetText(value)
		})
	}

	var options []huh.Option[string]
	if vocab, ok := m.vocabs[path]; ok {
		if e.value != "" && !slices.Contains(vocab, e.value) {
			options = append(options, huh.NewOption(oneLine(e.value)+" (not in vocabulary)", e.value))
		}
		for _, value := range vocab {
			options = append(options, huh.NewOption(oneLine(value), value))
		}
		options = append(options, huh.NewOption("(none)", ""))
	} else if n.Kind == tree.Bool {
		options = []huh.Option[string]{huh.NewOption("true", "true"), huh.NewOption("false", "false")}
		if n.Optional {
			unset = true
			options = append(options, huh.NewOption("(not set)", unsetOption))
			if e.value == "" {
				e.value = unsetOption
			}
		}
	}

	var field huh.Field
	switch {
	case options != nil:
		e.sel = huh.NewSelect[string]().
			Title(e.title).
			Description(ansi.Truncate(where+"/ to filter", width-2, "…")).
			Options(options...).
			Value(&e.value)
		field = e.sel
	case n.Kind == tree.String && (strings.Contains(e.value, "\n") || multilineKeys[n.Key]):
		if !roundTrips(e.value, func(v *string) huh.Field { return huh.NewText().Value(v) }) {
			return m.startRawInstead(n)
		}
		field = huh.NewText().
			Title(e.title).
			Description(ansi.Truncate(where+"alt+enter for a new line", width-2, "…")).
			Lines(max(min(6, m.layout().mainHeight-formErrorLines-3), 1)).
			// its editor would bypass the configured one; e edits as JSON
			ExternalEditor(false).
			Value(&e.value)
	default:
		if !roundTrips(e.value, func(v *string) huh.Field { return huh.NewInput().Value(v) }) {
			return m.startRawInstead(n)
		}
		desc := n.Kind.String()
		if n.Optional {
			desc += ", leave empty to unset"
		}
		field = huh.NewInput().
			Title(e.title).
			Description(ansi.Truncate(where+desc, width-2, "…")).
			Placeholder("(empty)").
			Validate(n.ParseText).
			Value(&e.value)
	}
	return m.openForm(e, field)
}

// openForm shows the form of e with field. Every form gets a new
// generation, see formMsg.
func (m *Model) openForm(e *editState, field huh.Field) tea.Cmd {
	m.formGen++
	e.gen = m.formGen
	e.form = m.newForm(field)
	e.resize(m.layout())
	m.edit = e
	return e.wrap(e.form.Init())
}

// roundTrips reports whether a text field keeps value exactly. The input
// widgets drop some characters (e.g. control characters) and limit the
// length, so values they would change must not be edited with them:
// confirming the form unchanged would silently alter the value.
func roundTrips(value string, newField func(*string) huh.Field) bool {
	probe := value
	field := newField(&probe)
	field.Blur() // copies the widget's value to probe
	return probe == value
}

// startRawInstead edits a value as JSON that a form cannot represent.
func (m *Model) startRawInstead(n *tree.Node) tea.Cmd {
	cmd := m.startRaw(n)
	if m.statusKind != statusError {
		m.setStatus(statusInfo, "the value has characters the form cannot show; editing it as JSON")
	}
	return cmd
}

// multilineKeys are free text fields edited as multiple lines.
var multilineKeys = map[string]bool{"comment": true, "details": true, "description": true}

// startVariantSelect asks for the type of a new class, module or domain.
func (m *Model) startVariantSelect(title string, variants []string, apply func(string)) tea.Cmd {
	e := &editState{title: title, apply: apply}
	options := make([]huh.Option[string], len(variants))
	for i, v := range variants {
		options[i] = huh.NewOption(v, v)
	}
	e.sel = huh.NewSelect[string]().
		Title(title).
		Description("type / to filter").
		Options(options...).
		Value(&e.value)
	return m.openForm(e, e.sel)
}

func (m *Model) updateEdit(msg tea.Msg) tea.Cmd {
	e := m.edit
	if k, ok := msg.(tea.KeyPressMsg); ok {
		switch k.String() {
		case "ctrl+c":
			m.edit = nil
			m.setStatus(statusInfo, "edit cancelled")
			return nil
		case "esc":
			if e.sel == nil || !e.sel.GetFiltering() {
				m.edit = nil
				m.setStatus(statusInfo, "edit cancelled")
				return nil
			}
		}
	}
	model, cmd := e.form.Update(msg)
	if form, ok := model.(*huh.Form); ok {
		e.form = form
	}
	switch e.form.State {
	case huh.StateCompleted:
		m.edit = nil
		e.apply(e.value)
		return nil
	case huh.StateAborted:
		m.edit = nil
		m.setStatus(statusInfo, "edit cancelled")
		return nil
	}
	return e.wrap(cmd)
}

// formMsg is a message produced by a command of the form with generation
// gen. Messages of forms that are no longer open are dropped, so that e.g.
// a late "next field" message of a cancelled form cannot submit the next.
type formMsg struct {
	gen int
	msg tea.Msg
}

var (
	cmdType = reflect.TypeFor[tea.Cmd]()
	// teaPkg is the package of Bubble Tea's own messages, e.g. window size
	// requests, which are for the runtime and must not be wrapped.
	teaPkg = reflect.TypeFor[tea.QuitMsg]().PkgPath()
)

// wrap tags the messages of cmd, including those of batched and sequenced
// commands, with the form's generation.
func (e *editState) wrap(cmd tea.Cmd) tea.Cmd {
	if cmd == nil {
		return nil
	}
	gen := e.gen
	var wrapCmd func(tea.Cmd) tea.Cmd
	wrapCmd = func(cmd tea.Cmd) tea.Cmd {
		if cmd == nil {
			return nil
		}
		return func() tea.Msg {
			msg := cmd()
			switch msg := msg.(type) {
			case nil:
				return nil
			case tea.BatchMsg:
				cmds := make([]tea.Cmd, len(msg))
				for i, c := range msg {
					cmds[i] = wrapCmd(c)
				}
				return tea.BatchMsg(cmds)
			}
			// sequences are an unexported []tea.Cmd type
			if rv := reflect.ValueOf(msg); rv.Kind() == reflect.Slice && rv.Type().Elem() == cmdType {
				cmds := make([]tea.Cmd, rv.Len())
				for i := range cmds {
					cmds[i] = wrapCmd(rv.Index(i).Interface().(tea.Cmd))
				}
				return tea.Sequence(cmds...)()
			}
			if t := reflect.TypeOf(msg); t.PkgPath() == teaPkg || t.Kind() == reflect.Pointer && t.Elem().PkgPath() == teaPkg {
				return msg
			}
			return formMsg{gen: gen, msg: msg}
		}
	}
	return wrapCmd(cmd)
}

// prompt is a yes/no question in the status line.
type prompt struct {
	question   string
	defaultYes bool
	onYes      func() tea.Cmd
	onNo       func() tea.Cmd
}

func (m *Model) ask(question string, defaultYes bool, onYes func() tea.Cmd) {
	m.prompt = &prompt{question: question, defaultYes: defaultYes, onYes: onYes}
}

func (m *Model) updatePrompt(msg tea.KeyPressMsg) tea.Cmd {
	p := m.prompt
	yes := false
	switch msg.String() {
	case "y", "Y":
		yes = true
	case "n", "N", "esc", "ctrl+c", "q":
	case "enter":
		yes = p.defaultYes
	default:
		return nil
	}
	m.prompt = nil
	m.status = ""
	if yes {
		return p.onYes()
	}
	if p.onNo != nil {
		return p.onNo()
	}
	return nil
}
