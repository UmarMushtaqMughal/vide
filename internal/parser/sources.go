// Package parser provides utilities for parsing Verilog/SystemVerilog source
// files, file lists, module declarations, and VCD waveform data.
package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GetModuleName extracts the base name without extension from a file path.
// For example, "path/to/counter.sv" returns "counter".
func GetModuleName(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// moduleNameRe matches the first `module <name>` declaration in source code.
var moduleNameRe = regexp.MustCompile(`(?m)^\s*module\s+(\w+)`)

// ExtractModuleName reads a Verilog/SystemVerilog file and extracts the actual
// module name from the `module <name>` declaration. Returns the filename-based
// name as a fallback if parsing fails.
func ExtractModuleName(filename string) string {
	data, err := os.ReadFile(filename)
	if err != nil {
		return GetModuleName(filename)
	}
	// Strip comments to avoid matching inside comments.
	cleaned := blockCommentRe.ReplaceAllString(string(data), " ")
	cleaned = lineCommentRe.ReplaceAllString(cleaned, " ")

	m := moduleNameRe.FindStringSubmatch(cleaned)
	if m == nil {
		return GetModuleName(filename)
	}
	return m[1]
}

// GetSources resolves a target into a list of source files and a base module name.
//
// If target ends with ".f", it is treated as a file list: each non-empty line
// that does not start with '#' or '//' is taken as a source file path.
// The returned baseName is the module name of the first file in the list.
//
// Otherwise, target is treated as a single source file. All sibling .sv and .v
// files in the same directory (excluding testbenches) are auto-discovered and
// included. The returned baseName is the filename-based name of the target.
func GetSources(target string) (files []string, baseName string, err error) {
	if strings.HasSuffix(target, ".f") {
		files, err = readFileList(target)
		if err != nil {
			return nil, "", fmt.Errorf("reading file list %s: %w", target, err)
		}
		if len(files) == 0 {
			return nil, "", fmt.Errorf("file list %s contains no source files", target)
		}
		baseName = GetModuleName(files[0])
		return files, baseName, nil
	}

	baseName = GetModuleName(target)

	// Auto-discover all sibling .sv/.v source files in the same directory.
	dir := filepath.Dir(target)
	if dir == "" {
		dir = "."
	}
	files = []string{target}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip the target file itself (already in list).
		full := filepath.Join(dir, name)
		if full == target || name == filepath.Base(target) {
			continue
		}
		// Skip testbenches — they're added separately by the sim logic.
		if strings.HasSuffix(name, "_tb.sv") || strings.HasSuffix(name, "_tb.v") {
			continue
		}
		if strings.HasSuffix(name, ".sv") || strings.HasSuffix(name, ".v") {
			files = append(files, full)
		}
	}

	return files, baseName, nil
}

// FindVCDFile finds the most recently modified .vcd file in the given directory.
// Returns "" if no VCD file is found.
func FindVCDFile(dir string) string {
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var newest string
	var newestTime int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".vcd") {
			info, err := e.Info()
			if err != nil {
				continue
			}
			t := info.ModTime().UnixNano()
			if t > newestTime {
				newestTime = t
				newest = filepath.Join(dir, e.Name())
			}
		}
	}
	return newest
}

// readFileList reads a .f file list, returning the non-empty, non-comment lines.
func readFileList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var files []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		files = append(files, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return files, nil
}
