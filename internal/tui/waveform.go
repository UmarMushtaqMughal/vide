package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/UmarMushtaqMughal/vide/internal/parser"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

type AnimationTickMsg struct{}

// WaveformView renders parsed VCD signals as ASCII waveforms.
type WaveformView struct {
	signals     []parser.VCDSignal
	endTime     uint64
	offset      int               // horizontal scroll position (time units)
	zoom        int               // time units per character column
	width       int               // available render width in characters
	height      int               // available render height in lines
	scrollY     int               // vertical scroll offset for many signals
	selectedIdx int               // currently selected signal index
	formats     map[string]string // maps signal name to format: "hex", "bin", "udec", "dec"
	timescale   string            // the time unit from the VCD file
	waveCursor  int               // character offset of the time marker in the trace pane
	traceWidth  int               // cached trace width from last render

	// Smooth scrolling physics
	spring        harmonica.Spring
	currentOffset float64
	targetOffset  float64
	velocity      float64
	Animating     bool
}

// NewWaveformView returns a WaveformView with sensible defaults.
func NewWaveformView() WaveformView {
	return WaveformView{
		zoom:    1,
		width:   80,
		height:  10,
		formats: make(map[string]string),
		// 60fps, 6.0Hz frequency, 1.0 damping (critically damped)
		spring: harmonica.NewSpring(harmonica.FPS(60), 6.0, 1.0),
	}
}

func (w *WaveformView) SetData(data *parser.VCDData) {
	if data == nil {
		return
	}
	w.signals = data.Signals
	w.endTime = data.EndTime
	w.timescale = data.Timescale
	w.offset = 0
	w.currentOffset = 0
	w.targetOffset = 0
	w.scrollY = 0

	// Auto-fit zoom so the full waveform is visible initially.
	if w.endTime > 0 && w.width > 0 {
		w.zoom = int(w.endTime) / w.width
		if w.zoom < 1 {
			w.zoom = 1
		}
	}
}

// ApplyChunk merges a streamed VCD chunk into the waveform view.
func (w *WaveformView) ApplyChunk(chunk parser.VCDChunk) {
	if chunk.EndTime > w.endTime {
		w.endTime = chunk.EndTime
	}
	for idx, waveUpdate := range chunk.Updates {
		if idx < len(w.signals) {
			w.signals[idx].Wave.Times = append(w.signals[idx].Wave.Times, waveUpdate.Times...)
			w.signals[idx].Wave.Values = append(w.signals[idx].Wave.Values, waveUpdate.Values...)
		}
	}

	// Update zoom dynamically if we're still loading and haven't zoomed manually
	if w.endTime > 0 && w.width > 0 {
		newZoom := int(w.endTime) / w.width
		if newZoom > w.zoom {
			w.zoom = newZoom
		}
	}
}

// SetSize updates the available render area.
func (w *WaveformView) SetSize(width, height int) {
	if width > 0 {
		w.width = width
	}
	if height > 0 {
		w.height = height
	}
}

// TickAnimation returns a tea.Cmd to trigger the next animation frame.
func (w *WaveformView) TickAnimation() tea.Cmd {
	return tea.Tick(time.Second/60, func(time.Time) tea.Msg {
		return AnimationTickMsg{}
	})
}

// ScrollLeft pans the waveform view left by one zoom unit.
func (w *WaveformView) ScrollLeft() tea.Cmd {
	w.targetOffset -= float64(w.zoom)
	if w.targetOffset < 0 {
		w.targetOffset = 0
	}
	w.Animating = true
	return w.TickAnimation()
}

// ScrollRight pans the waveform view right by one zoom unit.
func (w *WaveformView) ScrollRight() tea.Cmd {
	w.targetOffset += float64(w.zoom)
	maxOffset := float64(int(w.endTime) - w.width*w.zoom)
	if maxOffset < 0 {
		maxOffset = 0
	}
	if w.targetOffset > maxOffset {
		w.targetOffset = maxOffset
	}
	w.Animating = true
	return w.TickAnimation()
}

// ZoomIn decreases the time units per column (minimum 1).
func (w *WaveformView) ZoomIn() {
	w.zoom /= 2
	if w.zoom < 1 {
		w.zoom = 1
	}
}

// ZoomOut increases the time units per column (max endTime/2).
func (w *WaveformView) ZoomOut() {
	w.zoom *= 2
	maxZoom := int(w.endTime) / 2
	if maxZoom < 1 {
		maxZoom = 1
	}
	if w.zoom > maxZoom {
		w.zoom = maxZoom
	}
}

// SelectUp moves the selection cursor up.
func (w *WaveformView) SelectUp() {
	if w.selectedIdx > 0 {
		w.selectedIdx--
	}
	if w.selectedIdx < w.scrollY {
		w.scrollY = w.selectedIdx
	}
}

// SelectDown moves the selection cursor down.
func (w *WaveformView) SelectDown() {
	if len(w.signals) == 0 {
		return
	}
	if w.selectedIdx < len(w.signals)-1 {
		w.selectedIdx++
	}
	// Note: w.height is reduced by 1 because of the ruler
	if w.selectedIdx >= w.scrollY+(w.height-1) {
		w.scrollY = w.selectedIdx - (w.height - 1) + 1
	}
}

// CycleFormat changes the format of the currently selected signal.
func (w *WaveformView) CycleFormat() {
	if len(w.signals) == 0 {
		return
	}
	sig := w.signals[w.selectedIdx]
	cur := w.formats[sig.Name]
	switch cur {
	case "", "hex":
		w.formats[sig.Name] = "bin"
	case "bin":
		w.formats[sig.Name] = "udec"
	case "udec":
		w.formats[sig.Name] = "dec"
	case "dec":
		w.formats[sig.Name] = "hex"
	}
}

// CursorLeft moves the time inspection marker left.
func (w *WaveformView) CursorLeft() {
	if w.waveCursor > 0 {
		w.waveCursor--
	} else {
		w.ScrollLeft()
	}
}

// CursorRight moves the time inspection marker right.
func (w *WaveformView) CursorRight() {
	// We'll approximate traceWidth here, it gets clamped in Render anyway.
	w.waveCursor++
	// Scroll happens in Render if waveCursor exceeds traceWidth.
}

// EdgeLeft moves the time cursor to the previous value transition.
func (w *WaveformView) EdgeLeft() {
	if len(w.signals) == 0 || w.selectedIdx >= len(w.signals) {
		return
	}
	sig := w.signals[w.selectedIdx]
	cursorTime := uint64(w.offset + w.waveCursor*w.zoom)

	var targetTime uint64 = 0
	if sig.Wave != nil {
		for i := len(sig.Wave.Times) - 1; i >= 0; i-- {
			if sig.Wave.Times[i] < cursorTime {
				targetTime = sig.Wave.Times[i]
				break
			}
		}
	}
	w.snapToTime(targetTime)
}

// EdgeRight moves the time cursor to the next value transition.
func (w *WaveformView) EdgeRight() {
	if len(w.signals) == 0 || w.selectedIdx >= len(w.signals) {
		return
	}
	sig := w.signals[w.selectedIdx]
	cursorTime := uint64(w.offset + w.waveCursor*w.zoom)

	var targetTime uint64 = w.endTime
	if sig.Wave != nil {
		for _, t := range sig.Wave.Times {
			if t > cursorTime {
				targetTime = t
				break
			}
		}
	}
	w.snapToTime(targetTime)
}

func (w *WaveformView) snapToTime(t uint64) {
	if w.traceWidth == 0 {
		w.traceWidth = 10 // safe fallback
	}
	// Try to place the cursor in the middle of the screen if it's far away
	w.offset = int(t) - (w.traceWidth/2)*w.zoom
	if w.offset < 0 {
		w.offset = 0
	}
	w.waveCursor = (int(t) - w.offset) / w.zoom
	if w.waveCursor < 0 {
		w.waveCursor = 0
	}
}

// ---------------------------------------------------------------------------
// Rendering
// ---------------------------------------------------------------------------

// Render produces the ASCII waveform view as a styled string.
func (w WaveformView) Render() string {
	if len(w.signals) == 0 {
		pad := ""
		if w.width > 0 {
			msg := "No waveform data. Press 's' to simulate."
			left := (w.width - len(msg)) / 2
			if left < 0 {
				left = 0
			}
			pad = strings.Repeat(" ", left) + msg
		}
		return pad
	}

	// Determine label column width.
	nameWidth := 0
	for _, sig := range w.signals {
		if len(sig.Name) > nameWidth {
			nameWidth = len(sig.Name)
		}
	}
	nameWidth += 12 // padding + space for " = value"

	traceWidth := w.width - nameWidth
	if traceWidth < 1 {
		traceWidth = 1
	}
	w.traceWidth = traceWidth

	// Clamp waveCursor
	if w.waveCursor >= traceWidth {
		diff := w.waveCursor - traceWidth + 1
		w.offset += diff * w.zoom
		w.waveCursor = traceWidth - 1
	}
	if w.waveCursor < 0 {
		w.waveCursor = 0
	}

	// Visible signal window (leave 2 lines for ruler)
	maxSignals := (w.height - 2) / 2
	if maxSignals < 1 {
		maxSignals = 1
	}
	start := w.scrollY
	end := start + maxSignals
	if end > len(w.signals) {
		end = len(w.signals)
	}
	if start > len(w.signals) {
		start = len(w.signals)
	}

	var lines []string

	// Add Time Ruler
	rulerLabel := StyleWaveBus.Render(padRight("Time", nameWidth))
	lines = append(lines, rulerLabel+w.renderRuler(traceWidth))

	for i := start; i < end; i++ {
		sig := w.signals[i]

		format := w.formats[sig.Name]
		if format == "" {
			format = "hex"
		}

		// Exact value at cursor
		cursorTime := uint64(w.offset + w.waveCursor*w.zoom)
		valAtCursor := lookupValue(sig, cursorTime)
		valDisp := busFormat(valAtCursor, format)

		nameDisp := sig.Name
		if i == w.selectedIdx {
			nameDisp = "> " + nameDisp + " = " + valDisp
		} else {
			nameDisp = "  " + nameDisp + " = " + valDisp
		}

		label := StyleWaveBus.Render(padRight(nameDisp, nameWidth))
		if i == w.selectedIdx {
			label = StyleActiveFile.Render(padRight(nameDisp, nameWidth))
		}

		trace := w.renderSignalTrace(sig, traceWidth, format)
		lines = append(lines, label+trace)
		lines = append(lines, "") // Space between traces
	}

	return strings.Join(lines, "\n")
}

func (w WaveformView) renderRuler(traceWidth int) string {
	var b strings.Builder

	// Strip the number from timescale if we just want the unit, but often Timescale is "1ps".
	// We'll just display it directly: e.g. "10(1ps)" or we can just append it: "10ps".
	unit := w.timescale
	if unit != "" {
		// Clean up spacing if it's like "1 ps"
		unit = strings.ReplaceAll(unit, " ", "")
		// Strip the '1' if it's '1ps', '1ns', etc. so we just show 'ps', 'ns'
		unit = strings.TrimPrefix(unit, "1")
	}

	for col := 0; col < traceWidth; col++ {
		t := uint64(w.offset + col*w.zoom)
		char := " "

		if col%10 == 0 {
			ts := fmt.Sprintf("%d%s", t, unit)
			if col+len(ts) <= traceWidth && col != w.waveCursor {
				b.WriteString(StyleFileName.Render(ts))
				col += len(ts) - 1
				continue
			} else {
				char = "│"
			}
		} else if col%5 == 0 {
			char = "│"
		}

		if col == w.waveCursor {
			b.WriteString(StyleActiveFile.Render("#"))
		} else if char != " " {
			b.WriteString(StyleFileName.Render(char))
		} else {
			b.WriteString(" ")
		}
	}
	return b.String()
}

// renderSignalTrace draws a single signal across traceWidth columns.
func (w WaveformView) renderSignalTrace(sig parser.VCDSignal, traceWidth int, format string) string {
	if sig.Width <= 1 {
		return w.render1BitTrace(sig, traceWidth)
	}
	return w.renderBusTrace(sig, traceWidth, format)
}

// render1BitTrace renders a 1-bit signal using box-drawing characters.
func (w WaveformView) render1BitTrace(sig parser.VCDSignal, traceWidth int) string {
	var b strings.Builder
	b.Grow(traceWidth * 4) // UTF-8 chars can be multi-byte

	prevVal := lookupValue(sig, uint64(w.offset))

	for col := 0; col < traceWidth; col++ {
		t := uint64(w.offset + col*w.zoom)
		val := lookupValue(sig, t)

		charStr := ""
		if val != prevVal {
			charStr = "│" // Thin vertical line
		} else if val == "1" {
			charStr = "‾" // Thin overline
		} else {
			charStr = "_" // Thin underscore
		}

		style := StyleWaveLow
		if val == "1" || (val != prevVal && (val == "1" || prevVal == "1")) {
			style = StyleWaveHigh
		}

		if col == w.waveCursor {
			style = style.Copy().Background(lipgloss.Color("236"))
		}

		b.WriteString(style.Render(charStr))
		prevVal = val
	}
	return b.String()
}

// renderBusTrace renders a multi-bit bus signal.
func (w WaveformView) renderBusTrace(sig parser.VCDSignal, traceWidth int, format string) string {
	var b strings.Builder
	b.Grow(traceWidth * 4)

	prevVal := lookupValue(sig, uint64(w.offset))

	for col := 0; col < traceWidth; col++ {
		t := uint64(w.offset + col*w.zoom)
		val := lookupValue(sig, t)

		charStr := ""
		skip := 0
		if val != prevVal {
			// Transition – show value of the new state.
			disp := busFormat(val, format)
			charStr = "╪" + disp // ╪ + val
			skip = len(disp)
		} else {
			charStr = "═" // ═
		}

		style := StyleWaveBus
		if col == w.waveCursor {
			style = style.Copy().Background(lipgloss.Color("236"))
		}

		b.WriteString(style.Render(charStr))

		if skip > 0 {
			// Adjust column, but watch cursor position!
			col += skip
		}

		prevVal = val
	}
	return b.String()
}

// lookupValue finds the signal value at a given time using the sparse SignalWave structure.
func lookupValue(sig parser.VCDSignal, t uint64) string {
	if sig.Wave == nil {
		return "x"
	}
	return sig.Wave.ValueAt(t)
}

// busFormat converts a binary string value to the requested representation.
func busFormat(val string, format string) string {
	if len(val) <= 1 {
		return val
	}

	if format == "bin" {
		return val
	}

	n := uint64(0)
	for _, ch := range val {
		n <<= 1
		if ch == '1' {
			n |= 1
		} else if ch != '0' {
			return val // contains x/z, return as-is
		}
	}

	switch format {
	case "dec":
		// Assume signed? Let's just do signed based on highest bit if needed, but for now decimal:
		if val[0] == '1' {
			// 2s complement
			mask := (uint64(1) << len(val)) - 1
			inv := (^n) & mask
			return fmt.Sprintf("-%d", inv+1)
		}
		return fmt.Sprintf("%d", n)
	case "udec":
		return fmt.Sprintf("%d", n)
	case "hex":
		return fmt.Sprintf("%X", n)
	default:
		return fmt.Sprintf("%X", n)
	}
}

// padRight pads s with spaces on the right up to width.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}
