package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/UmarMushtaqMughal/vide/internal/parser"
	"github.com/UmarMushtaqMughal/vide/internal/templates"
	"github.com/spf13/cobra"
)

var newCmd = &cobra.Command{
	Use:   "new <file>",
	Short: "Create a new Verilog or SystemVerilog module",
	Long: `Creates a new Verilog (.v) or SystemVerilog (.sv) file with boilerplate code.
If no extension is provided, defaults to .sv.`,
	Args: cobra.ExactArgs(1),
	Run:  runNew,
}

func init() {
	rootCmd.AddCommand(newCmd)
}

func runNew(cmd *cobra.Command, args []string) {
	target := args[0]

	// Add default extension if none provided
	if !strings.HasSuffix(target, ".v") && !strings.HasSuffix(target, ".sv") {
		target += ".sv"
	}
	isSV := strings.HasSuffix(target, ".sv")

	if _, err := os.Stat(target); err == nil {
		fmt.Fprintf(os.Stderr, "Error: %s already exists.\n", target)
		os.Exit(1)
	}

	name := parser.GetModuleName(target)
	ext := filepath.Ext(target)
	_ = ext

	var content string
	if isSV {
		content = fmt.Sprintf(templates.BoilerplateSV, name)
	} else {
		content = fmt.Sprintf(templates.BoilerplateV, name)
	}

	if err := os.WriteFile(target, []byte(content), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[OK] Created %s\n", target)
}
