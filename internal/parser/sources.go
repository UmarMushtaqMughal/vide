// Package parser provides utilities for parsing Verilog/SystemVerilog source
// files, file lists, module declarations, and VCD waveform data.
package parser

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GetModuleName extracts the base name without extension from a file path.
// For example, "path/to/counter.sv" returns "counter".
func GetModuleName(filename string) string {
	base := filepath.Base(filename)
	ext := filepath.Ext(base)
	return strings.TrimSuffix(base, ext)
}

// GetSources resolves a target into a list of source files and a base module name.
//
// If target ends with ".f", it is treated as a file list: each non-empty line
// that does not start with '#' or '//' is taken as a source file path.
// The returned baseName is the module name of the first file in the list.
//
// Otherwise, target is treated as a single source file. The returned list
// contains only that file, and baseName is its module name.
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

	return []string{target}, GetModuleName(target), nil
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
