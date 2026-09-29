package main

import (
	"fmt"
	"strconv"
	"strings"
)

// A zero value uses the default 60% of the terminal's columns.
type barWidthSpec struct {
	cells   int
	percent int
}

func parseBarWidth(value string) (barWidthSpec, error) {
	if value == "auto" {
		value = "60%"
	}
	number, percent := strings.CutSuffix(value, "%")
	n, err := strconv.Atoi(number)
	if err == nil && strconv.Itoa(n) == number {
		if percent && n >= 1 && n <= 100 {
			return barWidthSpec{percent: n}, nil
		}
		if !percent && n >= 3 && n <= 999999999 {
			return barWidthSpec{cells: n}, nil
		}
	}
	return barWidthSpec{}, fmt.Errorf("invalid width %q: use 1%%–100%% or 3–999999999 columns", value)
}

func (s barWidthSpec) columns(terminalWidth int) int {
	if s.cells > 0 {
		return s.cells
	}
	percent := s.percent
	if percent == 0 {
		percent = 60
	}
	return max(3, terminalWidth*percent/100)
}
