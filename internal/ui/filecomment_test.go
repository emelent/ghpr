package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"ghpr/internal/gh"
)

func TestFileCommentInput(t *testing.T) {
	m := newTestModel(t)
	m.cursor = 5
	m.handleKey(tea.KeyPressMsg{Code: 'w', Text: "w"})
	if m.overlay != overlayInput || m.inKind != inputFileComment || m.inFilePath != "main.go" {
		t.Fatalf("file comment input not opened: overlay=%v kind=%v path=%q", m.overlay, m.inKind, m.inFilePath)
	}
	if !strings.Contains(ansi.Strip(m.View().Content), "New comment on the file main.go") {
		t.Fatalf("title missing")
	}
}

func TestFileCommentFromFileList(t *testing.T) {
	m := newTestModel(t)
	m.selectFile(1)
	m.filesFocused = true
	m.handleKey(tea.KeyPressMsg{Code: 'c', Text: "c"})
	if m.overlay != overlayInput || m.inKind != inputFileComment || m.inFilePath != "b.py" {
		t.Fatalf("file comment input not opened from the file list: kind=%v path=%q", m.inKind, m.inFilePath)
	}
}

func TestFileThreadShownAtTop(t *testing.T) {
	m := newTestModel(t)
	threads := append(m.threads, gh.Thread{ID: "TF", Path: "main.go", OnFile: true, DiffSide: "RIGHT",
		Comments: []gh.Comment{{DatabaseID: 9, Author: "erin", Body: "whole file remark", CreatedAt: time.Now()}}})
	m.Update(threadsMsg{threads: threads})
	if r := m.rows[0]; r.kind != rowThread || r.thread.ID != "TF" {
		t.Fatalf("file thread not first: %+v", r)
	}
	for _, r := range m.rows {
		if r.kind == rowNote && strings.Contains(r.note, "outdated") {
			t.Fatalf("file thread counted as outdated")
		}
	}
	plain := ansi.Strip(m.View().Content)
	if !strings.Contains(plain, "· file") || !strings.Contains(plain, "whole file remark") {
		t.Fatalf("file thread not rendered:\n%s", plain)
	}
	m.cursor = 0
	m.handleKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	if m.inKind != inputReply || !strings.Contains(m.inputTitle, "Reply to @erin on main.go") || strings.Contains(m.inputTitle, ":0") {
		t.Fatalf("reply title: %q", m.inputTitle)
	}
}
