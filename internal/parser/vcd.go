package parser

import (
	"bufio"
	"fmt"
	"math/big"
	"os"
	"strconv"
	"strings"
)

// VCDSignal represents a single signal tracked in a VCD file.
type VCDSignal struct {
	Name    string        // hierarchical name, e.g. "tb/uut/clk"
	Width   int           // bit width
	ID      string        // single-char (or short) identifier from VCD
	Changes []ValueChange // value change history, in chronological order
}

// ValueChange records a single value transition at a given simulation time.
type ValueChange struct {
	Time  uint64
	Value string // "0", "1", "x", "z" for 1-bit; uppercase hex string for multi-bit
}

// VCDData holds the complete parsed contents of a VCD file.
type VCDData struct {
	Timescale string      // e.g. "1ns", "10ps"
	Signals   []VCDSignal // signals in order of first appearance
	EndTime   uint64      // last timestamp encountered
}

// ParseVCD reads and parses an IEEE 1364 VCD file, returning the extracted
// signal data and timing information.
//
// It handles:
//   - $timescale sections
//   - $var declarations for wire and reg types
//   - $scope / $upscope hierarchy (joined with "/" separators)
//   - Value change records: 1-bit (value+id) and multi-bit (b... id)
//   - Malformed or unexpected lines are silently skipped
func ParseVCD(filename string) (*VCDData, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("opening VCD file: %w", err)
	}
	defer f.Close()

	data := &VCDData{}

	// idToIndex maps VCD identifier codes to their index in data.Signals,
	// preserving first-appearance order.
	idToIndex := make(map[string]int)

	// scopeStack tracks the current hierarchy for signal naming.
	var scopeStack []string

	// currentTime tracks the simulation timestamp for value changes.
	var currentTime uint64

	// inDefinitions is true while we are in the header/definition section.
	inDefinitions := true

	// Accumulator for multi-line $timescale parsing.
	var inTimescale bool
	var timescaleBuf strings.Builder

	scanner := bufio.NewScanner(f)
	// Increase buffer size for VCD files with very long lines.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		// ── Timescale parsing (may span multiple lines) ──
		if inTimescale {
			if strings.Contains(line, "$end") {
				// Extract content before $end on this line.
				before, _, _ := strings.Cut(line, "$end")
				timescaleBuf.WriteString(" ")
				timescaleBuf.WriteString(strings.TrimSpace(before))
				data.Timescale = strings.TrimSpace(timescaleBuf.String())
				inTimescale = false
			} else {
				timescaleBuf.WriteString(" ")
				timescaleBuf.WriteString(line)
			}
			continue
		}

		// ── Header/definition keywords ──
		if strings.HasPrefix(line, "$timescale") {
			rest := strings.TrimPrefix(line, "$timescale")
			if idx := strings.Index(rest, "$end"); idx >= 0 {
				// Single-line timescale.
				data.Timescale = strings.TrimSpace(rest[:idx])
			} else {
				// Multi-line timescale.
				inTimescale = true
				timescaleBuf.Reset()
				timescaleBuf.WriteString(strings.TrimSpace(rest))
			}
			continue
		}

		if strings.HasPrefix(line, "$scope") {
			// Format: $scope module <name> $end
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				scopeStack = append(scopeStack, fields[2])
			}
			continue
		}

		if strings.HasPrefix(line, "$upscope") {
			if len(scopeStack) > 0 {
				scopeStack = scopeStack[:len(scopeStack)-1]
			}
			continue
		}

		if strings.HasPrefix(line, "$var") {
			parseVarLine(line, scopeStack, data, idToIndex)
			continue
		}

		if strings.HasPrefix(line, "$enddefinitions") {
			inDefinitions = false
			continue
		}

		// Skip other header keywords.
		if strings.HasPrefix(line, "$") {
			continue
		}

		// ── Value change section ──
		if inDefinitions {
			continue
		}

		if line[0] == '#' {
			// Timestamp line.
			ts, err := strconv.ParseUint(line[1:], 10, 64)
			if err == nil {
				currentTime = ts
				if ts > data.EndTime {
					data.EndTime = ts
				}
			}
			continue
		}

		// 1-bit value change: single char value followed by identifier.
		// Format: <value><id>  where value is 0, 1, x, X, z, Z
		if len(line) >= 2 && is1BitValue(line[0]) {
			val := strings.ToLower(string(line[0]))
			id := line[1:]
			appendChange(data, idToIndex, id, currentTime, val)
			continue
		}

		// Multi-bit value change: b<binary> <id>  or  B<binary> <id>
		if (line[0] == 'b' || line[0] == 'B') && len(line) > 1 {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				binStr := parts[0][1:] // strip leading 'b'/'B'
				id := parts[1]
				appendChange(data, idToIndex, id, currentTime, binStr)
			}
			continue
		}

		// Real value change: r<value> <id>  or  R<value> <id>
		if (line[0] == 'r' || line[0] == 'R') && len(line) > 1 {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				id := parts[1]
				realVal := parts[0][1:] // strip leading 'r'/'R'
				appendChange(data, idToIndex, id, currentTime, realVal)
			}
			continue
		}

		// Unknown line format — skip silently.
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading VCD file: %w", err)
	}

	return data, nil
}

// parseVarLine parses a $var declaration line and registers the signal.
// Format: $var wire|reg <width> <id> <name> [<array>] $end
func parseVarLine(line string, scopeStack []string, data *VCDData, idToIndex map[string]int) {
	fields := strings.Fields(line)
	// Minimum fields: $var wire 1 ! clk $end  → 6 fields
	if len(fields) < 6 {
		return
	}

	varType := fields[1]
	if varType != "wire" && varType != "reg" && varType != "integer" &&
		varType != "real" && varType != "parameter" && varType != "event" {
		return
	}

	width, err := strconv.Atoi(fields[2])
	if err != nil || width < 1 {
		return
	}

	id := fields[3]
	name := fields[4]

	// Build hierarchical name.
	var fullName string
	if len(scopeStack) > 0 {
		fullName = strings.Join(scopeStack, "/") + "/" + name
	} else {
		fullName = name
	}

	// Only register each ID once (first occurrence wins).
	if _, exists := idToIndex[id]; !exists {
		idToIndex[id] = len(data.Signals)
		data.Signals = append(data.Signals, VCDSignal{
			Name:  fullName,
			Width: width,
			ID:    id,
		})
	}
}

// appendChange adds a value change to the signal identified by id.
func appendChange(data *VCDData, idToIndex map[string]int, id string, time uint64, value string) {
	idx, ok := idToIndex[id]
	if !ok {
		return
	}
	data.Signals[idx].Changes = append(data.Signals[idx].Changes, ValueChange{
		Time:  time,
		Value: value,
	})
}

// is1BitValue returns true if ch is a valid 1-bit VCD value character.
func is1BitValue(ch byte) bool {
	return ch == '0' || ch == '1' ||
		ch == 'x' || ch == 'X' ||
		ch == 'z' || ch == 'Z'
}

// binaryToHex converts a VCD binary string (which may contain x and z) to a
// hex representation. Pure binary digits are converted numerically; strings
// containing x or z are converted nibble-by-nibble.
func binaryToHex(bin string) string {
	lower := strings.ToLower(bin)

	// If the string contains x or z, handle nibble-by-nibble.
	if strings.ContainsAny(lower, "xz") {
		return binaryToHexWithXZ(lower)
	}

	// Pure binary → big.Int → uppercase hex.
	n := new(big.Int)
	n, ok := n.SetString(lower, 2)
	if !ok {
		// Fallback: return the raw binary if parsing fails.
		return bin
	}
	hex := strings.ToUpper(n.Text(16))
	if hex == "" {
		return "0"
	}
	return hex
}

// binaryToHexWithXZ converts a binary string that may contain x/z to hex,
// processing 4 bits at a time. A nibble that is all-x becomes "X", all-z
// becomes "Z". Mixed nibbles are left as-is with a leading "x".
func binaryToHexWithXZ(bin string) string {
	// Pad to a multiple of 4 bits.
	padLen := (4 - len(bin)%4) % 4
	padded := strings.Repeat("0", padLen) + bin

	var result strings.Builder
	for i := 0; i < len(padded); i += 4 {
		nibble := padded[i : i+4]
		if nibble == "xxxx" {
			result.WriteByte('X')
		} else if nibble == "zzzz" {
			result.WriteByte('Z')
		} else if strings.ContainsAny(nibble, "xz") {
			// Mixed nibble — can't represent as clean hex.
			result.WriteString("x")
		} else {
			val, err := strconv.ParseUint(nibble, 2, 8)
			if err != nil {
				result.WriteString("?")
			} else {
				result.WriteString(strings.ToUpper(fmt.Sprintf("%x", val)))
			}
		}
	}

	hex := result.String()
	if hex == "" {
		return "0"
	}
	return hex
}
