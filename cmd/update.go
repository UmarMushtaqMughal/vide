package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/UmarMushtaqMughal/vide/internal/updater"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update vide to the latest release",
	Long:  `Fetches the latest release from GitHub, compares versions, and safely replaces the current binary.`,
	Args:  cobra.NoArgs,
	Run:   runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) {
	fmt.Println("[SEARCH] Checking for latest updates from GitHub...")

	result, err := updater.CheckForUpdates()
	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}

	if !result.UpdateAvail {
		fmt.Printf("[OK] vide is already up to date (%s).\n", result.CurrentVersion)
		return
	}

	fmt.Printf("[PKG] Found version %s (Current: %s). Downloading...\n",
		result.LatestVersion, result.CurrentVersion)

	// Progress bar state.
	var lastPct int64 = -1

	err = updater.ExecuteUpdate(result, func(downloaded, total int64) {
		if total <= 0 {
			// No Content-Length header, show raw bytes.
			fmt.Printf("\r   [DOWNLOAD]  %.1f MB downloaded...", float64(downloaded)/1024/1024)
			return
		}
		pct := downloaded * 100 / total
		if pct != lastPct {
			lastPct = pct
			barWidth := 40
			filled := int(pct) * barWidth / 100
			bar := strings.Repeat("#", filled) + strings.Repeat("-", barWidth-filled)
			fmt.Printf("\r   [DOWNLOAD]  [%s] %3d%% (%.1f/%.1f MB)",
				bar, pct,
				float64(downloaded)/1024/1024,
				float64(total)/1024/1024)
		}
	})

	fmt.Println() // newline after progress bar

	if err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Update failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[OK] Successfully updated to %s! Please restart vide.\n", result.LatestVersion)
}
