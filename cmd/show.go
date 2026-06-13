package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/UmarMushtaqMughal/vide/internal/parser"
	"github.com/UmarMushtaqMughal/vide/internal/tools"
	"github.com/spf13/cobra"
)

var showCmd = &cobra.Command{
	Use:   "show <file> [--prep|--hier]",
	Short: "Generate and view a schematic with Yosys",
	Long: `Generates a circuit schematic using Yosys and opens it with the system viewer.
  
Flags:
  --prep   Abstract RTL representation
  --hier   Hierarchical box view`,
	Args: cobra.ExactArgs(1),
	Run:  runShow,
}

var showPrep bool
var showHier bool

func init() {
	showCmd.Flags().BoolVar(&showPrep, "prep", false, "Abstract RTL representation")
	showCmd.Flags().BoolVar(&showHier, "hier", false, "Hierarchical box view")
	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) {
	if err := tools.CheckTool("yosys"); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	target := args[0]
	files, topModule, err := parser.GetSources(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

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

	dotFile := topModule + ".dot"

	var algo string
	if showHier {
		fmt.Println("🧱 Generating Hierarchy View (Boxes)...")
		algo = fmt.Sprintf("hierarchy -check -top %s; proc", topModule)
	} else if showPrep {
		fmt.Println("🎨 Generating Abstract RTL Schematic...")
		algo = fmt.Sprintf("prep -top %s", topModule)
	} else {
		fmt.Println("⚡ Generating Gate-Level Schematic (Flattened)...")
		algo = fmt.Sprintf("synth -top %s", topModule)
	}

	var loadParts []string
	for _, f := range files {
		loadParts = append(loadParts, fmt.Sprintf("%s %s", readCmd, f))
	}
	loadScript := strings.Join(loadParts, "; ")
	yosysScript := fmt.Sprintf("%s; %s; show -prefix %s -format dot -colors 2 -width -stretch -enum", loadScript, algo, topModule)

	yosysCmd := exec.Command("yosys", "-q", "-p", yosysScript)
	yosysCmd.Stdout = os.Stdout
	yosysCmd.Stderr = os.Stderr
	if err := yosysCmd.Run(); err != nil {
		fmt.Println("❌ Failed to generate schematic.")
		os.Exit(1)
	}

	// Open the dot file with system viewer
	openDotFile(dotFile)
}

func openDotFile(path string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", path)
	case "darwin":
		cmd = exec.Command("open", path)
	default:
		// Try xdot first, fall back to xdg-open
		if _, err := exec.LookPath("xdot"); err == nil {
			cmd = exec.Command("xdot", path)
		} else {
			cmd = exec.Command("xdg-open", path)
		}
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	_ = cmd.Start()
}
