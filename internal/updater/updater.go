// Package updater provides self-update functionality for the vide binary.
// It queries the GitHub Releases API, compares versions, downloads the
// correct platform-specific asset, and safely replaces the running binary.
package updater

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Version is set at build time via ldflags.
// Default is "dev" for local development builds.
var Version = "dev"

const (
	repoOwner   = "UmarMushtaqMughal"
	repoName    = "vide"
	releasesURL = "https://api.github.com/repos/" + repoOwner + "/" + repoName + "/releases"
	timeout     = 30 * time.Second
)

// ReleaseInfo holds the parsed fields from a GitHub release.
type ReleaseInfo struct {
	TagName    string  `json:"tag_name"`
	Prerelease bool    `json:"prerelease"`
	Draft      bool    `json:"draft"`
	Assets     []Asset `json:"assets"`
}

// Asset represents a single downloadable file attached to a release.
type Asset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CheckResult is the outcome of a version check.
type CheckResult struct {
	CurrentVersion string
	LatestVersion  string
	UpdateAvail    bool
	DownloadURL    string
	AssetSize      int64
	AssetName      string
}

// CheckForUpdates queries the GitHub API and compares versions.
func CheckForUpdates() (*CheckResult, error) {
	client := &http.Client{Timeout: timeout}
	// Use /releases (not /releases/latest) so pre-releases are included.
	req, err := http.NewRequest("GET", releasesURL+"?per_page=10", nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "vide-updater/"+Version)

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var releases []ReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("parsing release info: %w", err)
	}

	// Find the newest non-draft release (pre-releases are fine).
	var release *ReleaseInfo
	for i := range releases {
		if !releases[i].Draft {
			release = &releases[i]
			break
		}
	}
	if release == nil {
		return nil, fmt.Errorf("no releases found on GitHub")
	}

	result := &CheckResult{
		CurrentVersion: Version,
		LatestVersion:  release.TagName,
		UpdateAvail:    normalizeVersion(release.TagName) != normalizeVersion(Version),
	}

	// Find the matching asset for this OS/arch.
	assetName := buildAssetName()
	for _, a := range release.Assets {
		if a.Name == assetName {
			result.DownloadURL = a.BrowserDownloadURL
			result.AssetSize = a.Size
			result.AssetName = a.Name
			break
		}
	}

	if result.UpdateAvail && result.DownloadURL == "" {
		return nil, fmt.Errorf("no binary available for %s/%s in release %s",
			runtime.GOOS, runtime.GOARCH, release.TagName)
	}

	return result, nil
}

// ExecuteUpdate downloads and installs the update. The progress callback
// receives (bytesDownloaded, totalBytes) and is called periodically.
func ExecuteUpdate(info *CheckResult, progress func(int64, int64)) error {
	if !info.UpdateAvail {
		return nil
	}

	// Pre-flight: check we can actually write to the binary location.
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	if err := checkWritePermission(execPath); err != nil {
		return err
	}

	// Download the archive to a temp file in the same directory as the binary
	// (same filesystem guarantees atomic rename on POSIX).
	execDir := filepath.Dir(execPath)
	tmpArchive, err := os.CreateTemp(execDir, "vide-update-*.tar.gz")
	if err != nil {
		// Fallback to system temp if same-dir fails (e.g. read-only mount).
		tmpArchive, err = os.CreateTemp("", "vide-update-*.tar.gz")
		if err != nil {
			return fmt.Errorf("creating temp file: %w", err)
		}
	}
	tmpArchivePath := tmpArchive.Name()
	defer os.Remove(tmpArchivePath) // cleanup on any exit path

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(info.DownloadURL)
	if err != nil {
		tmpArchive.Close()
		return fmt.Errorf("downloading update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpArchive.Close()
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	totalSize := resp.ContentLength
	var downloaded int64

	// Stream the download with progress reporting.
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := tmpArchive.Write(buf[:n]); writeErr != nil {
				tmpArchive.Close()
				return fmt.Errorf("writing download: %w", writeErr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, totalSize)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			tmpArchive.Close()
			return fmt.Errorf("reading download: %w", readErr)
		}
	}
	tmpArchive.Close()

	// Extract the "vide" binary from the tar.gz archive.
	newBinaryPath, err := extractBinary(tmpArchivePath, execDir)
	if err != nil {
		return fmt.Errorf("extracting update: %w", err)
	}
	defer func() {
		// If the swap fails, clean up the extracted binary.
		if _, err := os.Stat(newBinaryPath); err == nil {
			os.Remove(newBinaryPath)
		}
	}()

	// Safe swap: rename current → .old, rename new → current.
	return swapBinary(execPath, newBinaryPath)
}

// normalizeVersion strips the leading 'v' for comparison.
func normalizeVersion(v string) string {
	return strings.TrimPrefix(strings.TrimSpace(v), "v")
}

// buildAssetName returns the GoReleaser-style archive name for this platform.
func buildAssetName() string {
	osName := strings.Title(runtime.GOOS) //nolint:staticcheck // fine for linux/darwin
	archName := runtime.GOARCH
	switch archName {
	case "amd64":
		archName = "x86_64"
	case "386":
		archName = "i386"
	}
	return fmt.Sprintf("vide_%s_%s.tar.gz", osName, archName)
}

// checkWritePermission verifies we can write to the binary's directory.
// On Unix this returns a clear message suggesting sudo if permission is denied.
func checkWritePermission(execPath string) error {
	dir := filepath.Dir(execPath)
	testFile := filepath.Join(dir, ".vide-update-test")
	f, err := os.Create(testFile)
	if err != nil {
		if os.IsPermission(err) {
			if runtime.GOOS != "windows" {
				return fmt.Errorf(
					"permission denied: cannot write to %s\n"+
						"  Try running with elevated privileges:\n"+
						"    sudo vide update", dir)
			}
			return fmt.Errorf(
				"permission denied: cannot write to %s\n"+
					"  Try running as Administrator.", dir)
		}
		return fmt.Errorf("cannot write to install directory %s: %w", dir, err)
	}
	f.Close()
	os.Remove(testFile)
	return nil
}

// extractBinary opens a tar.gz archive and extracts the "vide" binary
// to a temp file in targetDir. It returns the path to the extracted file.
func extractBinary(archivePath, targetDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	binaryName := "vide"
	if runtime.GOOS == "windows" {
		binaryName = "vide.exe"
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("reading tar: %w", err)
		}
		if filepath.Base(hdr.Name) == binaryName && hdr.Typeflag == tar.TypeReg {
			tmpBin, err := os.CreateTemp(targetDir, "vide-new-*")
			if err != nil {
				return "", fmt.Errorf("creating temp binary: %w", err)
			}
			if _, err := io.Copy(tmpBin, tr); err != nil {
				tmpBin.Close()
				os.Remove(tmpBin.Name())
				return "", fmt.Errorf("extracting binary: %w", err)
			}
			tmpBin.Close()

			// Preserve executable permission on Unix.
			if runtime.GOOS != "windows" {
				if err := os.Chmod(tmpBin.Name(), 0755); err != nil {
					os.Remove(tmpBin.Name())
					return "", fmt.Errorf("setting permissions: %w", err)
				}
			}
			return tmpBin.Name(), nil
		}
	}

	return "", fmt.Errorf("binary %q not found in archive", binaryName)
}

// swapBinary safely replaces oldPath with newPath using the rename trick.
func swapBinary(oldPath, newPath string) error {
	backupPath := oldPath + ".old"

	// Remove any stale backup from a previous update.
	os.Remove(backupPath)

	// Step 1: Rename current binary → .old
	if err := os.Rename(oldPath, backupPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	// Step 2: Rename new binary → current
	if err := os.Rename(newPath, oldPath); err != nil {
		// Rollback: restore the backup.
		os.Rename(backupPath, oldPath)
		return fmt.Errorf("renaming new binary: %w", err)
	}

	// Step 3: Clean up the old backup (best-effort).
	// On Windows this may fail because the old binary is still running;
	// it will be cleaned up on the next update.
	os.Remove(backupPath)

	return nil
}

// Rollback restores the previous version of the binary from the .old backup file.
func Rollback() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("determining executable path: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("evaluating symlinks: %w", err)
	}

	backupPath := execPath + ".old"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup found at %s. You can only rollback once.", backupPath)
	}

	if err := checkWritePermission(execPath); err != nil {
		return err
	}

	brokenPath := execPath + ".broken"
	os.Remove(brokenPath)

	if err := os.Rename(execPath, brokenPath); err != nil {
		return fmt.Errorf("moving current binary out of the way: %w", err)
	}

	if err := os.Rename(backupPath, execPath); err != nil {
		// try to recover
		os.Rename(brokenPath, execPath)
		return fmt.Errorf("restoring backup binary: %w", err)
	}

	// Clean up broken binary
	os.Remove(brokenPath)
	return nil
}
