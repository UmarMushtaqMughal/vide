package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/UmarMushtaqMughal/vide/internal/parser"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch <file>",
	Short: "Auto-recompile and simulate on file changes",
	Long:  `Monitors source and testbench files for changes and re-runs the simulation automatically. Press Ctrl+C to stop.`,
	Args:  cobra.ExactArgs(1),
	Run:   runWatch,
}

func init() {
	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) {
	target := args[0]

	files, baseName, err := parser.GetSources(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
		os.Exit(1)
	}

	// Also watch testbench files
	if !strings.HasSuffix(target, ".f") {
		tbSV := baseName + "_tb.sv"
		tbV := baseName + "_tb.v"
		if _, err := os.Stat(tbSV); err == nil {
			files = append(files, tbSV)
		} else if _, err := os.Stat(tbV); err == nil {
			files = append(files, tbV)
		}
	}

	fmt.Printf("👀 Watching %d files... (Ctrl+C to stop)\n", len(files))

	// Initial simulation
	if err := RunSimulation(target, false); err != nil {
		fmt.Fprintf(os.Stderr, "❌ %v\n", err)
	}

	// Seed mtimes with current values to avoid double-trigger
	mtimes := make(map[string]time.Time)
	for _, f := range files {
		info, err := os.Stat(f)
		if err == nil {
			mtimes[f] = info.ModTime()
		}
	}

	for {
		time.Sleep(500 * time.Millisecond)

		changed := false
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			if info.ModTime().After(mtimes[f]) {
				mtimes[f] = info.ModTime()
				changed = true
			}
		}

		if changed {
			fmt.Println("\n🔄 Change detected. Re-simulating...")
			if err := RunSimulation(target, false); err != nil {
				fmt.Fprintf(os.Stderr, "❌ %v\n", err)
			}
		}
	}
}
