package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// Port represents a single port in a Verilog/SystemVerilog module declaration.
type Port struct {
	Direction string // "input", "output", "inout"
	Width     string // e.g. "[7:0]" or "" for single-bit
	Name      string // e.g. "clk", "data"
}

// Instance represents a sub-module instantiation.
type Instance struct {
	ModuleName   string
	InstanceName string
}

// moduleHeaderRe matches `module <name> (...);` with DOTALL semantics.
// Group 1: module name, Group 2: port list body.
var moduleHeaderRe = regexp.MustCompile(`(?s)module\s+(\w+)\s*\((.*?)\)\s*;`)

// lineCommentRe matches single-line comments.
var lineCommentRe = regexp.MustCompile(`//[^\n]*`)

// blockCommentRe matches block comments.
var blockCommentRe = regexp.MustCompile(`(?s)/\*.*?\*/`)

// widthRe matches a bus width specifier like [7:0].
var widthRe = regexp.MustCompile(`\[\s*\d+\s*:\s*\d+\s*\]`)

// directionRe splits the port list on direction keywords, keeping the keyword.
// It matches input/output/inout that appear as standalone words.
var directionRe = regexp.MustCompile(`\b(input|output|inout)\b`)

// ParsePorts parses a Verilog/SystemVerilog source string and extracts the
// module name along with its input and output port declarations.
//
// It handles:
//   - Single-line (//) and block (/* */) comments
//   - Optional type keywords: logic, reg, wire, signed, unsigned
//   - Optional width specifiers like [7:0]
//   - Multi-port declarations such as `input a, b, c`
//   - Inout ports (returned in the outputs slice for convenience)
func ParsePorts(code string) (moduleName string, inputs []Port, outputs []Port, err error) {
	// Strip comments so they don't interfere with parsing.
	cleaned := blockCommentRe.ReplaceAllString(code, " ")
	cleaned = lineCommentRe.ReplaceAllString(cleaned, " ")

	m := moduleHeaderRe.FindStringSubmatch(cleaned)
	if m == nil {
		return "", nil, nil, fmt.Errorf("no module declaration found")
	}

	moduleName = m[1]
	portBody := m[2]

	// Split the port body into segments by direction keyword.
	// directionRe.FindAllStringIndex gives us the positions of each keyword.
	locs := directionRe.FindAllStringIndex(portBody, -1)
	if len(locs) == 0 {
		// No direction keywords found — could be an empty or ANSI-style port
		// list with no explicit directions. Return empty.
		return moduleName, nil, nil, nil
	}

	// Build segments: each starts at a direction keyword and ends where the
	// next one begins (or at the end of portBody).
	type segment struct {
		direction string
		body      string
	}
	var segments []segment
	for i, loc := range locs {
		dir := portBody[loc[0]:loc[1]]
		start := loc[1]
		var end int
		if i+1 < len(locs) {
			end = locs[i+1][0]
		} else {
			end = len(portBody)
		}
		segments = append(segments, segment{
			direction: dir,
			body:      portBody[start:end],
		})
	}

	for _, seg := range segments {
		ports := parsePortSegment(seg.direction, seg.body)
		for _, p := range ports {
			switch p.Direction {
			case "input":
				inputs = append(inputs, p)
			case "output", "inout":
				outputs = append(outputs, p)
			}
		}
	}

	return moduleName, inputs, outputs, nil
}

// parsePortSegment parses a segment of port declarations that share a single
// direction keyword. The body is the text after the keyword (e.g. " logic [7:0] a, b, c").
func parsePortSegment(direction, body string) []Port {
	// Remove optional type keywords.
	s := body
	for _, kw := range []string{"logic", "reg", "wire", "signed", "unsigned"} {
		s = replaceWord(s, kw, "")
	}

	// Extract width if present.
	width := ""
	if wm := widthRe.FindString(s); wm != "" {
		width = normalizeWidth(wm)
		s = widthRe.ReplaceAllString(s, "")
	}

	// Split remaining text by commas to get individual port names.
	parts := strings.Split(s, ",")
	var ports []Port
	for _, part := range parts {
		name := strings.TrimSpace(part)
		// Remove any trailing semicolons or stray characters.
		name = strings.TrimRight(name, "; \t\r\n")
		if name == "" {
			continue
		}
		// If the name still contains spaces (shouldn't normally), take the
		// last token which is the actual identifier.
		if fields := strings.Fields(name); len(fields) > 0 {
			name = fields[len(fields)-1]
		}
		if !isValidIdentifier(name) {
			continue
		}
		ports = append(ports, Port{
			Direction: direction,
			Width:     width,
			Name:      name,
		})
	}
	return ports
}

// replaceWord removes a standalone word from s, replacing it with repl.
func replaceWord(s, word, repl string) string {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(word) + `\b`)
	return re.ReplaceAllString(s, repl)
}

// normalizeWidth cleans up a width specifier by removing internal whitespace.
// "[  7 : 0 ]" becomes "[7:0]".
func normalizeWidth(w string) string {
	w = strings.ReplaceAll(w, " ", "")
	w = strings.ReplaceAll(w, "\t", "")
	return w
}

// isValidIdentifier checks whether s looks like a Verilog identifier
// (starts with a letter or underscore, followed by word characters).
func isValidIdentifier(s string) bool {
	if s == "" {
		return false
	}
	matched, _ := regexp.MatchString(`^[a-zA-Z_]\w*$`, s)
	return matched
}

// ParseHierarchy parses a Verilog string to find sub-module instantiations.
// It returns a list of instances containing the ModuleName and InstanceName.
func ParseHierarchy(code string) []Instance {
	// Strip comments first.
	cleaned := blockCommentRe.ReplaceAllString(code, " ")
	cleaned = lineCommentRe.ReplaceAllString(cleaned, " ")

	// Regex to find instantiations:
	// Matches: module_name #(params) instance_name (
	// Or:      module_name instance_name (
	// We avoid keywords like 'module', 'if', 'for', 'always', 'initial', etc.
	instRe := regexp.MustCompile(`(?m)^\s*([a-zA-Z_]\w*)\s+(?:#\s*\([^)]*\)\s+)?([a-zA-Z_]\w*)\s*\(`)
	
	keywords := map[string]bool{
		"module": true, "if": true, "for": true, "always": true, "initial": true,
		"case": true, "while": true, "task": true, "function": true, "begin": true,
		"end": true, "always_ff": true, "always_comb": true, "always_latch": true,
		"logic": true, "reg": true, "wire": true, "assign": true,
	}

	var instances []Instance
	matches := instRe.FindAllStringSubmatch(cleaned, -1)
	for _, m := range matches {
		modName := m[1]
		instName := m[2]
		if !keywords[modName] {
			instances = append(instances, Instance{
				ModuleName:   modName,
				InstanceName: instName,
			})
		}
	}
	
	return instances
}
