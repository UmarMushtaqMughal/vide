package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/lipgloss"
)

type EditorMode int

const (
	ModeNormal EditorMode = iota
	ModeInsert
	ModeVisual
	ModeCommand
)

var modeLabels = map[EditorMode]string{
	ModeNormal:  "NORMAL",
	ModeInsert:  "INSERT",
	ModeVisual:  "VISUAL",
	ModeCommand: "COMMAND",
}

var modeColors = map[EditorMode]lipgloss.Color{
	ModeNormal:  lipgloss.Color("#5f87d7"), // steel blue
	ModeInsert:  lipgloss.Color("#87af5f"), // sage green
	ModeVisual:  lipgloss.Color("#d7875f"), // copper
	ModeCommand: lipgloss.Color("#af87d7"), // mauve
}

type SimPhase int

const (
	SimPhaseNone SimPhase = iota
	SimPhaseCompiling
	SimPhaseRunning
)

func phaseLabel(phase SimPhase) string {
	switch phase {
	case SimPhaseCompiling:
		return "Compiling..."
	case SimPhaseRunning:
		return "Simulating..."
	default:
		return ""
	}
}

var notifyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e2e2e2"))

type StatusBar struct {
	Mode       EditorMode
	FilePath   string
	Modified   bool
	CursorLine int
	CursorCol  int
	TotalLines int
	LintErrors int
	SimPhase   SimPhase
	BGMessage  string
	msgExpiry  time.Time
	spinner    spinner.Model
}

func NewStatusBar() StatusBar {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))
	return StatusBar{
		Mode:    ModeNormal,
		spinner: s,
	}
}

func (sb *StatusBar) SetMessage(msg string, duration time.Duration) {
	sb.BGMessage = msg
	sb.msgExpiry = time.Now().Add(duration)
}

func (sb *StatusBar) View(width int) string {
	col := modeColors[sb.Mode]

	modeBlock := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#1e1e1e")).
		Background(col).
		Padding(0, 1).
		Render(" " + modeLabels[sb.Mode] + " ")

	filename := "No file"
	if sb.FilePath != "" {
		filename = filepath.Base(sb.FilePath)
	}
	if sb.Modified {
		filename += " *"
	}
	fileBlock := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#c0c0c0")).
		Background(lipgloss.Color("#2a2a2a")).
		Padding(0, 1).
		Render(filename)

	center := ""
	if sb.SimPhase == SimPhaseRunning || sb.SimPhase == SimPhaseCompiling {
		center = sb.spinner.View() + " " + phaseLabel(sb.SimPhase)
	} else if time.Now().Before(sb.msgExpiry) {
		center = notifyStyle.Render(" " + sb.BGMessage + " ")
	}

	lintStr := ""
	if sb.LintErrors > 0 {
		lintStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).
			Render(fmt.Sprintf(" [LINT] %d ", sb.LintErrors))
	}
	pct := 0
	if sb.TotalLines > 0 {
		pct = sb.CursorLine * 100 / sb.TotalLines
	}
	posBlock := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#808080")).
		Background(lipgloss.Color("#2a2a2a")).
		Padding(0, 1).
		Render(fmt.Sprintf("%d:%d  %d%%", sb.CursorLine, sb.CursorCol, pct))

	left := modeBlock + fileBlock
	right := lintStr + posBlock

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	centerW := lipgloss.Width(center)
	gapTotal := width - leftW - centerW - rightW
	if gapTotal < 0 {
		gapTotal = 0
	}
	padL := gapTotal / 2
	padR := gapTotal - padL

	return left +
		strings.Repeat(" ", padL) +
		center +
		strings.Repeat(" ", padR) +
		right
}
