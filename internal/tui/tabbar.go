package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func truncateANSI(s string, maxLen int) string {
	// A simple truncation that assumes standard ascii.
	// For full ANSI-aware truncation, we would use ansi.Truncate.
	// We'll just truncate the visible characters if there are no escapes,
	// or rely on lipgloss to handle sizing. But for filenames, they don't have ANSI.
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// RenderTabBar renders the list of open files as tabs.
func RenderTabBar(tabs []string, modified map[string]bool, activeIdx int, maxWidth int) string {
	var parts []string
	for i, path := range tabs {
		name := filepath.Base(path)
		// Truncate long names with ellipsis
		if len(name) > 18 {
			name = "…" + name[len(name)-17:]
		}

		dirtyMark := " "
		if modified[path] {
			dirtyMark = "*"
		}
		label := fmt.Sprintf(" %s %s ", name, dirtyMark)

		var s lipgloss.Style
		if i == activeIdx {
			s = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#ffffff")).
				Background(lipgloss.Color("#3a3d55")).
				BorderBottom(true).
				BorderForeground(modeColors[ModeNormal]) // using ModeNormal color for active tab line
		} else {
			s = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#888888")).
				Background(lipgloss.Color("#252830"))
		}
		parts = append(parts, s.Render(label))
	}

	full := strings.Join(parts, "")
	if lipgloss.Width(full) > maxWidth {
		// Truncate if overflowing
		full = truncateANSI(full, maxWidth-3) + "…"
	}
	return full
}
