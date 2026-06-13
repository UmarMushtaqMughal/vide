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
		signalDecls = strings.Join(regs, "\n    ") + "\n\n    " + strings.Join(wires, "\n    ")
	}

	// Build instance port connections
	allPorts := append(inputs, outputs...)
	var portConns []string
	for _, p := range allPorts {
		portConns = append(portConns, fmt.Sprintf(".%s(%s)", p.Name, p.Name))
	}
	instancePorts := strings.Join(portConns, ",\n        ")

	// Build input initializations (skip clk and rst_n)
	var inits []string
	for _, p := range inputs {
		if p.Name != "clk" && p.Name != "rst_n" {
			inits = append(inits, fmt.Sprintf("%s = 0;", p.Name))
		}
	}
	initInputs := strings.Join(inits, "\n        ")

	data := templates.TBTemplateData{
		TBName:        tbName,
		ModuleName:    moduleName,
		SignalDecls:   signalDecls,
		InstancePorts: instancePorts,
		InitInputs:    initInputs,
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
