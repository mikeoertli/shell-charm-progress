package main

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestInitializationQuotesExecutable(t *testing.T) {
	path := "/tmp/a 'quote' $(printf INJECTED) `printf BAD`/shell-charm-progress"
	for _, shell := range []string{"bash", "zsh", "dash", "sh"} {
		t.Run(shell, func(t *testing.T) {
			if _, err := exec.LookPath(shell); err != nil {
				t.Skip("shell unavailable")
			}
			cmd := exec.Command(shell, "-c", initialization(path)+"\nprintf '%s' \"$_SCP_BIN\"")
			got, err := cmd.CombinedOutput()
			if err != nil || string(got) != path {
				t.Fatalf("got %q, error %v", got, err)
			}
		})
	}
}

func TestCLIHelpAndErrors(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"help"}, {"--version"}, {"init", "sh"}} {
		var out bytes.Buffer
		if err := cli(args, &out); err != nil || out.Len() == 0 {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"unknown"}, {"init", "fish"}, {"init", "sh", "extra"}, {"demo", "--oops"}, {"render"}} {
		if err := cli(args, &bytes.Buffer{}); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestThemeAndCustomColors(t *testing.T) {
	for _, pair := range [][2]string{{"", ""}, {"212", ""}, {"#7D56F4", "#FF87D7"}, {"", "42"}} {
		bar, accent, err := newBar(pair[0], pair[1])
		if err != nil {
			t.Fatal(err)
		}
		bar.SetWidth(20)
		view := bar.ViewAs(.5) + accent.Render("5/10")
		if !strings.Contains(view, "\x1b[") || ansi.StringWidth(view) != 24 {
			t.Fatalf("unstyled or wrong width: %q", view)
		}
	}
	for _, s := range []string{"red", "-1", "256", "#123", "#gggggg", "212\n", "1;2", "+1"} {
		if _, _, err := newBar(s, ""); err == nil {
			t.Fatalf("accepted color %q", s)
		}
	}
}
