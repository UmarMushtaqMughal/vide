package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseVCD(t *testing.T) {
	vcdContent := `$date June 13, 2026 $end
$version Icarus Verilog $end
$timescale 1ns $end
$scope module tb $end
$var reg 1 ! clk $end
$var reg 4 " count [3:0] $end
$upscope $end
$enddefinitions $end
#0
$dumpvars
0!
b0000 "
$end
#10
1!
b0001 "
#20
0!
`

	tempDir := t.TempDir()
	vcdPath := filepath.Join(tempDir, "test.vcd")
	if err := os.WriteFile(vcdPath, []byte(vcdContent), 0644); err != nil {
		t.Fatalf("failed to write temp VCD file: %v", err)
	}

	data, err := ParseVCD(vcdPath)
	if err != nil {
		t.Fatalf("unexpected parsing error: %v", err)
	}

	if data.Timescale != "1ns" {
		t.Errorf("expected timescale '1ns', got '%s'", data.Timescale)
	}

	if data.EndTime != 20 {
		t.Errorf("expected EndTime 20, got %d", data.EndTime)
	}

	if len(data.Signals) != 2 {
		t.Fatalf("expected 2 signals, got %d", len(data.Signals))
	}

	// Verify clk signal
	clk := data.Signals[0]
	if clk.Name != "tb/clk" {
		t.Errorf("expected clk name 'tb/clk', got '%s'", clk.Name)
	}
	if clk.Width != 1 {
		t.Errorf("expected clk width 1, got %d", clk.Width)
	}
	if len(clk.Changes) != 3 {
		t.Fatalf("expected 3 clk changes, got %d", len(clk.Changes))
	}
	expectedClk := []ValueChange{
		{Time: 0, Value: "0"},
		{Time: 10, Value: "1"},
		{Time: 20, Value: "0"},
	}
	for i, c := range clk.Changes {
		if c.Time != expectedClk[i].Time || c.Value != expectedClk[i].Value {
			t.Errorf("clk change %d: expected %+v, got %+v", i, expectedClk[i], c)
		}
	}

	// Verify count signal
	count := data.Signals[1]
	if count.Name != "tb/count" {
		t.Errorf("expected count name 'tb/count', got '%s'", count.Name)
	}
	if count.Width != 4 {
		t.Errorf("expected count width 4, got %d", count.Width)
	}
}
