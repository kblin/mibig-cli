// Package editor implements an interactive terminal editor for MIBiG
// entries: a tree of the entry on the left, details and edit forms for the
// selected value on the right, and the validation issues below.
package editor

import (
	"bytes"
	"fmt"
	"path/filepath"
	"slices"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	mibig "go.kblin.dev/go-mibig"

	"go.kblin.dev/mibig-cli/internal/tree"
)

// Options configure the editor.
type Options struct {
	// Path is the file the entry is read from and saved to.
	Path string
	// Editor is the command used to edit values as JSON, e.g. "vim" or
	// "code --wait". If empty, $VISUAL, $EDITOR or vi is used.
	Editor string
	// ValidateAt is the quality level to validate at initially; if empty,
	// the entry's own level is used.
	ValidateAt mibig.Quality
}

type focusArea int

const (
	focusTree focusArea = iota
	focusIssues
)

type statusKind int

const (
	statusInfo statusKind = iota
	statusSuccess
	statusError
)

// issueCount counts the issues of a node and its descendants.
type issueCount struct {
	errors, warnings int
}

// Model is the Bubble Tea model of the editor.
type Model struct {
	opts  Options
	entry *mibig.Entry
	tree  *tree.Tree

	vocabs map[string][]string
	issues mibig.Issues
	// nodeIssues holds the issues at each node; issues about values that do
	// not exist are attached to the closest existing ancestor.
	nodeIssues map[*tree.Node][]mibig.Issue
	counts     map[*tree.Node]issueCount
	// target is the quality level to validate at, "" for the entry's own.
	target mibig.Quality

	expanded map[string]bool
	cursor   string
	rows     []*tree.Node
	offset   int

	focus       focusArea
	issueCursor int
	issueOffset int

	edit    *editState
	formGen int
	prompt  *prompt

	// current is the encoded entry, saved the encoded entry as last saved;
	// undo and redo hold earlier and later versions.
	current []byte
	saved   []byte
	undo    [][]byte
	redo    [][]byte

	status     string
	statusKind statusKind

	width, height int
	keys          keyMap
	help          help.Model
	quitting      bool
}

// New returns an editor for the entry, which was read from opts.Path.
func New(entry *mibig.Entry, opts Options) (*Model, error) {
	data, err := mibig.Marshal(entry)
	if err != nil {
		return nil, err
	}
	if opts.ValidateAt != "" && !opts.ValidateAt.Valid() {
		return nil, fmt.Errorf("unknown quality level %q", opts.ValidateAt)
	}
	m := &Model{
		opts:     opts,
		entry:    entry,
		target:   opts.ValidateAt,
		expanded: map[string]bool{},
		current:  data,
		saved:    data,
		keys:     defaultKeys(),
		help:     help.New(),
		width:    80,
		height:   24,
	}
	m.refresh("")
	if len(m.rows) > 0 {
		m.cursor = m.rows[0].Path
	}
	return m, nil
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd {
	return nil
}

// Dirty reports whether there are unsaved changes.
func (m *Model) Dirty() bool {
	return !bytes.Equal(m.current, m.saved)
}

// quality returns the level the entry is validated at.
func (m *Model) quality() mibig.Quality {
	if m.target != "" {
		return m.target
	}
	if m.entry.Quality.Valid() {
		return m.entry.Quality
	}
	return mibig.QualityLow
}

// refresh rebuilds the tree and validates the entry after a change, and
// selects the node at cursor (or its closest ancestor) if cursor is set.
func (m *Model) refresh(cursor string) {
	m.tree = tree.Build(m.entry)
	m.vocabs = m.entry.Vocabularies()
	m.issues = m.entry.ValidateAt(m.quality())

	m.nodeIssues = map[*tree.Node][]mibig.Issue{}
	m.counts = map[*tree.Node]issueCount{}
	for _, issue := range m.issues {
		n := m.tree.Nearest(issue.Path)
		m.nodeIssues[n] = append(m.nodeIssues[n], issue)
		for p := n; p != nil; p = p.Parent {
			c := m.counts[p]
			if issue.Severity == mibig.SeverityError {
				c.errors++
			} else {
				c.warnings++
			}
			m.counts[p] = c
		}
	}
	m.issueCursor = min(m.issueCursor, max(len(m.issues)-1, 0))
	m.scrollIssues()
	if m.focus == focusIssues && !m.layout().issuesShown {
		m.focus = focusTree
	}

	if cursor == "" {
		cursor = m.cursor
	}
	m.selectPath(cursor)
}

// selectPath moves the cursor to the node at path, or its closest existing
// ancestor, and expands its ancestors.
func (m *Model) selectPath(path string) {
	n := m.tree.Nearest(path)
	if n == m.tree.Root && len(n.Children) > 0 {
		n = n.Children[0]
	}
	for p := n.Parent; p != nil; p = p.Parent {
		m.expanded[p.Path] = true
	}
	m.cursor = n.Path
	m.buildRows()
}

// buildRows flattens the visible part of the tree.
func (m *Model) buildRows() {
	m.rows = m.rows[:0]
	var add func(n *tree.Node)
	add = func(n *tree.Node) {
		for _, child := range n.Children {
			m.rows = append(m.rows, child)
			if m.expanded[child.Path] {
				add(child)
			}
		}
	}
	add(m.tree.Root)
	m.scrollTree()
}

// selected returns the node at the cursor.
func (m *Model) selected() *tree.Node {
	if n := m.tree.Node(m.cursor); n != nil {
		return n
	}
	return m.tree.Root
}

func (m *Model) cursorRow() int {
	for i, n := range m.rows {
		if n.Path == m.cursor {
			return i
		}
	}
	return 0
}

func (m *Model) moveCursor(delta int) {
	if len(m.rows) == 0 {
		return
	}
	i := min(max(m.cursorRow()+delta, 0), len(m.rows)-1)
	m.cursor = m.rows[i].Path
	m.scrollTree()
}

func (m *Model) scrollTree() {
	h := m.layout().treeHeight
	i := m.cursorRow()
	if i < m.offset {
		m.offset = i
	}
	if i >= m.offset+h {
		m.offset = i - h + 1
	}
	m.offset = max(0, min(m.offset, len(m.rows)-h))
}

func (m *Model) setStatus(kind statusKind, format string, args ...any) {
	m.status = fmt.Sprintf(format, args...)
	m.statusKind = kind
}

// mutate applies a change to the entry. fn returns the path to select
// afterwards ("" to keep the selection). Changes are recorded for undo.
func (m *Model) mutate(fn func() (string, error)) {
	cursor, err := fn()
	if err != nil {
		// a failed change must not leave the entry half changed
		m.restore(m.current, "")
		m.setStatus(statusError, "%v", err)
		return
	}
	data, err := mibig.Marshal(m.entry)
	if err != nil {
		m.restore(m.current, "")
		m.setStatus(statusError, "cannot encode entry: %v", err)
		return
	}
	if !bytes.Equal(data, m.current) {
		m.undo = append(m.undo, m.current)
		m.redo = nil
		m.current = data
	}
	m.status = ""
	m.refresh(cursor)
}

// restore replaces the entry with an encoded version of it.
func (m *Model) restore(data []byte, cursor string) {
	entry, err := mibig.Parse(data)
	if err != nil {
		// cannot happen for data encoded by Marshal
		m.setStatus(statusError, "cannot restore entry: %v", err)
		return
	}
	m.entry = entry
	m.current = data
	m.refresh(cursor)
}

func (m *Model) undoChange() {
	if len(m.undo) == 0 {
		m.setStatus(statusInfo, "nothing to undo")
		return
	}
	prev := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]
	m.redo = append(m.redo, m.current)
	m.restore(prev, "")
	m.setStatus(statusInfo, "undone (%d more)", len(m.undo))
}

func (m *Model) redoChange() {
	if len(m.redo) == 0 {
		m.setStatus(statusInfo, "nothing to redo")
		return
	}
	next := m.redo[len(m.redo)-1]
	m.redo = m.redo[:len(m.redo)-1]
	m.undo = append(m.undo, m.current)
	m.restore(next, "")
	m.setStatus(statusInfo, "redone")
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.help.SetWidth(msg.Width)
		m.scrollTree()
		m.scrollIssues()
		if m.focus == focusIssues && !m.layout().issuesShown {
			m.focus = focusTree
		}
		if m.edit != nil {
			m.edit.resize(m.layout())
		}
		return m, nil
	case rawDoneMsg:
		return m, m.finishRaw(msg)
	}

	if fm, ok := msg.(formMsg); ok {
		if m.edit == nil || fm.gen != m.edit.gen {
			return m, nil // from a form that is no longer open
		}
		msg = fm.msg
	}
	if m.edit != nil {
		return m, m.updateEdit(msg)
	}

	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.prompt != nil {
		return m, m.updatePrompt(keyMsg)
	}
	m.status = ""

	// global keys
	switch {
	case key.Matches(keyMsg, m.keys.Quit):
		return m, m.quit()
	case key.Matches(keyMsg, m.keys.Save):
		return m, m.save()
	case key.Matches(keyMsg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
		m.scrollTree()
		return m, nil
	case key.Matches(keyMsg, m.keys.Quality):
		m.cycleQuality()
		return m, nil
	case key.Matches(keyMsg, m.keys.Undo):
		m.undoChange()
		return m, nil
	case key.Matches(keyMsg, m.keys.Redo):
		m.redoChange()
		return m, nil
	case key.Matches(keyMsg, m.keys.Next):
		m.jumpIssue(1)
		return m, nil
	case key.Matches(keyMsg, m.keys.Prev):
		m.jumpIssue(-1)
		return m, nil
	case key.Matches(keyMsg, m.keys.Focus):
		if m.focus == focusTree && m.layout().issuesShown {
			m.focus = focusIssues
		} else {
			m.focus = focusTree
		}
		return m, nil
	}

	if m.focus == focusIssues {
		return m, m.updateIssues(keyMsg)
	}
	return m, m.updateTree(keyMsg)
}

func (m *Model) updateTree(msg tea.KeyPressMsg) tea.Cmd {
	n := m.selected()
	h := m.layout().treeHeight
	switch {
	case key.Matches(msg, m.keys.Up):
		m.moveCursor(-1)
	case key.Matches(msg, m.keys.Down):
		m.moveCursor(1)
	case key.Matches(msg, m.keys.PageUp):
		m.moveCursor(-h)
	case key.Matches(msg, m.keys.PageDown):
		m.moveCursor(h)
	case key.Matches(msg, m.keys.Expand):
		switch {
		case n.IsContainer() && len(n.Children) > 0 && !m.expanded[n.Path]:
			m.expanded[n.Path] = true
			m.buildRows()
		case m.expanded[n.Path] && len(n.Children) > 0:
			m.cursor = n.Children[0].Path
			m.scrollTree()
		}
	case key.Matches(msg, m.keys.Collapse):
		if m.expanded[n.Path] && len(n.Children) > 0 {
			m.expanded[n.Path] = false
			m.buildRows()
		} else if n.Parent != nil && n.Parent != m.tree.Root {
			m.cursor = n.Parent.Path
			m.scrollTree()
		}
	case key.Matches(msg, m.keys.Edit):
		switch {
		case n.IsContainer():
			if len(n.Children) == 0 {
				return m.add(n)
			}
			m.expanded[n.Path] = !m.expanded[n.Path]
			m.buildRows()
		case n.IsScalar():
			return m.startEdit(n)
		case n.Kind == tree.Absent:
			return m.add(n)
		case n.Kind == tree.Raw:
			return m.startRaw(n)
		}
	case key.Matches(msg, m.keys.Add):
		return m.add(n)
	case key.Matches(msg, m.keys.Remove):
		m.remove(n)
	case key.Matches(msg, m.keys.MoveUp):
		m.move(n, -1)
	case key.Matches(msg, m.keys.MoveDown):
		m.move(n, 1)
	case key.Matches(msg, m.keys.Raw):
		if n.Kind == tree.Absent {
			return m.add(n)
		}
		return m.startRaw(n)
	case key.Matches(msg, m.keys.RawAll):
		return m.startRaw(m.tree.Root)
	}
	return nil
}

func (m *Model) updateIssues(msg tea.KeyPressMsg) tea.Cmd {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.issueCursor = max(m.issueCursor-1, 0)
	case key.Matches(msg, m.keys.Down):
		m.issueCursor = min(m.issueCursor+1, max(len(m.issues)-1, 0))
	case key.Matches(msg, m.keys.PageUp):
		m.issueCursor = max(m.issueCursor-m.layout().issuesHeight, 0)
	case key.Matches(msg, m.keys.PageDown):
		m.issueCursor = min(m.issueCursor+m.layout().issuesHeight, max(len(m.issues)-1, 0))
	case key.Matches(msg, m.keys.Edit):
		if len(m.issues) > 0 {
			m.selectPath(m.issues[m.issueCursor].Path)
			m.focus = focusTree
		}
	case msg.String() == "esc":
		m.focus = focusTree
	}
	m.scrollIssues()
	return nil
}

func (m *Model) scrollIssues() {
	h := max(m.layout().issuesHeight, 1)
	if m.issueCursor < m.issueOffset {
		m.issueOffset = m.issueCursor
	}
	if m.issueCursor >= m.issueOffset+h {
		m.issueOffset = m.issueCursor - h + 1
	}
	m.issueOffset = max(0, min(m.issueOffset, len(m.issues)-h))
}

// jumpIssue selects the next or previous issue and its node.
func (m *Model) jumpIssue(delta int) {
	if len(m.issues) == 0 {
		m.setStatus(statusSuccess, "no issues at quality %s", m.quality())
		return
	}
	m.issueCursor = (m.issueCursor + delta + len(m.issues)) % len(m.issues)
	issue := m.issues[m.issueCursor]
	m.selectPath(issue.Path)
	m.scrollIssues()
	m.setStatus(statusInfo, "issue %d/%d: %s", m.issueCursor+1, len(m.issues), issue.Message)
}

// qualityCycle is the order the validation level cycles through; "" is the
// entry's own level.
var qualityCycle = []mibig.Quality{"", mibig.QualityQuestionable, mibig.QualityLow, mibig.QualityMedium, mibig.QualityHigh}

func (m *Model) cycleQuality() {
	i := slices.Index(qualityCycle, m.target)
	m.target = qualityCycle[(i+1)%len(qualityCycle)]
	m.refresh("")
	if m.target == "" {
		m.setStatus(statusInfo, "validating at the entry's own quality (%s)", m.quality())
	} else {
		m.setStatus(statusInfo, "validating at quality %s", m.target)
	}
}

// add creates an absent value, or adds an element to a list (or after a list
// element), asking for the variant of classes, modules and domains.
func (m *Model) add(n *tree.Node) tea.Cmd {
	if n.CanCreate() {
		create := func(variant string) {
			path := n.Path
			m.mutate(func() (string, error) {
				return path, m.tree.Node(path).Create(variant)
			})
			m.expanded[path] = true
			m.buildRows()
		}
		if variants := n.Variants(); len(variants) > 0 {
			return m.startVariantSelect("Type of "+n.Label(), variants, create)
		}
		create("")
		return nil
	}
	if !n.CanAdd() {
		m.setStatus(statusInfo, "%s is not a list", n.Label())
		return nil
	}
	list, at := n.AddTarget()
	listPath := list.Path
	var newPath string
	add := func(variant string) {
		m.mutate(func() (string, error) {
			var err error
			newPath, err = m.tree.Node(listPath).Add(at, variant)
			return newPath, err
		})
		m.expanded[listPath] = true
		m.expanded[newPath] = true
		m.buildRows()
	}
	if variants := list.Variants(); len(variants) > 0 {
		return m.startVariantSelect("Type of new element", variants, add)
	}
	add("")
	// go straight to editing new single values, e.g. a new citation
	if n := m.tree.Node(newPath); n != nil && n.IsScalar() && m.status == "" {
		return m.startEdit(n)
	}
	return nil
}

func (m *Model) remove(n *tree.Node) {
	if !n.CanRemove() {
		m.setStatus(statusInfo, "nothing to delete")
		return
	}
	path := n.Path
	do := func() tea.Cmd {
		m.mutate(func() (string, error) {
			return path, m.tree.Node(path).Remove()
		})
		return nil
	}
	if n.IsScalar() {
		do()
		return
	}
	what := "Delete " + displayPath(path)
	if n.Index < 0 {
		what = "Clear " + displayPath(path)
	}
	if c := len(n.Children); c > 0 {
		what += fmt.Sprintf(" (%d children)", c)
	}
	m.ask(what+"?", false, do)
}

func (m *Model) move(n *tree.Node, delta int) {
	if n.Index < 0 {
		m.setStatus(statusInfo, "only list elements can be moved")
		return
	}
	path := n.Path
	wasExpanded := m.expanded[path]
	var newPath string
	m.mutate(func() (string, error) {
		var err error
		newPath, err = m.tree.Node(path).Move(delta)
		return newPath, err
	})
	if newPath != path {
		m.expanded[newPath], m.expanded[path] = wasExpanded, m.expanded[newPath]
		m.buildRows()
	}
}

// save writes the entry, asking for confirmation if it has errors.
func (m *Model) save() tea.Cmd {
	write := func() tea.Cmd {
		if err := mibig.WriteFile(m.opts.Path, m.entry); err != nil {
			m.setStatus(statusError, "save failed: %v", err)
			return nil
		}
		m.saved = m.current
		m.setStatus(statusSuccess, "saved %s", filepath.Base(m.opts.Path))
		return nil
	}
	if errs := m.issues.Errors(); len(errs) > 0 {
		m.ask(fmt.Sprintf("The entry has %d validation errors at quality %s. Save anyway?", len(errs), m.quality()), false, write)
		return nil
	}
	return write()
}

func (m *Model) quit() tea.Cmd {
	if !m.Dirty() {
		m.quitting = true
		return tea.Quit
	}
	m.ask("Discard unsaved changes and quit?", false, func() tea.Cmd {
		m.quitting = true
		return tea.Quit
	})
	return nil
}

// Run runs the editor until the user quits.
func Run(entry *mibig.Entry, opts Options) error {
	m, err := New(entry, opts)
	if err != nil {
		return err
	}
	_, err = tea.NewProgram(m).Run()
	return err
}
