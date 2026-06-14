package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/UmarMushtaqMughal/vide/internal/parser"
	"github.com/UmarMushtaqMughal/vide/internal/tools"
	"github.com/spf13/cobra"
)

var synthCmd = &cobra.Command{
	Use:   "synth <file>",
	Short: "Run Yosys synthesis and display statistics",
	Long:  `Synthesizes the design with Yosys and reports gate count, wire count, and area statistics.`,
	Args:  cobra.ExactArgs(1),
	Run:   runSynth,
}

func init() {
	rootCmd.AddCommand(synthCmd)
}

func runSynth(cmd *cobra.Command, args []string) {
	if err := tools.EnsureToolchain(); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	target := args[0]
	files, _, err := parser.GetSources(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	// Use the actual module name from the source file, not the filename.
	topModule := parser.ExtractModuleName(target)

	hasSV := false
	for _, f := range files {
		if strings.HasSuffix(f, ".sv") {
			hasSV = true
			break
		}
	}

	readCmd := "read_verilog"
	if hasSV {
		readCmd = "read_verilog -sv"
	}

	fmt.Printf("🔬 Synthesizing %s...\n", topModule)

	var loadParts []string
	for _, f := range files {
		loadParts = append(loadParts, fmt.Sprintf("%s %s", readCmd, f))
	}
	loadScript := strings.Join(loadParts, "; ")
	yosysScript := fmt.Sprintf("%s; synth -top %s; stat", loadScript, topModule)

	yosysCmd := exec.Command(tools.GetBinPath("yosys"), "-p", yosysScript)
	output, err := yosysCmd.CombinedOutput()
	if err != nil {
		fmt.Println("❌ SYNTHESIS FAILED")
		for _, line := range strings.Split(string(output), "\n") {
			if strings.Contains(line, "ERROR") {
				fmt.Printf("   %s\n", strings.TrimSpace(line))
			}
		}
		os.Exit(1)
	}

	fmt.Println("✅ SYNTHESIS SUCCESSFUL")
	fmt.Println(strings.Repeat("─", 40))
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "Number of wires:") ||
			strings.Contains(trimmed, "Number of cells:") ||
			strings.Contains(trimmed, "Chip area") ||
			strings.Contains(trimmed, "printing statistics") {
			fmt.Printf("   %s\n", trimmed)
		}
	}
	fmt.Println(strings.Repeat("─", 40))
}
