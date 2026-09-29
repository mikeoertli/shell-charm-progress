package main

import (
	"fmt"
	"regexp"
	"strconv"

	"charm.land/bubbles/v2/progress"
	"charm.land/lipgloss/v2"
)

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func validColor(s string) bool {
	if hexColor.MatchString(s) {
		return true
	}
	if len(s) < 1 || len(s) > 3 {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(s)
	return err == nil && n >= 0 && n <= 255
}

func newBar(start, end string) (progress.Model, lipgloss.Style, error) {
	// Gum uses ANSI pink 212 for its spinner; blend purple 99 into that pink.
	if start == "" {
		start = "99"
		if end == "" {
			end = "212"
		}
	}
	for _, c := range []string{start, end} {
		if c != "" && !validColor(c) {
			return progress.Model{}, lipgloss.Style{}, fmt.Errorf("invalid color %q: use #RRGGBB or 0–255", c)
		}
	}
	colors := progress.WithColors(lipgloss.Color(start))
	accent := start
	if end != "" {
		colors = progress.WithColors(lipgloss.Color(start), lipgloss.Color(end))
		accent = end
	}
	bar := progress.New(colors, progress.WithScaled(true), progress.WithoutPercentage())
	bar.EmptyColor = lipgloss.Color("240")
	return bar, lipgloss.NewStyle().Foreground(lipgloss.Color(accent)).Bold(true), nil
}
