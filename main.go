// The renderer owns terminal output while the calling shell runs its own work.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"

	"charm.land/bubbles/v2/progress"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

const chunkSize = 64 * 1024

type record struct{ key, value string }
type batch struct {
	records []record
	lines   []string
	done    bool
	err     error
}

// Journals avoid pipe backpressure and inherited-writer EOF deadlocks. Only
// one poll command is in flight; partial records remain buffered between polls.
type journal struct {
	logs, control  *os.File
	logBuf, ctlBuf []byte
	stopping       bool
	remaining      int64
	parent         int
	orphan         bool
}

func (j *journal) poll() tea.Msg {
	if !j.stopping {
		time.Sleep(25 * time.Millisecond)
	}
	b := batch{}
	data, err := io.ReadAll(io.LimitReader(j.control, chunkSize))
	if err != nil {
		b.err = err
		return b
	}
	j.ctlBuf = append(j.ctlBuf, data...)
	for {
		i := bytes.IndexByte(j.ctlBuf, 0)
		if i < 0 {
			break
		}
		k := bytes.IndexByte(j.ctlBuf[i+1:], 0)
		if k < 0 {
			break
		}
		end := i + 1 + k
		r := record{string(j.ctlBuf[:i]), string(j.ctlBuf[i+1 : end])}
		j.ctlBuf = j.ctlBuf[end+1:]
		b.records = append(b.records, r)
		if r.key == "stop" {
			j.stopping = true
		}
	}
	if syscall.Kill(j.parent, 0) == syscall.ESRCH || os.Getppid() == 1 {
		j.orphan, j.stopping = true, true
	}
	if j.stopping && j.remaining < 0 {
		info, err := j.logs.Stat()
		if err != nil {
			b.err = err
			return b
		}
		offset, err := j.logs.Seek(0, io.SeekCurrent)
		if err != nil {
			b.err = err
			return b
		}
		// Snapshot once: unrelated background writers cannot prolong shutdown.
		j.remaining = info.Size() - offset
	}
	limit := int64(chunkSize)
	if j.stopping {
		limit = min(limit, j.remaining)
	}
	data, err = io.ReadAll(io.LimitReader(j.logs, limit))
	if err != nil {
		b.err = err
		return b
	}
	if j.stopping {
		j.remaining -= int64(len(data))
		b.done = j.remaining == 0
	}
	j.logBuf = append(j.logBuf, data...)
	for {
		i := bytes.IndexByte(j.logBuf, '\n')
		if i < 0 {
			break
		}
		b.lines = append(b.lines, logLine(string(j.logBuf[:i])))
		j.logBuf = j.logBuf[i+1:]
	}
	// Bound memory for programs emitting very long unterminated lines.
	if len(j.logBuf) >= chunkSize || (b.done && len(j.logBuf) > 0) {
		b.lines = append(b.lines, logLine(string(j.logBuf)))
		j.logBuf = nil
	}
	return b
}

// Plain line output is the contract. Strip ANSI and replace cursor controls;
// external terminal programs must not compete with Bubble Tea's renderer.
func logLine(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' || r == '\r' {
			return ' '
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, ansi.Strip(s))
}

func labelText(s string) string {
	return strings.Join(strings.Fields(logLine(strings.ReplaceAll(s, "\n", " "))), " ")
}

type readyMsg struct{ err error }
type model struct {
	bar          progress.Model
	total, count int
	width        int
	label, dir   string
	journal      *journal
	finished     bool
	err          error
	profile      colorprofile.Profile
	barWidth     barWidthSpec
	accent       lipgloss.Style
	muted        lipgloss.Style
}

func (m model) Init() tea.Cmd {
	return func() tea.Msg {
		return readyMsg{os.WriteFile(filepath.Join(m.dir, "ready"), nil, 0600)}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case readyMsg:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		return m, m.journal.poll
	case tea.WindowSizeMsg:
		m.width = msg.Width
	case batch:
		if msg.err != nil {
			m.err = msg.err
			return m, tea.Quit
		}
		var animation tea.Cmd
		for _, r := range msg.records {
			switch r.key {
			case "label":
				m.label = labelText(r.value)
			case "count":
				n, err := strconv.Atoi(r.value)
				if err == nil && n >= 0 && n <= m.total {
					m.count = n
					animation = m.bar.SetPercent(float64(n) / float64(m.total))
				}
			}
		}
		var print tea.Cmd
		if len(msg.lines) > 0 {
			print = tea.Println(strings.Join(msg.lines, "\n"))
		}
		if msg.done {
			m.finished = true
			// Bubble Tea v2 clears its live view on exit. Commit the actual
			// final count to scrollback before quitting (never force 100%).
			// Println bypasses the live renderer's color conversion.
			var final bytes.Buffer
			writer := colorprofile.Writer{Forward: &final, Profile: m.profile}
			_, _ = writer.Write([]byte(m.View().Content))
			return m, tea.Sequence(print, tea.Println(final.String()), tea.Quit)
		}
		return m, tea.Batch(animation, tea.Sequence(print, m.journal.poll))
	case progress.FrameMsg:
		var cmd tea.Cmd
		m.bar, cmd = m.bar.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() tea.View {
	// Leave a spare column to avoid terminal autowrap at the right margin.
	w := max(0, m.width-1)
	// Reserve the final count's space so digit changes cannot move the bar.
	count := fmt.Sprintf("%*d/%d", len(strconv.Itoa(m.total)), m.count, m.total)
	text := m.accent.Render(count)
	if w >= len(count)+5 {
		// Choose the bar's width independently of status text. Labels use the
		// remaining space and truncate rather than make the bar jump around.
		barWidth := min(m.barWidth.columns(m.width), w-len(count)-2)
		labelWidth := max(0, w-barWidth-len(count)-5)
		label := ""
		if m.label != "" && labelWidth > 0 {
			clipped := ansi.Truncate(m.label, labelWidth, "…")
			label = m.muted.Render(" · ") + clipped
		}
		m.bar.SetWidth(barWidth)
		bar := m.bar.View()
		if m.finished {
			bar = m.bar.ViewAs(float64(m.count) / float64(m.total))
		}
		text = bar + "  " + m.accent.Render(count) + label
	}
	// Keep the separator in the live view: updates must not accumulate blank
	// lines in scrollback. The same separator is preserved in the final print.
	v := tea.NewView("\n" + ansi.Truncate(text, w, "…"))
	v.DisableBracketedPasteMode = true
	return v
}

func runRenderer(args []string) error {
	flags := flag.NewFlagSet("render", flag.ContinueOnError)
	dir := flags.String("session", "", "private session directory")
	total := flags.Int("total", 0, "positive item count")
	parent := flags.Int("parent", 0, "owning shell PID")
	noColor := flags.Bool("no-color", false, "disable color")
	width := flags.String("width", "60%", "bar width as terminal percentage or column count")
	start := flags.String("color", "", "fill color")
	end := flags.String("color-end", "", "gradient end color")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 0 {
		return fmt.Errorf("unexpected render argument %q", flags.Arg(0))
	}
	if *dir == "" || *total <= 0 || *total > 999999999 || *parent <= 1 {
		return fmt.Errorf("session, positive total, and parent PID are required")
	}
	barWidth, err := parseBarWidth(*width)
	if err != nil {
		return err
	}
	bar, accent, err := newBar(*start, *end)
	if err != nil {
		return err
	}
	logs, err := os.Open(filepath.Join(*dir, "logs"))
	if err != nil {
		return err
	}
	defer logs.Close()
	control, err := os.Open(filepath.Join(*dir, "control"))
	if err != nil {
		return err
	}
	defer control.Close()
	// The shell handles interruptions and sends stop after restoring its streams.
	// Do not race it by quitting on the process group's Ctrl-C.
	signal.Ignore(syscall.SIGINT, syscall.SIGTERM)
	j := &journal{logs: logs, control: control, remaining: -1, parent: *parent}
	profile := colorprofile.Detect(os.Stdout, os.Environ())
	if *noColor || os.Getenv("NO_COLOR") != "" {
		profile = colorprofile.ASCII
	}
	if profile == colorprofile.ASCII {
		// Half blocks rely on background color to fill the other half. Use
		// whole blocks when color is disabled so the bar remains solid.
		bar.Full = progress.DefaultFullCharFullBlock
	}
	m := model{bar: bar, accent: accent, muted: lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		total: *total, dir: *dir, journal: j, profile: profile, barWidth: barWidth, width: 80}
	opts := []tea.ProgramOption{tea.WithInput(nil), tea.WithOutput(os.Stdout),
		tea.WithoutSignalHandler(), tea.WithColorProfile(profile)}
	result, err := tea.NewProgram(m, opts...).Run()
	if err != nil {
		return err
	}
	final := result.(model)
	if final.err != nil {
		return final.err
	}
	if j.orphan {
		// Only remove files owned by this session, never recursively delete a
		// directory supplied to the internal render command.
		for _, name := range []string{"logs", "control", "ready", "renderer-error"} {
			_ = os.Remove(filepath.Join(*dir, name))
		}
		return os.Remove(*dir)
	}
	if !final.finished {
		return fmt.Errorf("renderer stopped before draining output")
	}
	return os.WriteFile(filepath.Join(*dir, "finished"), nil, 0600)
}

func main() {
	if err := cli(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "shell-charm-progress:", err)
		os.Exit(1)
	}
}
