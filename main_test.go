package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestViewFitsEveryWidth(t *testing.T) {
	for _, label := range []string{"repo", "项目 🐚 é " + strings.Repeat("long", 100), "a\tline\nnext\x1b[2J"} {
		for width := 1; width <= 160; width++ {
			bar, accent, err := newBar("", "")
			if err != nil {
				t.Fatal(err)
			}
			m := model{bar: bar, accent: accent,
				total: 999999999, count: 123456789, width: width, label: labelText(label), finished: true}
			view := strings.Split(m.View().Content, "\n")
			if len(view) != 2 || view[0] != "" {
				t.Fatalf("expected blank separator and one progress line: %q", view)
			}
			if strings.ContainsAny(view[1], "\r\t") || ansi.StringWidth(view[1]) >= width {
				t.Fatalf("width %d: %q has width %d", width, view[1], ansi.StringWidth(view[1]))
			}
		}
	}
}

func TestProgressResizesWithoutCountUpdates(t *testing.T) {
	for _, label := range []string{"", "Updating repo", "项目 🐚 " + strings.Repeat("long", 50)} {
		bar, _, err := newBar("", "")
		if err != nil {
			t.Fatal(err)
		}
		m := model{bar: bar, total: 10, label: label}
		// Widen, shrink, then widen again without advancing progress.
		for _, width := range []int{40, 120, 20, 240} {
			updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
			m = updated.(model)
			line := strings.TrimPrefix(ansi.Strip(m.View().Content), "\n")
			if got := strings.Count(line, "░"); got != width*60/100 {
				t.Fatalf("terminal %d: bar width %d, want 60%%, %q", width, got, line)
			}
			if m.count != 0 || !strings.Contains(line, "0/10") {
				t.Fatalf("count lost: %q", line)
			}
		}
	}
}

func TestExplicitBarWidthRemainsACap(t *testing.T) {
	bar, _, err := newBar("", "")
	if err != nil {
		t.Fatal(err)
	}
	m := model{bar: bar, total: 10, barWidth: barWidthSpec{cells: 40}}
	for _, test := range []struct{ width, barWidth int }{{120, 40}, {20, 12}, {240, 40}} {
		updated, _ := m.Update(tea.WindowSizeMsg{Width: test.width, Height: 24})
		m = updated.(model)
		line := ansi.Strip(m.View().Content)
		if got := strings.Count(line, "░"); got != test.barWidth {
			t.Fatalf("terminal %d: bar width %d, want %d", test.width, got, test.barWidth)
		}
	}
}

func TestJournalPartialRecordsAndStopSnapshot(t *testing.T) {
	dir := t.TempDir()
	open := func(name string) *os.File {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_RDWR|os.O_CREATE, 0600)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { f.Close() })
		return f
	}
	logs, control := open("logs"), open("control")
	appendFile := func(name, value string) {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if _, err := f.WriteString(value); err != nil {
			t.Fatal(err)
		}
	}
	j := journal{logs: logs, control: control, remaining: -1, parent: os.Getpid()}
	appendFile("control", "label\x00a\t项目\n")
	appendFile("logs", "first\ntail")
	b := j.poll().(batch)
	if b.err != nil || len(b.records) != 0 || strings.Join(b.lines, "|") != "first" || b.done {
		t.Fatalf("first poll: %+v", b)
	}
	appendFile("control", "last\x00count\x001\x00stop\x00\x00")
	b = j.poll().(batch)
	if b.err != nil || len(b.records) != 3 || b.records[0].value != "a\t项目\nlast" || strings.Join(b.lines, "|") != "tail" || !b.done {
		t.Fatalf("stop poll: %+v", b)
	}
	appendFile("logs", "background output after stop")
	b = j.poll().(batch)
	if !b.done || len(b.lines) != 0 {
		t.Fatalf("snapshot changed: %+v", b)
	}
}

func TestLogControlsCannotMoveCursor(t *testing.T) {
	got := logLine("\x1b[31merror\x1b[0m\x1b[2J\rnext\tcolumn\a")
	if got != "error next column" {
		t.Fatalf("got %q", got)
	}
	if got := labelText("a\t项目\nlast"); got != "a 项目 last" {
		t.Fatal(got)
	}
}
