package parser

import (
	"testing"
)

func TestParsePorts(t *testing.T) {
	code := `
module test_module (
    input logic clk,
    input logic rst_n,
    input logic [7:0] data_in,
    output logic [15:0] data_out
);
    // logic
endmodule
`
	moduleName, inputs, outputs, err := ParsePorts(code)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if moduleName != "test_module" {
		t.Errorf("expected moduleName 'test_module', got '%s'", moduleName)
	}

	expectedInputs := []struct {
		Name  string
		Width string
	}{
		{"clk", ""},
		{"rst_n", ""},
		{"data_in", "[7:0]"},
	}

	if len(inputs) != len(expectedInputs) {
		t.Fatalf("expected %d inputs, got %d", len(expectedInputs), len(inputs))
	}

	for i, expected := range expectedInputs {
		if inputs[i].Name != expected.Name {
			t.Errorf("input %d name: expected '%s', got '%s'", i, expected.Name, inputs[i].Name)
		}
		if inputs[i].Width != expected.Width {
			t.Errorf("input %d width: expected '%s', got '%s'", i, expected.Width, inputs[i].Width)
		}
	}

	if len(outputs) != 1 {
		t.Fatalf("expected 1 output, got %d", len(outputs))
	}
	if outputs[0].Name != "data_out" {
		t.Errorf("output name: expected 'data_out', got '%s'", outputs[0].Name)
	}
	if outputs[0].Width != "[15:0]" {
		t.Errorf("output width: expected '[15:0]', got '%s'", outputs[0].Width)
	}
}
