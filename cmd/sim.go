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

var simCmd = &cobra.Command{
	Use:   "sim <file>",
	Short: "Compile and run a Verilog/SystemVerilog simulation",
	Long:  `Compiles the design with Icarus Verilog (iverilog), runs the simulation with vvp, and reports the results. Auto-discovers testbench files.`,
	Args:  cobra.ExactArgs(1),
	Run:   runSim,
}

func init() {
	rootCmd.AddCommand(simCmd)
}

// RunSimulation is the core simulation logic, exported for use by watch and IDE.
func RunSimulation(target string, silent bool) error {
	if err := tools.EnsureToolchain(); err != nil {
		return err
	}

	files, baseName, err := parser.GetSources(target)
	if err != nil {
		return fmt.Errorf("getting sources: %w", err)
	}

	// Auto-discover testbench
	if !strings.HasSuffix(target, ".f") {
		tbSV := baseName + "_tb.sv"
		tbV := baseName + "_tb.v"
		if _, err := os.Stat(tbSV); err == nil {
			files = append(files, tbSV)
		} else if _, err := os.Stat(tbV); err == nil {
			files = append(files, tbV)
		}
	}

	// Validate all files exist
	for _, f := range files {
		if _, err := os.Stat(f); os.IsNotExist(err) {
			return fmt.Errorf("source file '%s' not found", f)
		}
	}

	outFile := baseName + ".vvp"

	// Determine flags
	hasSV := false
	for _, f := range files {
		if strings.HasSuffix(f, ".sv") {
			hasSV = true
			break
		}
	}

	args := []string{}
	if hasSV {
		args = append(args, "-g2012")
	}
	args = append(args, "-o", outFile)
	args = append(args, files...)

	if !silent {
		fmt.Printf("🔨 Compiling %d file(s)...\n", len(files))
	}
	compileCmd := exec.Command(tools.GetBinPath("iverilog"), args...)
	compileCmd.Stdout = os.Stdout
	compileCmd.Stderr = os.Stderr
	if err := compileCmd.Run(); err != nil {
		return fmt.Errorf("compilation failed")
	}

	if !silent {
		fmt.Println("🚀 Running Simulation...")
	}
	simCmd := exec.Command(tools.GetBinPath("vvp"), outFile)
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr
	if err := simCmd.Run(); err != nil {
		return fmt.Errorf("simulation runtime error")
	}

	// Check for VCD output
	vcdFile := parser.FindVCDFile(".")
	if vcdFile == "" {
		if !silent {
			fmt.Println("⚠️  No VCD file generated. Ensure your testbench has $dumpfile/$dumpvars and $finish.")
		}
	} else if !silent {
		fmt.Printf("✅ Simulation complete. VCD: %s\n", vcdFile)
		fmt.Println("💡 Use 'vide ide' for an integrated waveform viewer.")
	}

	return nil
}

func runSim(cmd *cobra.Command, args []string) {
	if err := RunSimulation(args[0], false); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}
}
