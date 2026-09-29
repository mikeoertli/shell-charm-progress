package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestParseBarWidth(t *testing.T) {
	for _, test := range []struct {
		value string
		want  barWidthSpec
	}{
		{"auto", barWidthSpec{percent: 60}},
		{"60%", barWidthSpec{percent: 60}},
		{"1%", barWidthSpec{percent: 1}},
		{"100%", barWidthSpec{percent: 100}},
		{"3", barWidthSpec{cells: 3}},
		{"40", barWidthSpec{cells: 40}},
		{"300", barWidthSpec{cells: 300}},
		{"999999999", barWidthSpec{cells: 999999999}},
	} {
		got, err := parseBarWidth(test.value)
		if err != nil || got != test.want {
			t.Fatalf("%q: got %+v, %v", test.value, got, err)
		}
	}
	for _, value := range []string{"", "0", "2", "0%", "101%", "60%%", "-1", "-1%", "1.5%", "060%", "040", "+40", "60% ", "9999999999"} {
		if _, err := parseBarWidth(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}

func TestBarDoesNotChangeWithLabelOrCountDigits(t *testing.T) {
	for _, test := range []struct {
		spec string
		want int
	}{{"60%", 60}, {"75%", 75}, {"40", 40}, {"100%", 90}} {
		bar, _, err := newBar("", "")
		if err != nil {
			t.Fatal(err)
		}
		spec, err := parseBarWidth(test.spec)
		if err != nil {
			t.Fatal(err)
		}
		m := model{bar: bar, total: 100, width: 100, barWidth: spec}
		for _, count := range []int{0, 9, 10, 99, 100} {
			m.count = count
			for _, label := range []string{"", "Short", "Updating a repository with a very long name", strings.Repeat("项目 🐚", 80)} {
				m.label = label
				line := ansi.Strip(m.View().Content)
				// The bar stays at zero until its animation is driven; measure
				// its complete empty track while changing the displayed count.
				if got := strings.Count(line, "░"); got != test.want {
					t.Fatalf("%s count %d label %q: bar %d, want %d", test.spec, count, label, got, test.want)
				}
				if ansi.StringWidth(strings.TrimPrefix(line, "\n")) > 99 {
					t.Fatalf("overflow: %q", line)
				}
			}
		}
	}
}
