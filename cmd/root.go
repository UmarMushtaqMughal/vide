package cmd

import (
	"fmt"
	"os"

	"github.com/UmarMushtaqMughal/vide/internal/updater"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "vide",
	Short: "Vide — A terminal-native Verilog/SystemVerilog IDE",
	Long: `Vide is a lightweight, command-line Verilog/SystemVerilog development
environment with auto-simulation, synthesis checking, an integrated
waveform viewer, and a full terminal UI built with Bubble Tea.`,
	Version: updater.Version,
	Args:    cobra.ArbitraryArgs,
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 1 {
			// Redirect default 'vide <file>' to 'vide ide <file>'
			runIDE(cmd, args)
		} else {
			cmd.Help()
		}
	},
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
