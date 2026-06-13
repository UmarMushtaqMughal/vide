// Package templates provides Go string constants for Verilog and SystemVerilog
// boilerplate code, along with a testbench template renderer.
package templates

import (
	"bytes"
	"text/template"
)

// BoilerplateV is a basic Verilog module template.
// Use fmt.Sprintf(BoilerplateV, moduleName) to fill in the module name.
const BoilerplateV = "`timescale 1ns/1ps\n" +
	"\n" +
	"module %s (\n" +
	"    // Add ports here\n" +
	");\n" +
	"\n" +
	"    // Logic here\n" +
	"\n" +
	"endmodule\n"

// BoilerplateSV is a basic SystemVerilog module template.
// Use fmt.Sprintf(BoilerplateSV, moduleName) to fill in the module name.
const BoilerplateSV = "`timescale 1ns/1ps\n" +
	"\n" +
	"module %s (\n" +
	"    // Add ports here (e.g. input logic [7:0] data)\n" +
	");\n" +
	"\n" +
	"    // Logic here\n" +
	"\n" +
	"endmodule\n"

// TBTemplate is a testbench template using text/template syntax.
const TBTemplate = "`timescale 1ns/1ps\n" +
	"\n" +
	"module {{.TBName}};\n" +
	"\n" +
	"    // Signals\n" +
	"    {{.SignalDecls}}\n" +
	"\n" +
	"    // Instantiate UUT\n" +
	"    {{.ModuleName}} uut (\n" +
	"        {{.InstancePorts}}\n" +
	"    );\n" +
	"{{if .HasClock}}\n" +
	"    // Clock\n" +
	"    initial begin\n" +
	"        {{.ClockName}} = 0;\n" +
	"        forever #5 {{.ClockName}} = ~{{.ClockName}};\n" +
	"    end\n" +
	"{{end}}\n" +
	"    initial begin\n" +
	"        $dumpfile(\"{{.ModuleName}}.vcd\");\n" +
	"        $dumpvars(1, {{.TBName}});\n" +
	"{{if .InitInputs}}\n" +
	"        // Initialize\n" +
	"        {{.InitInputs}}\n" +
	"{{end}}" +
	"{{if .HasReset}}\n" +
	"        // Reset\n" +
	"        {{.ResetName}} = {{if .ResetActiveLow}}0{{else}}1{{end}};\n" +
	"        #20;\n" +
	"        {{.ResetName}} = {{if .ResetActiveLow}}1{{else}}0{{end}};\n" +
	"{{end}}\n" +
	"        // Stimulus\n" +
	"{{if .StimulusLines}}" +
	"        {{.StimulusLines}}\n" +
	"{{else}}" +
	"        #10000;\n" +
	"{{end}}\n" +
	"        $finish;\n" +
	"    end\n" +
	"\n" +
	"endmodule\n"

// TBTemplateData holds the fields required by TBTemplate.
type TBTemplateData struct {
	TBName         string // Name of the testbench module (e.g. "counter_tb").
	ModuleName     string // Name of the unit under test (e.g. "counter").
	SignalDecls    string // Signal declarations (e.g. "reg clk;\nreg rst_n;\nwire [7:0] out;").
	InstancePorts  string // Port connections (e.g. ".clk(clk),\n        .rst_n(rst_n)").
	InitInputs     string // Initialization statements (e.g. "clk = 0;\n        rst_n = 0;").
	HasClock       bool   // True if the module has a clock signal.
	ClockName      string // The name of the clock signal.
	HasReset       bool   // True if the module has a reset signal.
	ResetName      string // The name of the reset signal.
	ResetActiveLow bool   // True if the reset is active low (e.g. rst_n).
	StimulusLines  string // Multi-bit signal stimulus (assignments + delays).
}

// tbTmpl is the parsed testbench template, compiled once at package init.
var tbTmpl = template.Must(template.New("testbench").Parse(TBTemplate))

// RenderTB executes TBTemplate with the supplied data and returns the
// rendered testbench source as a string.
func RenderTB(data TBTemplateData) (string, error) {
	var buf bytes.Buffer
	if err := tbTmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
