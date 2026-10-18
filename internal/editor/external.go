package editor

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"go.kblin.dev/mibig-cli/internal/tree"
)

// rawDoneMsg reports that the external editor exited.
type rawDoneMsg struct {
	path     string // path of the edited node
	file     string // temporary file with the JSON
	original []byte
	err      error
}

// editorCommand returns the command to edit file with: the configured
// editor, $VISUAL, $EDITOR or vi. The editor may include arguments, e.g.
// "code --wait".
func editorCommand(editor, file string) (*exec.Cmd, error) {
	for _, candidate := range []string{editor, os.Getenv("VISUAL"), os.Getenv("EDITOR"), "vi"} {
		if args := strings.Fields(candidate); len(args) > 0 {
			return exec.Command(args[0], append(args[1:], file)...), nil
		}
	}
	return nil, fmt.Errorf("no editor configured")
}

// startRaw opens the JSON of a node in the external editor.
func (m *Model) startRaw(n *tree.Node) tea.Cmd {
	data, err := n.MarshalJSON()
	if err != nil {
		m.setStatus(statusError, "cannot encode %s: %v", displayPath(n.Path), err)
		return nil
	}
	data = append(data, '\n')
	f, err := os.CreateTemp("", "mibig-*.json")
	if err != nil {
		m.setStatus(statusError, "%v", err)
		return nil
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		m.setStatus(statusError, "%v", err)
		return nil
	}
	return m.execEditor(n.Path, f.Name(), data)
}

func (m *Model) execEditor(path, file string, original []byte) tea.Cmd {
	cmd, err := editorCommand(m.opts.Editor, file)
	if err != nil {
		os.Remove(file)
		m.setStatus(statusError, "%v", err)
		return nil
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return rawDoneMsg{path: path, file: file, original: original, err: err}
	})
}

// finishRaw applies the edited JSON. If it is invalid, it offers to edit it
// again, keeping the user's changes.
func (m *Model) finishRaw(msg rawDoneMsg) tea.Cmd {
	cleanup := func() { os.Remove(msg.file) }
	if msg.err != nil {
		cleanup()
		m.setStatus(statusError, "editor failed: %v", msg.err)
		return nil
	}
	data, err := os.ReadFile(msg.file)
	if err != nil {
		cleanup()
		m.setStatus(statusError, "%v", err)
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(msg.original)) {
		cleanup()
		m.setStatus(statusInfo, "no changes")
		return nil
	}
	n := m.tree.Node(msg.path)
	if n == nil {
		cleanup()
		m.setStatus(statusError, "%s no longer exists", displayPath(msg.path))
		return nil
	}
	var applyErr error
	m.mutate(func() (string, error) {
		applyErr = m.tree.Replace(n, data)
		return msg.path, applyErr
	})
	if applyErr != nil {
		m.prompt = &prompt{
			question:   fmt.Sprintf("Invalid JSON: %v. Edit again?", applyErr),
			defaultYes: true,
			onYes: func() tea.Cmd {
				return m.execEditor(msg.path, msg.file, msg.original)
			},
			onNo: func() tea.Cmd {
				cleanup()
				m.setStatus(statusInfo, "changes discarded")
				return nil
			},
		}
		return nil
	}
	cleanup()
	m.setStatus(statusSuccess, "updated %s", displayPath(msg.path))
	return nil
}
