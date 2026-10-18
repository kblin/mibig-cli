package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	mibig "go.kblin.dev/go-mibig"
)

// newTestModel opens a copy of a test entry.
func newTestModel(t *testing.T) (*Model, string) {
	t.Helper()
	data, err := os.ReadFile("testdata/BGC0000370.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "BGC0000370.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := mibig.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(entry, Options{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	return m, path
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "ctrl+u":
		return tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
	}
	r := []rune(k)
	if len(r) != 1 {
		panic("unknown key " + k)
	}
	msg := tea.KeyPressMsg{Code: r[0], Text: k}
	if r[0] >= 'A' && r[0] <= 'Z' {
		msg.Code = r[0] + 'a' - 'A'
		msg.ShiftedCode = r[0]
		msg.Mod = tea.ModShift
	}
	return msg
}

// press sends keys to the model and runs the resulting commands, feeding
// their messages back, like the Bubble Tea runtime does. Commands that do
// not return quickly (timers such as cursor blinking) are dropped.
func press(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		send(m, keyMsg(k), 0)
	}
}

func typeText(m *Model, text string) {
	for _, r := range text {
		send(m, tea.KeyPressMsg{Code: r, Text: string(r)}, 0)
	}
}

func send(m *Model, msg tea.Msg, depth int) {
	if depth > 20 {
		return
	}
	_, cmd := m.Update(msg)
	run(m, cmd, depth)
}

func run(m *Model, cmd tea.Cmd, depth int) {
	if cmd == nil {
		return
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(50 * time.Millisecond):
		return
	}
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			run(m, c, depth+1)
		}
	case tea.QuitMsg:
	default:
		// sequences are an unexported []tea.Cmd type
		if rv := reflect.ValueOf(msg); rv.Kind() == reflect.Slice && rv.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
			for i := range rv.Len() {
				run(m, rv.Index(i).Interface().(tea.Cmd), depth+1)
			}
			return
		}
		send(m, msg, depth+1)
	}
}

func TestEditText(t *testing.T) {
	m, _ := newTestModel(t)
	m.selectPath("comment")
	press(t, m, "enter")
	if m.edit == nil {
		t.Fatal("no edit form")
	}
	press(t, m, "ctrl+u")
	typeText(m, "checked by test")
	press(t, m, "enter")
	if m.edit != nil {
		t.Fatalf("form still open; status %q", m.status)
	}
	if m.entry.Comment != "checked by test" {
		t.Fatalf("comment = %q", m.entry.Comment)
	}
	if !m.Dirty() {
		t.Error("not dirty after an edit")
	}

	press(t, m, "u")
	if m.entry.Comment == "checked by test" || m.Dirty() {
		t.Errorf("undo: comment = %q, dirty %v", m.entry.Comment, m.Dirty())
	}
	send(m, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}, 0)
	if m.entry.Comment != "checked by test" {
		t.Errorf("redo: comment = %q", m.entry.Comment)
	}
}

func TestEditCancel(t *testing.T) {
	m, _ := newTestModel(t)
	m.selectPath("comment")
	before := m.entry.Comment
	press(t, m, "enter")
	typeText(m, "xyz")
	press(t, m, "esc")
	if m.edit != nil || m.entry.Comment != before {
		t.Errorf("esc did not cancel: %q", m.entry.Comment)
	}
}

func TestEditInvalidNumber(t *testing.T) {
	m, _ := newTestModel(t)
	m.selectPath("version")
	press(t, m, "enter", "ctrl+u")
	typeText(m, "abc")
	press(t, m, "enter")
	if m.edit == nil {
		t.Fatal("invalid number accepted")
	}
	press(t, m, "ctrl+u")
	typeText(m, "9")
	press(t, m, "enter")
	if m.edit != nil || m.entry.Version != 9 {
		t.Errorf("version = %d, form open %v", m.entry.Version, m.edit != nil)
	}
}

func TestEditVocabulary(t *testing.T) {
	m, _ := newTestModel(t)
	path := "loci[0].evidence[0].method"
	if m.tree.Node(path) == nil {
		t.Fatalf("no %s", path)
	}
	m.selectPath(path)
	before := m.tree.Node(path).Text()
	press(t, m, "enter")
	if m.edit == nil || m.edit.sel == nil {
		t.Fatal("no select form for a controlled field")
	}
	press(t, m, "down", "enter")
	after := m.tree.Node(path).Text()
	if after == before {
		t.Errorf("value unchanged: %q", after)
	}
	if !contains(m.vocabs[path], after) {
		t.Errorf("value %q not in vocabulary", after)
	}
}

func TestAddClass(t *testing.T) {
	m, _ := newTestModel(t)
	n := len(m.entry.Biosynthesis.Classes)
	m.selectPath("biosynthesis.classes")
	press(t, m, "a")
	if m.edit == nil || m.edit.sel == nil {
		t.Fatal("no variant selection")
	}
	press(t, m, "/")
	typeText(m, "terpene")
	press(t, m, "enter", "enter")
	if got := len(m.entry.Biosynthesis.Classes); got != n+1 {
		t.Fatalf("%d classes, want %d (status %q)", got, n+1, m.status)
	}
	if c := m.entry.Biosynthesis.Classes[n]; c.ClassName() != mibig.ClassTerpene {
		t.Errorf("new class is %s", c.ClassName())
	}
	if want := "biosynthesis.classes[" + itoa(n) + "]"; m.cursor != want {
		t.Errorf("cursor at %s, want %s", m.cursor, want)
	}
}

func TestDeleteWithPrompt(t *testing.T) {
	m, _ := newTestModel(t)
	n := len(m.entry.Compounds)
	m.selectPath("compounds[0]")
	press(t, m, "d")
	if m.prompt == nil {
		t.Fatal("no confirmation")
	}
	press(t, m, "n")
	if len(m.entry.Compounds) != n {
		t.Fatal("deleted despite no")
	}
	press(t, m, "d", "y")
	if len(m.entry.Compounds) != n-1 {
		t.Errorf("%d compounds, want %d", len(m.entry.Compounds), n-1)
	}
}

func TestSaveWithErrors(t *testing.T) {
	m, path := newTestModel(t)
	m.selectPath("accession")
	press(t, m, "enter", "ctrl+u")
	typeText(m, "broken")
	press(t, m, "enter")
	if !m.issues.HasErrors() {
		t.Fatal("expected a validation error")
	}
	press(t, m, "w")
	if m.prompt == nil {
		t.Fatal("no confirmation for saving with errors")
	}
	press(t, m, "y")
	if m.Dirty() {
		t.Fatalf("still dirty after save; status %q", m.status)
	}
	saved, err := mibig.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Accession != "broken" {
		t.Errorf("saved accession %q", saved.Accession)
	}
}

func TestQuitWithChanges(t *testing.T) {
	m, _ := newTestModel(t)
	press(t, m, "q")
	if !m.quitting {
		t.Fatal("clean editor did not quit")
	}

	m, _ = newTestModel(t)
	m.selectPath("comment")
	press(t, m, "enter", "ctrl+u")
	typeText(m, "x")
	press(t, m, "enter", "q")
	if m.quitting || m.prompt == nil {
		t.Fatal("quit without confirmation")
	}
	press(t, m, "y")
	if !m.quitting {
		t.Error("did not quit after confirmation")
	}
}

func TestIssueNavigation(t *testing.T) {
	m, _ := newTestModel(t)
	m.selectPath("accession")
	press(t, m, "enter", "ctrl+u")
	typeText(m, "broken")
	press(t, m, "enter")
	m.selectPath("comment")
	for range m.issues {
		press(t, m, "n")
		if m.cursor == "accession" {
			return
		}
	}
	t.Errorf("n never reached the accession issue; issues %v", m.issues)
}

func TestCycleQuality(t *testing.T) {
	m, _ := newTestModel(t)
	seen := map[mibig.Quality]bool{}
	for range qualityCycle {
		press(t, m, "v")
		seen[m.quality()] = true
	}
	if len(seen) != 4 {
		t.Errorf("levels seen: %v", seen)
	}
}

func TestRawEdit(t *testing.T) {
	m, _ := newTestModel(t)
	file := filepath.Join(t.TempDir(), "raw.json")
	if err := os.WriteFile(file, []byte(`"edited as JSON"`), 0o600); err != nil {
		t.Fatal(err)
	}
	m.finishRaw(rawDoneMsg{path: "comment", file: file, original: []byte(`""`)})
	if m.entry.Comment != "edited as JSON" {
		t.Errorf("comment = %q, status %q", m.entry.Comment, m.status)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("temporary file not removed")
	}

	// invalid JSON offers to edit again and keeps the file
	if err := os.WriteFile(file, []byte(`{"class": `), 0o600); err != nil {
		t.Fatal(err)
	}
	m.finishRaw(rawDoneMsg{path: "biosynthesis.classes[0]", file: file, original: []byte(`{}`)})
	if m.prompt == nil {
		t.Fatalf("no prompt for invalid JSON; status %q", m.status)
	}
	if _, err := os.Stat(file); err != nil {
		t.Error("file removed before the user decided")
	}
	press(t, m, "n")
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Error("file not removed after discarding")
	}
}

func TestViewSizes(t *testing.T) {
	m, _ := newTestModel(t)
	m.expanded["loci"] = true
	m.buildRows()
	for _, size := range [][2]int{{120, 40}, {80, 24}, {40, 10}, {10, 3}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		v := m.View()
		if v.Content == "" {
			t.Errorf("%v: empty view", size)
		}
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	press(t, m, "?")
	_ = m.View()
	m.selectPath("comment")
	press(t, m, "enter")
	if !strings.Contains(m.View().Content, "comment") {
		t.Error("edit form not shown")
	}
}

func TestViewFitsTerminal(t *testing.T) {
	m, _ := newTestModel(t)
	m.entry.Accession = strings.Repeat("BGC0000370-", 200)
	m.refresh("accession")
	check := func(what string, w, h int) {
		t.Helper()
		lines := strings.Split(m.View().Content, "\n")
		if len(lines) > h {
			t.Errorf("%s %dx%d: view has %d lines", what, w, h, len(lines))
		}
		for i, line := range lines {
			if lw := lipgloss.Width(line); lw > w {
				t.Errorf("%s %dx%d: line %d is %d wide", what, w, h, i, lw)
				break
			}
		}
	}
	sizes := [][2]int{{120, 40}, {80, 24}, {60, 15}, {60, 12}, {40, 10}, {10, 3}}
	for _, size := range sizes {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		check("tree", size[0], size[1])
		press(t, m, "?")
		check("full help", size[0], size[1])
		press(t, m, "?")
	}

	// edit forms, with a long title, keep the input visible
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.selectPath("biosynthesis.modules[0].name")
	if m.selected().Path != "biosynthesis.modules[0].name" {
		t.Fatalf("selected %s", m.selected().Path)
	}
	press(t, m, "enter")
	if m.edit == nil {
		t.Fatal("no form")
	}
	for _, size := range sizes {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		check("form", size[0], size[1])
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if v := m.View().Content; !strings.Contains(v, ">") && !strings.Contains(v, m.selected().Text()) {
		t.Errorf("input not visible:\n%s", v)
	}
}

func TestAddCoreSequence(t *testing.T) {
	m, _ := newTestModel(t)
	m.entry.Biosynthesis.Classes = mibig.Classes{&mibig.RibosomalClass{Precursors: []mibig.RiPPPrecursor{{}}}}
	m.refresh("biosynthesis.classes[0].precursors[0].core_sequence")
	press(t, m, "a")
	if m.edit == nil {
		t.Fatalf("no edit form for the new sequence; status %q", m.status)
	}
	typeText(m, "MKV")
	press(t, m, "enter")
	class := m.entry.Biosynthesis.Classes[0].(*mibig.RibosomalClass)
	if got := class.Precursors[0].CoreSequence.Sequences; len(got) != 1 || got[0] != "MKV" {
		t.Errorf("sequences = %q", got)
	}
}

func TestFormErrorVisible(t *testing.T) {
	m, _ := newTestModel(t)
	m.selectPath("version")
	press(t, m, "enter", "ctrl+u")
	typeText(m, "abc")
	press(t, m, "enter")
	if m.edit == nil {
		t.Fatal("invalid number accepted")
	}
	if v := m.View().Content; !strings.Contains(v, "not a whole number") {
		t.Errorf("error not shown:\n%s", v)
	}
}

func TestFormInputVisibleInSmallTerminal(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	m.target = mibig.QualityHigh
	m.refresh("")
	path := "biosynthesis.modules[0].name"
	m.selectPath(path)
	press(t, m, "enter")
	if m.edit == nil || m.layout().tooSmall {
		t.Fatalf("no form (too small: %v)", m.layout().tooSmall)
	}
	v := m.View().Content
	if !strings.Contains(v, "> ") || !strings.Contains(v, m.tree.Node(path).Text()) {
		t.Errorf("input not visible:\n%s", v)
	}
}

func TestMultilineValues(t *testing.T) {
	m, _ := newTestModel(t)
	m.entry.Accession = "bad\naccession\nlines"
	m.entry.Comment = "two\nlines"
	m.refresh("comment")
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if lines := strings.Count(m.View().Content, "\n") + 1; lines > 24 {
		t.Errorf("view has %d lines", lines)
	}
}

func TestIssueScrollAfterRefresh(t *testing.T) {
	m, _ := newTestModel(t)
	m.target = mibig.QualityHigh
	m.refresh("")
	if len(m.issues) < 2 {
		t.Skip("test entry has too few issues at quality high")
	}
	m.focus = focusIssues
	m.issueCursor = len(m.issues) - 1
	m.scrollIssues()
	m.target = mibig.QualityQuestionable
	m.refresh("")
	if m.issueOffset > m.issueCursor || (len(m.issues) > 0 && m.issueOffset >= len(m.issues)) {
		t.Errorf("offset %d, cursor %d, %d issues", m.issueOffset, m.issueCursor, len(m.issues))
	}
	if !m.layout().issuesShown && m.focus == focusIssues {
		t.Error("focus left on hidden issues pane")
	}
}

func TestMultilineCommentUnchanged(t *testing.T) {
	m, _ := newTestModel(t)
	m.entry.Comment = "first\nsecond"
	data, _ := mibig.Marshal(m.entry)
	m.current, m.saved = data, data
	m.refresh("comment")
	press(t, m, "enter")
	if m.edit == nil {
		t.Fatal("no form")
	}
	press(t, m, "enter", "enter")
	if m.entry.Comment != "first\nsecond" || m.Dirty() {
		t.Errorf("comment = %q, dirty %v", m.entry.Comment, m.Dirty())
	}
}

func TestControlCharactersUseRawEditor(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	m, _ := newTestModel(t)
	m.entry.Comment = "a\tb"
	m.refresh("comment")
	if cmd := m.startEdit(m.selected()); cmd == nil || m.edit != nil {
		t.Errorf("value with a tab opened a form (cmd %v)", cmd != nil)
	}
}

func TestEscapesInValuesAreShown(t *testing.T) {
	m, _ := newTestModel(t)
	m.entry.Comment = "visible\x1b[8mhidden"
	m.refresh("comment")
	if v := m.View().Content; strings.Contains(v, "\x1b[8m") {
		t.Error("escape sequence from the value reached the terminal")
	}
}

func TestLongInputError(t *testing.T) {
	m, _ := newTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	m.selectPath("version")
	press(t, m, "enter", "ctrl+u")
	typeText(m, strings.Repeat("a", 80))
	press(t, m, "enter")
	if v := m.View().Content; !strings.Contains(v, "not a whole number") {
		t.Errorf("error not shown:\n%s", v)
	}
}

func TestResizeMovesFocusOffHiddenIssues(t *testing.T) {
	m, _ := newTestModel(t)
	m.target = mibig.QualityHigh
	m.refresh("")
	if !m.layout().issuesShown {
		t.Skip("no issues shown")
	}
	m.focus = focusIssues
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	if !m.layout().issuesShown && m.focus != focusTree {
		t.Error("focus left on hidden issues pane")
	}
}

func TestValuesFormsCannotKeepUseRawEditor(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())
	for name, value := range map[string]string{
		"NEL":         "before\u0085after",
		"replacement": "before�after",
		"CRLF":        "a\r\nb",
		"many lines":  strings.Repeat("x\n", 10001),
	} {
		m, _ := newTestModel(t)
		m.entry.Comment = value
		m.refresh("comment")
		cmd := m.startEdit(m.selected())
		if m.edit != nil || cmd == nil {
			t.Errorf("%s: opened a form", name)
		}
		if m.entry.Comment != value {
			t.Errorf("%s: value changed", name)
		}
	}
	// ordinary values still use forms
	m, _ := newTestModel(t)
	m.entry.Comment = "first\nsecond"
	m.refresh("comment")
	if m.startEdit(m.selected()); m.edit == nil {
		t.Error("ordinary multiline comment did not open a form")
	}
}

func TestTextFormHasNoOwnEditor(t *testing.T) {
	m, _ := newTestModel(t)
	t.Setenv("TMPDIR", "/nonexistent/mibig-test")
	m.selectPath("comment")
	press(t, m, "enter")
	if m.edit == nil {
		t.Fatal("no form")
	}
	send(m, tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}, 0) // must not panic
	if m.edit == nil {
		t.Error("form closed")
	}
}

func TestStaleFormMessagesDropped(t *testing.T) {
	m, _ := newTestModel(t)
	m.selectPath("comment")
	press(t, m, "enter")
	if m.edit == nil {
		t.Fatal("no comment form")
	}
	// confirm the comment form, but deliver its command late
	_, late := m.Update(keyMsg("enter"))
	if m.edit != nil && m.edit.form.State == 0 {
		press(t, m, "esc")
	}
	m.selectPath("accession")
	press(t, m, "enter")
	if m.edit == nil {
		t.Fatal("no accession form")
	}
	before := m.entry.Accession
	typeText(m, "X")
	run(m, late, 0)
	if m.edit == nil || m.entry.Accession != before {
		t.Errorf("late message of the old form submitted the new one: accession %q", m.entry.Accession)
	}
}

func TestEscapesInSummaries(t *testing.T) {
	m, _ := newTestModel(t)
	m.entry.Compounds[0].Name = "visible\x1b[8mhidden"
	m.refresh("compounds[0]")
	if strings.Contains(m.View().Content, "\x1b[8m") {
		t.Error("escape sequence from a summary reached the terminal")
	}
}

func TestUnsetSentinelOnlyForBooleans(t *testing.T) {
	m, _ := newTestModel(t)
	m.entry.Quality = "\x00unset"
	data, _ := mibig.Marshal(m.entry)
	m.current, m.saved = data, data
	m.refresh("quality")
	press(t, m, "enter", "enter")
	if m.entry.Quality != "\x00unset" || m.Dirty() {
		t.Errorf("quality = %q, dirty %v", m.entry.Quality, m.Dirty())
	}
}

func TestStaleVariantPickerMessagesDropped(t *testing.T) {
	m, _ := newTestModel(t)
	n := len(m.entry.Biosynthesis.Classes)
	m.selectPath("biosynthesis.classes")
	press(t, m, "a")
	if m.edit == nil {
		t.Fatal("no picker")
	}
	first := m.edit.gen
	_, late := m.Update(keyMsg("enter"))
	if m.edit != nil {
		press(t, m, "esc")
	}
	n = len(m.entry.Biosynthesis.Classes)
	m.selectPath("biosynthesis.classes")
	press(t, m, "a")
	if m.edit == nil || m.edit.gen == first {
		t.Fatalf("picker reuses generation %d", first)
	}
	run(m, late, 0)
	if m.edit == nil || len(m.entry.Biosynthesis.Classes) != n {
		t.Errorf("late message of the old picker submitted the new one")
	}
}

func TestWindowTitleSanitized(t *testing.T) {
	m, _ := newTestModel(t)
	m.opts.Path = filepath.Join(t.TempDir(), "evil\x07\x1b[2J.json")
	if title := m.View().WindowTitle; strings.ContainsAny(title, "\x07\x1b") {
		t.Errorf("window title %q has control characters", title)
	}
}

func TestRuntimeMessagesNotWrapped(t *testing.T) {
	e := &editState{gen: 1}
	msg := e.wrap(func() tea.Msg { return tea.RequestWindowSize() })()
	if _, ok := msg.(formMsg); ok {
		t.Error("Bubble Tea's own message was wrapped")
	}
	msg = e.wrap(func() tea.Msg { return struct{}{} })()
	if fm, ok := msg.(formMsg); !ok || fm.gen != 1 {
		t.Errorf("form message not wrapped: %#v", msg)
	}
}
