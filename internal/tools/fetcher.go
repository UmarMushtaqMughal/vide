package tools

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ProgressMsg represents download/extraction progress
type ProgressMsg struct {
	Percent float64
	Status  string
	Done    bool
	Err     error
}

type progressWriter struct {
	total   uint64
	written uint64
	ch      chan<- ProgressMsg
	status  string
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n := len(p)
	pw.written += uint64(n)
	if pw.total > 0 && pw.ch != nil {
		// Limit message spam: send every 1% or so if possible, but for simplicity
		// we just send it. Bubble tea can handle rapid messages.
		pw.ch <- ProgressMsg{
			Percent: float64(pw.written) / float64(pw.total),
			Status:  pw.status,
		}
	}
	return n, nil
}

// GetToolchainDir returns the local directory where OSS CAD Suite should be installed.
func GetToolchainDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = "."
	}
	return filepath.Join(configDir, "vide", "oss-cad-suite")
}

// GetBinPath returns the absolute path to the local executable.
func GetBinPath(tool string) string {
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return filepath.Join(GetToolchainDir(), "bin", tool+ext)
}

// IsToolchainPresent checks if iverilog, vvp, and yosys exist locally.
func IsToolchainPresent() bool {
	tools := []string{"iverilog", "vvp", "yosys"}
	for _, t := range tools {
		if _, err := os.Stat(GetBinPath(t)); err != nil {
			return false
		}
	}
	return true
}

func getAssetPrefix() (string, error) {
	osName := runtime.GOOS
	archName := runtime.GOARCH

	var prefixOS string
	switch osName {
	case "windows":
		prefixOS = "windows"
	case "darwin":
		prefixOS = "darwin"
	case "linux":
		prefixOS = "linux"
	default:
		return "", fmt.Errorf("unsupported OS: %s", osName)
	}

	var prefixArch string
	switch archName {
	case "amd64":
		prefixArch = "x64"
	case "arm64":
		prefixArch = "arm64"
	default:
		return "", fmt.Errorf("unsupported arch: %s", archName)
	}

	return fmt.Sprintf("oss-cad-suite-%s-%s", prefixOS, prefixArch), nil
}

func fetchLatestReleaseURL() (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/YosysHQ/oss-cad-suite-build/releases/latest", nil)
	if err != nil {
		return "", err
	}
	
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}

	prefix, err := getAssetPrefix()
	if err != nil {
		return "", err
	}

	for _, asset := range release.Assets {
		if strings.HasPrefix(asset.Name, prefix) {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("no matching asset found for %s", prefix)
}

// DownloadAndExtract performs the bootstrap logic.
func DownloadAndExtract(ch chan<- ProgressMsg) {
	defer close(ch)

	ch <- ProgressMsg{Status: "Finding latest OSS CAD Suite release...", Percent: 0.0}
	url, err := fetchLatestReleaseURL()
	if err != nil {
		ch <- ProgressMsg{Err: fmt.Errorf("failed to find release: %v", err)}
		return
	}

	ch <- ProgressMsg{Status: "Downloading...", Percent: 0.0}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		ch <- ProgressMsg{Err: err}
		return
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		ch <- ProgressMsg{Err: err}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		ch <- ProgressMsg{Err: fmt.Errorf("download failed: %d", resp.StatusCode)}
		return
	}

	totalSize := uint64(resp.ContentLength)
	pw := &progressWriter{
		total:  totalSize,
		ch:     ch,
		status: "Downloading...",
	}

	reader := io.TeeReader(resp.Body, pw)

	tmpPattern := "oss-cad-suite-*"
	if runtime.GOOS == "windows" {
		tmpPattern += ".exe"
	} else {
		tmpPattern += ".tgz"
	}

	tmpFile, err := os.CreateTemp("", tmpPattern)
	if err != nil {
		ch <- ProgressMsg{Err: err}
		return
	}
	tmpName := tmpFile.Name()
	defer os.Remove(tmpName)

	if _, err := io.Copy(tmpFile, reader); err != nil {
		tmpFile.Close()
		ch <- ProgressMsg{Err: err}
		return
	}
	tmpFile.Close()

	ch <- ProgressMsg{Status: "Extracting (this may take a minute)...", Percent: 1.0}

	targetDir := GetToolchainDir()
	
	// Create parent directory of targetDir since OSS CAD Suite creates 'oss-cad-suite' dir inside
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		ch <- ProgressMsg{Err: err}
		return
	}

	if runtime.GOOS == "windows" {
		// Add execute permission just in case
		os.Chmod(tmpName, 0755)
		// Run SFX
		cmd := exec.Command(tmpName, "-y", "-o"+parentDir)
		if err := cmd.Run(); err != nil {
			ch <- ProgressMsg{Err: fmt.Errorf("extraction failed: %v", err)}
			return
		}
	} else {
		// Untar
		if err := extractTarGz(tmpName, parentDir); err != nil {
			ch <- ProgressMsg{Err: fmt.Errorf("extraction failed: %v", err)}
			return
		}
	}

	ch <- ProgressMsg{Status: "Complete", Percent: 1.0, Done: true}
}

func extractTarGz(archive, target string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()

	gzReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzReader.Close()

	tarReader := tar.NewReader(gzReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(target, header.Name)
		if !strings.HasPrefix(path, filepath.Clean(target)+string(os.PathSeparator)) {
			// Skip relative path escapes
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, os.FileMode(header.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tarReader); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
