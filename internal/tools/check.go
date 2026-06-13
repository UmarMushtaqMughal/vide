// Package tools provides utilities for verifying that external tool
// dependencies (e.g. iverilog, yosys) are available on the system PATH.
package tools

import (
	"fmt"
	"os/exec"
)

// CheckTool verifies that the executable named by name is present on the
// system PATH. It returns a descriptive error if the tool cannot be found.
func CheckTool(name string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s: not found on PATH – please install it before continuing", name)
	}
	return nil
}

// CheckToolWithHint works like CheckTool but appends hint to the error
// message so the user knows how to install the missing dependency.
//
// Example:
//
//	CheckToolWithHint("iverilog", "Install Icarus Verilog: https://steveicarus.github.io/iverilog/")
func CheckToolWithHint(name, hint string) error {
	_, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("%s: not found on PATH – %s", name, hint)
	}
	return nil
}

// CheckTools verifies that every tool in names is available on the system
// PATH. It returns the first error encountered, or nil if all tools are found.
func CheckTools(names ...string) error {
	for _, name := range names {
		if err := CheckTool(name); err != nil {
			return err
		}
	}
	return nil
}
