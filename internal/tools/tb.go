package tools

import (
	"fmt"
	"os"
	"strings"

	"github.com/UmarMushtaqMughal/vide/internal/parser"
	"github.com/UmarMushtaqMughal/vide/internal/templates"
)

// GenerateTB creates a testbench for the given target file and returns the generated filename.
func GenerateTB(target string) (string, error) {
	code, err := os.ReadFile(target)
	if err != nil {
		return "", fmt.Errorf("file %s not found", target)
	}

	moduleName, inputs, outputs, err := parser.ParsePorts(string(code))
	if err != nil {
		return "", fmt.Errorf("parsing ports: %w", err)
	}

	isSV := strings.HasSuffix(target, ".sv")
	tbName := moduleName + "_tb"
	tbFile := tbName + ".v"
	if isSV {
		tbFile = tbName + ".sv"
	}

	// Build signal declarations
	var signalDecls string
	if isSV {
		var decls []string
		for _, p := range append(inputs, outputs...) {
			if p.Width != "" {
				decls = append(decls, fmt.Sprintf("logic %s %s;", p.Width, p.Name))
			} else {
				decls = append(decls, fmt.Sprintf("logic %s;", p.Name))
			}
		}
		signalDecls = strings.Join(decls, "\n    ")
	} else {
		var regs, wires []string
		for _, p := range inputs {
			if p.Width != "" {
				regs = append(regs, fmt.Sprintf("reg %s %s;", p.Width, p.Name))
			} else {
				regs = append(regs, fmt.Sprintf("reg %s;", p.Name))
			}
		}
		for _, p := range outputs {
			if p.Width != "" {
				wires = append(wires, fmt.Sprintf("wire %s %s;", p.Width, p.Name))
			} else {
				wires = append(wires, fmt.Sprintf("wire %s;", p.Name))
			}
		}
		// Only add the separator line when both groups are non-empty.
		if len(regs) > 0 && len(wires) > 0 {
			signalDecls = strings.Join(regs, "\n    ") + "\n\n    " + strings.Join(wires, "\n    ")
		} else {
			signalDecls = strings.Join(append(regs, wires...), "\n    ")
		}
	}

	// Build instance port connections (use a safe copy to avoid slice aliasing).
	allPorts := make([]parser.Port, len(inputs)+len(outputs))
	copy(allPorts, inputs)
	copy(allPorts[len(inputs):], outputs)
	var portConns []string
	for _, p := range allPorts {
		portConns = append(portConns, fmt.Sprintf(".%s(%s)", p.Name, p.Name))
	}
	instancePorts := strings.Join(portConns, ",\n        ")

	// Detect clock and reset signals
	var clkName, rstName string
	var hasClock, hasReset, rstActiveLow bool

	for _, p := range inputs {
		name := strings.ToLower(p.Name)
		if !hasClock && (strings.Contains(name, "clk") || strings.Contains(name, "clock")) {
			hasClock = true
			clkName = p.Name
		}
		if !hasReset && (strings.Contains(name, "rst") || strings.Contains(name, "reset")) {
			hasReset = true
			rstName = p.Name
			// Active-low convention: rst_n, nRst, n_rst, resetn, etc.
			if strings.HasSuffix(name, "_n") || strings.HasSuffix(name, "n") ||
				strings.HasPrefix(name, "n_") {
				rstActiveLow = true
			}
		}
	}

	// Build input initializations (skip clk and rst_n)
	var inits []string
	for _, p := range inputs {
		if p.Name != clkName && p.Name != rstName {
			inits = append(inits, fmt.Sprintf("%s = 0;", p.Name))
		}
	}
	initInputs := strings.Join(inits, "\n        ")

	data := templates.TBTemplateData{
		TBName:         tbName,
		ModuleName:     moduleName,
		SignalDecls:    signalDecls,
		InstancePorts:  instancePorts,
		InitInputs:     initInputs,
		HasClock:       hasClock,
		ClockName:      clkName,
		HasReset:       hasReset,
		ResetName:      rstName,
		ResetActiveLow: rstActiveLow,
	}

	result, err := templates.RenderTB(data)
	if err != nil {
		return "", fmt.Errorf("rendering testbench: %w", err)
	}

	if err := os.WriteFile(tbFile, []byte(result), 0644); err != nil {
		return "", fmt.Errorf("writing file: %w", err)
	}

	return tbFile, nil
}
