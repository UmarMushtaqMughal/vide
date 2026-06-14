package parser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExtractModuleName(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		expected string
	}{
		{
			name: "module name differs from filename",
			content: `module six_in_controller(
    input logic [3:0] digit_input,
    output logic morse_led
);
endmodule`,
			expected: "six_in_controller",
		},
		{
			name: "module name matches filename",
			content: `module controller(
    input logic clk
);
endmodule`,
			expected: "controller",
		},
		{
			name: "module with comments before",
			content: `// This is a comment
/* block comment */
module my_design(
    input logic a
);
endmodule`,
			expected: "my_design",
		},
		{
			name: "module name in comment is skipped",
			content: `// module fake_name
module real_name(
    input logic a
);
endmodule`,
			expected: "real_name",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile := filepath.Join(t.TempDir(), "controller.sv")
			if err := os.WriteFile(tmpFile, []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			got := ExtractModuleName(tmpFile)
			if got != tc.expected {
				t.Errorf("ExtractModuleName() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestExtractModuleName_FileNotFound(t *testing.T) {
	// Should fall back to filename-based name.
	got := ExtractModuleName("nonexistent_file.sv")
	if got != "nonexistent_file" {
		t.Errorf("expected fallback 'nonexistent_file', got %q", got)
	}
}

func TestGetSources_AutoDiscovers(t *testing.T) {
	dir := t.TempDir()

	// Create source files.
	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("controller.sv", "module controller(); endmodule")
	write("morse.sv", "module morsecode(); endmodule")
	write("controller_tb.sv", "module controller_tb(); endmodule")
	write("notes.txt", "not a source file")

	target := filepath.Join(dir, "controller.sv")
	files, baseName, err := GetSources(target)
	if err != nil {
		t.Fatal(err)
	}

	if baseName != "controller" {
		t.Errorf("baseName = %q, want 'controller'", baseName)
	}

	// Should include controller.sv and morse.sv but NOT controller_tb.sv or notes.txt.
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(files), files)
	}

	hasTarget := false
	hasMorse := false
	for _, f := range files {
		base := filepath.Base(f)
		if base == "controller.sv" {
			hasTarget = true
		}
		if base == "morse.sv" {
			hasMorse = true
		}
		if base == "controller_tb.sv" {
			t.Error("testbench should not be auto-discovered")
		}
		if base == "notes.txt" {
			t.Error("non-source file should not be discovered")
		}
	}
	if !hasTarget {
		t.Error("target file missing from results")
	}
	if !hasMorse {
		t.Error("sibling source file morse.sv missing from results")
	}
}

func TestGetSources_SingleFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "adder.sv")
	os.WriteFile(target, []byte("module adder(); endmodule"), 0644)

	files, baseName, err := GetSources(target)
	if err != nil {
		t.Fatal(err)
	}
	if baseName != "adder" {
		t.Errorf("baseName = %q, want 'adder'", baseName)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 file, got %d", len(files))
	}
}

func TestFindVCDFile(t *testing.T) {
	dir := t.TempDir()

	// No VCD files → empty string.
	if got := FindVCDFile(dir); got != "" {
		t.Errorf("expected empty, got %q", got)
	}

	// Create two VCD files with different modification times.
	oldVCD := filepath.Join(dir, "old.vcd")
	newVCD := filepath.Join(dir, "waveform.vcd")

	os.WriteFile(oldVCD, []byte("old"), 0644)
	// Ensure a time gap.
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(newVCD, []byte("new"), 0644)

	got := FindVCDFile(dir)
	if filepath.Base(got) != "waveform.vcd" {
		t.Errorf("expected newest VCD 'waveform.vcd', got %q", filepath.Base(got))
	}
}

func TestFindVCDFile_EmptyDir(t *testing.T) {
	if got := FindVCDFile(""); got != "" {
		// Empty string means CWD, which may or may not have VCDs.
		// Just ensure it doesn't panic.
		_ = got
	}
}
