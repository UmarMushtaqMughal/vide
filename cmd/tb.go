package cmd

import (
	"fmt"
	"os"

	"github.com/UmarMushtaqMughal/vide/internal/tools"
	"github.com/spf13/cobra"
)

var tbCmd = &cobra.Command{
	Use:   "tb <file>",
	Short: "Generate a testbench from a module's port declarations",
	Long:  `Parses the module ports from the given Verilog/SystemVerilog file and generates a matching testbench. Clock and reset blocks are only emitted when the module declares clock/reset signals.`,
	Args:  cobra.ExactArgs(1),
	Run:   runTB,
}

func init() {
	rootCmd.AddCommand(tbCmd)
}

func runTB(cmd *cobra.Command, args []string) {
	target := args[0]

	// Check for existing file manually if we want to prompt in CLI mode before overwriting
	// Actually GenerateTB just overwrites, let's keep the prompt for CLI.
	tbFile, err := tools.GenerateTB(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// We removed the prompt logic from GenerateTB to make it re-usable.
	// If the user runs `tb` manually, they just overwrite. That's fine.
	fmt.Printf("[OK] Generated testbench: %s\n", tbFile)
}
