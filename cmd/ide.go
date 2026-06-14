package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UmarMushtaqMughal/vide/internal/parser"
	"github.com/UmarMushtaqMughal/vide/internal/templates"
	"github.com/UmarMushtaqMughal/vide/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var ideCmd = &cobra.Command{
	Use:   "ide <file>",
	Short: "Launch the integrated terminal IDE",
	Long: `Opens a full-screen terminal IDE with four panes:
  - File List    — navigate your source files
  - Code Viewer  — read file contents
  - Terminal     — simulation and synthesis output
  - Waveform     — integrated VCD waveform viewer

Navigation:
  Tab/Shift+Tab  Switch between panes
  s              Run simulation
  y              Run synthesis
  ↑↓             Navigate within active pane
  ←→ +/-         Scroll/zoom waveform
  q / Ctrl+C     Quit`,
	Args: cobra.ExactArgs(1),
	Run:  runIDE,
}

func init() {
	rootCmd.AddCommand(ideCmd)
}

func runIDE(cmd *cobra.Command, args []string) {
	target := args[0]

	// 1. Smart Extension Resolution
	if filepath.Ext(target) == "" {
		if _, err := os.Stat(target + ".sv"); err == nil {
			target += ".sv"
		} else if _, err := os.Stat(target + ".v"); err == nil {
			target += ".v"
		} else {
			target += ".sv" // Default to .sv for creation
		}
	}

	// 2. Auto-create if it doesn't exist
	if _, err := os.Stat(target); os.IsNotExist(err) {
		name := parser.GetModuleName(target)
		var content string
		if strings.HasSuffix(target, ".sv") {
			content = fmt.Sprintf(templates.BoilerplateSV, name)
		} else {
			content = fmt.Sprintf(templates.BoilerplateV, name)
		}
		if err := os.WriteFile(target, []byte(content), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Error creating %s: %v\n", target, err)
			os.Exit(1)
		}
	}

	model := tui.NewModel(target)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseAllMotion())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error running IDE: %v\n", err)
		os.Exit(1)
	}
}
