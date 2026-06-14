package tui

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
)

// SafeHighlight wraps Chroma with panic recovery and graceful fallback.
// If highlighting fails for any reason, it returns the raw text unstyled.
func SafeHighlight(lexer chroma.Lexer, formatter chroma.Formatter, style *chroma.Style, src string) (result string, err error) {
	defer func() {
		if r := recover(); r != nil {
			result = src // return plain text, don't crash
			err = fmt.Errorf("highlighter panic on line: %v", r)
		}
	}()

	var buf strings.Builder
	iter, tokenErr := lexer.Tokenise(nil, src)
	if tokenErr != nil {
		return src, tokenErr
	}
	if fmtErr := formatter.Format(&buf, style, iter); fmtErr != nil {
		return src, fmtErr
	}
	return buf.String(), nil
}

// HighlightWithLineFallback attempts whole-file highlighting. If it fails, 
// it falls back to highlighting each line independently.
func HighlightWithLineFallback(content string, lexer chroma.Lexer, fmt chroma.Formatter, style *chroma.Style) []string {
	result, err := SafeHighlight(lexer, fmt, style, content)
	if err == nil {
		return strings.Split(result, "\n")
	}

	// Fallback: highlight each line independently (stateless, but safe)
	lines := strings.Split(content, "\n")
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i], _ = SafeHighlight(lexer, fmt, style, line)
	}
	return out
}
