package parser

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestParseVCDStream(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	parser, err := NewVCDStreamParser(vcdPath)
	if err != nil {
		t.Fatalf("unexpected initialization error: %v", err)
	}
	defer parser.Close()

	ch := parser.Stream(ctx, 100)
	
	var allChunks []VCDChunk
	for chunk := range ch {
		if chunk.Err != nil && chunk.Err.Error() != "EOF" {
			t.Fatalf("unexpected chunk error: %v", chunk.Err)
		}
		allChunks = append(allChunks, chunk)
	}

	if parser.Data.Timescale != "1ns" {
		t.Errorf("expected timescale '1ns', got '%s'", parser.Data.Timescale)
	}

	var clkChanges []ValueChange
	var countChanges []ValueChange

	for _, chunk := range allChunks {
		for sigIdx, wave := range chunk.Updates {
			sigName := parser.Data.Signals[sigIdx].Name
			for i := 0; i < len(wave.Times); i++ {
				vc := ValueChange{Time: int64(wave.Times[i]), Value: wave.Values[i]}
				if sigName == "tb/clk" {
					clkChanges = append(clkChanges, vc)
				} else if sigName == "tb/count" {
					countChanges = append(countChanges, vc)
				}
			}
		}
	}

	expectedClk := []ValueChange{
		{Time: 0, Value: "0"},
		{Time: 10, Value: "1"},
		{Time: 20, Value: "0"},
	}

	if len(clkChanges) != len(expectedClk) {
		t.Fatalf("expected %d clk changes, got %d", len(expectedClk), len(clkChanges))
	}

	for i, c := range clkChanges {
		if c.Time != expectedClk[i].Time || c.Value != expectedClk[i].Value {
			t.Errorf("clk change %d: expected %+v, got %+v", i, expectedClk[i], c)
		}
	}
}

type ValueChange struct {
	Time  int64
	Value string
}
