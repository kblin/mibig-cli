package editor

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	mibig "go.kblin.dev/go-mibig"

	"go.kblin.dev/mibig-cli/internal/tree"
)

// layout holds the sizes of the panes, without their borders.
type layout struct {
	treeWidth, detailWidth int
	mainHeight             int
	treeHeight             int
	issuesHeight           int
	issuesShown            bool
	// tooSmall is set if the terminal cannot fit the editor.
	tooSmall bool
}

const (
	minWidth = 60
	// the main panes must fit an edit form: title, description, input and
	// an error message
	minMainHeight = 6
)

func (m *Model) layout() layout {
	var l layout
	helpHeight := lipgloss.Height(m.viewHelp())
	// header, status line, help, borders of the main panes
	rest := m.height - 2 - helpHeight - 2
	if len(m.issues) > 0 {
		// the issues pane gets what the main panes can spare
		h := min(len(m.issues), max(3, rest/4), 10, rest-2-minMainHeight)
		if h >= 1 {
			l.issuesShown = true
			l.issuesHeight = h
			rest -= h + 2
		}
	}
	l.mainHeight = rest
	l.treeHeight = max(rest, 1)
	l.treeWidth = max(min(m.width*45/100, 70), 24) - 2
	l.detailWidth = m.width - l.treeWidth - 4
	l.tooSmall = m.width < minWidth || l.mainHeight < minMainHeight
	return l
}

// View implements tea.Model.
func (m *Model) View() tea.View {
	if m.quitting {
		return tea.NewView("")
	}
	l := m.layout()
	if l.tooSmall {
		msg := fmt.Sprintf("The terminal is too small for the editor (at least %d columns needed).", minWidth)
		if m.help.ShowAll {
			msg += " Press ? to hide the key list."
		}
		v := tea.NewView(clip(msg, m.width, m.height, ""))
		v.AltScreen = true
		return v
	}
	treePane, detailPane := paneStyle, paneStyle
	issuesPane := paneStyle
	if m.focus == focusTree && m.edit == nil {
		treePane = focusedPane
	}
	if m.edit != nil {
		detailPane = focusedPane
	}
	if m.focus == focusIssues {
		issuesPane = focusedPane
	}

	// Height is a minimum, so content is clipped to keep the status line,
	// prompts and help on screen.
	main := lipgloss.JoinHorizontal(lipgloss.Top,
		treePane.Width(l.treeWidth+2).Height(l.mainHeight+2).Render(clip(m.viewTree(l), l.treeWidth, l.mainHeight, "")),
		detailPane.Width(l.detailWidth+2).Height(l.mainHeight+2).Render(clip(m.viewDetail(l), l.detailWidth, l.mainHeight, m.detailMore())),
	)
	parts := []string{m.viewHeader(), main}
	if l.issuesShown {
		parts = append(parts, issuesPane.Width(m.width).Height(l.issuesHeight+2).Render(clip(m.viewIssues(l), m.width-2, l.issuesHeight, "")))
	}
	parts = append(parts, m.viewStatus(), m.viewHelp())

	v := tea.NewView(lipgloss.JoinVertical(lipgloss.Left, parts...))
	v.AltScreen = true
	v.WindowTitle = "mibig edit " + oneLine(filepath.Base(m.opts.Path))
	return v
}

func (m *Model) viewHeader() string {
	e := m.entry
	var parts []string
	parts = append(parts, headerStyle.Render("MIBiG"))
	name := e.AccessionVersion()
	if e.Accession == "" {
		name = "(no accession)"
	}
	parts = append(parts, lipgloss.NewStyle().Bold(true).Render(oneLine(name)))
	parts = append(parts, dimStyle.Render(oneLine(filepath.Base(m.opts.Path))))
	parts = append(parts, "quality "+oneLine(string(e.Quality)))
	check := "checking at " + string(m.quality())
	if m.target != "" {
		check = warningStyle.Render(check)
	}
	parts = append(parts, check)

	errs, warns := len(m.issues.Errors()), len(m.issues.Warnings())
	switch {
	case errs == 0 && warns == 0:
		parts = append(parts, okStyle.Render("✓ valid"))
	default:
		var c []string
		if errs > 0 {
			c = append(c, errorStyle.Render(fmt.Sprintf("● %d errors", errs)))
		}
		if warns > 0 {
			c = append(c, warningStyle.Render(fmt.Sprintf("▲ %d warnings", warns)))
		}
		parts = append(parts, strings.Join(c, " "))
	}
	if m.Dirty() {
		parts = append(parts, promptStyle.Render("modified"))
	}
	return ansi.Truncate(strings.Join(parts, dimStyle.Render(" · ")), m.width, "…")
}

func (m *Model) viewTree(l layout) string {
	var lines []string
	end := min(m.offset+l.treeHeight, len(m.rows))
	for _, n := range m.rows[m.offset:end] {
		lines = append(lines, m.treeLine(n, l.treeWidth))
	}
	return strings.Join(lines, "\n")
}

func (m *Model) treeLine(n *tree.Node, width int) string {
	selected := n.Path == m.cursor
	c := m.counts[n]
	badge, badgeStyle := "  ", lipgloss.NewStyle()
	switch {
	case c.errors > 0:
		badge, badgeStyle = "● ", errorStyle
	case c.warnings > 0:
		badge, badgeStyle = "▲ ", warningStyle
	}

	marker := "  "
	if n.IsContainer() {
		switch {
		case len(n.Children) == 0:
			marker = "◦ "
		case m.expanded[n.Path]:
			marker = "▾ "
		default:
			marker = "▸ "
		}
	}
	indent := strings.Repeat("  ", n.Depth())
	label := oneLine(n.Label())
	value := oneLine(n.Summary())
	if n.IsScalar() && !n.IsSet() {
		value = "—"
	}
	sep := ": "
	if n.IsContainer() || n.Kind == tree.Absent || n.Kind == tree.Raw {
		sep = "  "
	}

	if selected {
		plain := badge + indent + marker + label + sep + value
		return selectedStyle.Width(width).Render(ansi.Truncate(plain, width, "…"))
	}
	valueStyle := lipgloss.NewStyle()
	if !n.IsScalar() || !n.IsSet() {
		valueStyle = dimStyle
	}
	line := badgeStyle.Render(badge) + indent + dimStyle.Render(marker) +
		keyStyle.Render(label) + sep + valueStyle.Render(value)
	return ansi.Truncate(line, width, "…")
}

func (m *Model) viewDetail(l layout) string {
	if m.edit != nil {
		return m.edit.form.View()
	}
	n := m.selected()
	w := lipgloss.NewStyle().Width(l.detailWidth)
	var b strings.Builder

	b.WriteString(titleStyle.Render(ansi.Wrap(oneLine(displayPath(n.Path)), l.detailWidth, ".[")) + "\n")
	desc := n.Kind.String()
	if n.Unknown {
		desc = "unknown key, not part of the schema"
	}
	if n.Optional {
		desc = "optional " + desc
	}
	if n.Variant != "" {
		desc += " · " + oneLine(n.Variant)
	}
	if vocab, ok := m.vocabs[n.Path]; ok {
		desc += fmt.Sprintf(" · one of %d values", len(vocab))
	}
	b.WriteString(dimStyle.Render(desc) + "\n\n")

	switch {
	case n.IsScalar():
		if n.IsSet() {
			b.WriteString(w.Render(showControls(n.Text())) + "\n")
		} else {
			b.WriteString(dimStyle.Render("(empty)") + "\n")
		}
	case n.Kind == tree.Raw:
		data, err := n.MarshalJSON()
		if err == nil {
			b.WriteString(w.Render(ansi.Truncate(string(data), l.detailWidth*4, "…")) + "\n")
		}
	default:
		b.WriteString(dimStyle.Render(oneLine(n.Summary())) + "\n")
	}

	if issues := m.nodeIssues[n]; len(issues) > 0 {
		b.WriteString("\n" + lipgloss.NewStyle().Bold(true).Render("Issues") + "\n")
		for _, issue := range issues {
			b.WriteString(w.Render(issueLine(issue, false)) + "\n")
		}
	}
	if c := m.counts[n]; n.IsContainer() && (c.errors+c.warnings) > len(m.nodeIssues[n]) {
		b.WriteString(dimStyle.Render(fmt.Sprintf("\n%d errors, %d warnings inside", c.errors, c.warnings)) + "\n")
	}

	b.WriteString("\n" + dimStyle.Render(m.actions(n)))
	return b.String()
}

// actions lists the keys that apply to the node.
func (m *Model) actions(n *tree.Node) string {
	var acts []string
	switch {
	case n.IsScalar():
		acts = append(acts, "enter edit")
	case n.Kind == tree.Absent:
		acts = append(acts, "a create")
	case n.Kind == tree.Raw:
		acts = append(acts, "enter edit JSON")
	}
	if n.Kind == tree.List {
		acts = append(acts, "a add element")
	} else if n.Index >= 0 {
		acts = append(acts, "a add after", "K/J move")
	}
	if n.CanRemove() {
		if n.Index >= 0 {
			acts = append(acts, "d delete")
		} else {
			acts = append(acts, "d clear")
		}
	}
	if n.Kind != tree.Absent {
		acts = append(acts, "e edit JSON")
	}
	return strings.Join(acts, " · ")
}

func issueLine(issue mibig.Issue, withPath bool) string {
	mark := warningStyle.Render("▲")
	if issue.Severity == mibig.SeverityError {
		mark = errorStyle.Render("●")
	}
	text := oneLine(issue.Message)
	if withPath {
		text = keyStyle.Render(oneLine(displayPath(issue.Path))) + ": " + text
	}
	return mark + " " + text + dimStyle.Render(" ["+string(issue.Code)+"]")
}

func (m *Model) viewIssues(l layout) string {
	var lines []string
	end := min(m.issueOffset+l.issuesHeight, len(m.issues))
	for i := m.issueOffset; i < end; i++ {
		line := issueLine(m.issues[i], true)
		if m.focus == focusIssues && i == m.issueCursor {
			plain := ansi.Strip(line)
			line = selectedStyle.Width(m.width - 2).Render(ansi.Truncate(plain, m.width-2, "…"))
		} else {
			line = ansi.Truncate(line, m.width-2, "…")
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) viewStatus() string {
	if m.prompt != nil {
		q := m.prompt.question
		if m.prompt.defaultYes {
			q += " [Y/n]"
		} else {
			q += " [y/N]"
		}
		return ansi.Truncate(promptStyle.Render(oneLine(q)), m.width, "…")
	}
	style := dimStyle
	switch m.statusKind {
	case statusError:
		style = errorStyle
	case statusSuccess:
		style = okStyle
	}
	return ansi.Truncate(style.Render(oneLine(m.status)), m.width, "…")
}

func (m *Model) viewHelp() string {
	text := dimStyle.Render("enter confirm · esc cancel")
	if m.edit == nil {
		text = m.help.View(m.keys)
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, m.width, "…")
	}
	return strings.Join(lines, "\n")
}

// clip wraps text to width and cuts it to at most height lines, replacing
// the last line with the marker more, if not empty. Pane styles only set minimum sizes, so content must
// fit before it is rendered.
func clip(text string, width, height int, more string) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(lipgloss.NewStyle().Width(width).Render(text), "\n")
	if len(lines) > height {
		lines = lines[:height]
		if more != "" {
			lines[height-1] = dimStyle.Render(ansi.Truncate(more, width, "…"))
		}
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n")
}

// oneLine makes text safe for single-line display: line breaks and other
// control characters, e.g. in values, are shown as symbols.
func oneLine(text string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\r':
			return '⏎'
		case r == '\t':
			return '→'
		case r < 0x20 || r == 0x7f:
			return '·'
		}
		return r
	}, text)
}

// showControls is oneLine for text that may span lines: line breaks are
// kept, other control characters (e.g. terminal escapes) are made visible.
func showControls(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = oneLine(line)
	}
	return strings.Join(lines, "\n")
}

// detailMore is the marker for clipped details.
func (m *Model) detailMore() string {
	if m.edit != nil {
		return ""
	}
	return "… more (e: view as JSON)"
}

// displayPath returns a path for display; the root is "entry".
func displayPath(path string) string {
	if path == "" {
		return "entry"
	}
	return path
}
