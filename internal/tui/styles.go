// Package tui implements the Bubble Tea terminal UI for the Vide IDE.
package tui

import (
	"os"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Enable adaptive colors based on terminal profile (TrueColor -> 256 -> 16 -> ANSI)
	lipgloss.SetColorProfile(termenv.Profile(colorprofile.Detect(os.Stdout, os.Environ())))
}

// PaneType identifies the four panes in the IDE layout.
type PaneType int

const (
	PaneFiles     PaneType = iota // File explorer pane
	PaneCode                      // Editor pane
	PaneTerminal                  // Output log pane
	PaneWaveform                  // Waveform viewer pane
	PaneBootstrap                 // Loading/Downloading screen
)

// ---------------------------------------------------------------------------
// Pane chrome
// ---------------------------------------------------------------------------

// StyleActivePane is applied to the currently focused pane border.
var StyleActivePane = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#00FFFF"))

// StyleInactivePane is applied to unfocused pane borders.
var StyleInactivePane = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#555555"))

// ---------------------------------------------------------------------------
// Title bar
// ---------------------------------------------------------------------------

// StyleTitle styles the pane title labels.
var StyleTitle = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#FAFAFA")).
	Background(lipgloss.Color("#7D56F4")).
	PaddingLeft(1).
	PaddingRight(1)

// ---------------------------------------------------------------------------
// Status bar
// ---------------------------------------------------------------------------

// StyleStatusBar styles the bottom status / help bar.
var StyleStatusBar = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#A0A0A0")).
	Background(lipgloss.Color("#1A1A2E"))

// ---------------------------------------------------------------------------
// Help text
// ---------------------------------------------------------------------------

// StyleHelpKey highlights keyboard shortcuts in the help bar.
var StyleHelpKey = lipgloss.NewStyle().
	Bold(true).
	Foreground(lipgloss.Color("#00FFFF"))

// StyleHelpDesc describes the action for a keyboard shortcut.
var StyleHelpDesc = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#A0A0A0"))

// ---------------------------------------------------------------------------
// Simulation state indicators
// ---------------------------------------------------------------------------

// StyleSimRunning is used while a simulation is in progress.
var StyleSimRunning = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFD700")).
	Bold(true)

// StyleSimSuccess is shown after a successful simulation.
var StyleSimSuccess = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#00FF7F")).
	Bold(true)

// StyleSimError is shown when a simulation fails.
var StyleSimError = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FF4444")).
	Bold(true)

// ---------------------------------------------------------------------------
// Waveform rendering
// ---------------------------------------------------------------------------

// StyleWaveHigh colours a logic-high signal trace.
var StyleWaveHigh = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#00FF7F"))

// StyleWaveLow colours a logic-low signal trace.
var StyleWaveLow = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FF6B6B"))

// StyleWaveBus colours a multi-bit / bus signal trace.
var StyleWaveBus = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#87CEEB"))

// ---------------------------------------------------------------------------
// File list
// ---------------------------------------------------------------------------

// StyleFileName styles a normal (unselected) file name.
var StyleFileName = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#E0E0E0"))

// StyleActiveFile styles the currently selected file in the explorer.
var StyleActiveFile = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#00FFFF")).
	Bold(true)
