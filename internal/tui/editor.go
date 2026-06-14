package tui

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/lipgloss"
)

type Editor struct {
	textarea  textarea.Model
	isEditing bool
	filename  string

	// Smooth scrolling state
	targetScroll int
	scrollFloat  float64
	scrollVel    float64
	scrollSpring harmonica.Spring

	width         int
	height        int
	lintErrs      []int
	bufferVersion int
	hlCache       *HighlightCache
	bgHighlighter *BackgroundHighlighter
}

func NewEditor() Editor {
	ta := textarea.New()
	ta.Placeholder = "Write Verilog/SystemVerilog code here..."
	ta.Focus()
	ta.Prompt = "┃ "
	ta.CharLimit = 0
	ta.ShowLineNumbers = false // We render our own
	ta.SetWidth(10000)         // Prevent native wrapping
	ta.SetHeight(10000)

	// Remove default colors so Chroma can take over if we want,
	// but we'll use Chroma mostly for the view mode.
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()

	return Editor{
		textarea:      ta,
		isEditing:     false,
		scrollSpring:  harmonica.NewSpring(harmonica.FPS(60), 6.0, 1.0),
		hlCache:       NewHighlightCache(1000),
		bgHighlighter: NewBackgroundHighlighter(),
	}
}

func (e *Editor) SetContent(content string, filename string) {
	e.filename = filename
	e.textarea.SetValue(content)
	e.bufferVersion++
	e.hlCache.InvalidateAll()
	e.bgHighlighter.Submit(HighlightJob{
		BufferID: e.filename,
		Version:  e.bufferVersion,
		Content:  content,
	})
}

func (e *Editor) GetContent() string {
	return e.textarea.Value()
}

func (e *Editor) SetErrors(errs []int) {
	e.lintErrs = errs
}

func (e *Editor) LineInfo() (line, col, total int) {
	line = e.textarea.Line()
	info := e.textarea.LineInfo()
	col = info.ColumnOffset
	total = e.textarea.LineCount()
	return line + 1, col + 1, total // 1-indexed
}

func getSnippet(word string) (string, bool) {
	snippets := map[string]string{
		"always":  "always_ff @(posedge clk or negedge rst_n) begin\n    if (!rst_n) begin\n        \n    end else begin\n        \n    end\nend",
		"alwaysc": "always_comb begin\n    \nend",
		"module":  "module name (\n    input logic clk,\n    input logic rst_n,\n    output logic out\n);\n    \nendmodule",
		"logic":   "logic [31:0] ",
	}
	s, ok := snippets[word]
	return s, ok
}

func (e *Editor) SetSize(width, height int) {
	e.width = width
	e.height = height
}

func (e *Editor) Focus() {
	e.isEditing = true
	e.textarea.Focus()
}

func (e *Editor) Blur() {
	e.isEditing = false
	e.textarea.Blur()
}

func (e *Editor) Update(msg tea.Msg) (Editor, tea.Cmd) {
	if hlMsg, ok := msg.(HighlightResult); ok {
		if hlMsg.Version < e.bufferVersion {
			return *e, e.bgHighlighter.WaitForResult()
		}
		e.hlCache.ReplaceAll(hlMsg.Lines)
		return *e, nil
	}

	var cmd tea.Cmd
	if !e.isEditing {
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case "up", "k":
				if e.targetScroll > 0 {
					e.targetScroll--
				}
			case "down", "j":
				lines := len(strings.Split(e.textarea.Value(), "\n"))
				maxScroll := lines - e.height
				if maxScroll < 0 {
					maxScroll = 0
				}
				if e.targetScroll < maxScroll {
					e.targetScroll++
				}
			}
			cmd = tea.Batch(cmd, EditorTick())
		}
	} else {
		if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.String() == "tab" {
			lineIdx := e.textarea.Line()
			colIdx := e.textarea.LineInfo().CharOffset
			lines := strings.Split(e.textarea.Value(), "\n")
			word := ""
			if lineIdx < len(lines) {
				l := lines[lineIdx]
				start := colIdx - 1
				for start >= 0 && start < len(l) && ((l[start] >= 'a' && l[start] <= 'z') || (l[start] >= 'A' && l[start] <= 'Z') || l[start] == '_') {
					start--
				}
				start++
				if start >= 0 && start < colIdx && colIdx <= len(l) {
					word = l[start:colIdx]
				}
			}

			if snippet, ok := getSnippet(word); ok {
				for i := 0; i < len(word); i++ {
					// Hacky way to simulate backspace if DeleteCharacterBackward is missing
					// Bubble tea's textarea has DeleteBeforeCursor() or we can send backspace msg.
					// Let's send backspace msg.
					e.textarea, _ = e.textarea.Update(tea.KeyMsg{Type: tea.KeyBackspace})
				}
				e.textarea.InsertString(snippet)
			} else {
				e.textarea.InsertString("    ")
			}
		} else {
			contentBefore := e.textarea.Value()
			e.textarea, cmd = e.textarea.Update(msg)
			contentAfter := e.textarea.Value()

			if contentBefore != contentAfter {
				e.bufferVersion++
				e.hlCache.InvalidateLine(e.textarea.Line())
				e.bgHighlighter.Submit(HighlightJob{
					BufferID: e.filename,
					Version:  e.bufferVersion,
					Content:  contentAfter,
				})
				cmd = tea.Batch(cmd, e.bgHighlighter.WaitForResult())
			}
		}
		// Track cursor to update our custom scroll bounds
		cursorLine := e.textarea.Line()
		if cursorLine < e.targetScroll {
			e.targetScroll = cursorLine
			cmd = tea.Batch(cmd, EditorTick())
		} else if cursorLine >= e.targetScroll+e.height {
			e.targetScroll = cursorLine - e.height + 1
			cmd = tea.Batch(cmd, EditorTick())
		}
	}

	// Animation step
	if math.Abs(float64(e.targetScroll)-e.scrollFloat) > 0.05 || math.Abs(e.scrollVel) > 0.05 {
		e.scrollFloat, e.scrollVel = e.scrollSpring.Update(e.scrollFloat, e.scrollVel, float64(e.targetScroll))
		cmd = tea.Batch(cmd, EditorTick())
	} else {
		e.scrollFloat = float64(e.targetScroll)
		e.scrollVel = 0
	}

	return *e, cmd
}

type EditorTickMsg struct{}

func EditorTick() tea.Cmd {
	return tea.Tick(time.Second/60, func(t time.Time) tea.Msg { return EditorTickMsg{} })
}

func (e *Editor) View() string {
	content := e.textarea.Value()
	if content == "" && !e.isEditing {
		return "No content."
	}

	cursorLine := -1
	cursorCol := -1
	if e.isEditing {
		cursorLine = e.textarea.Line()
		cursorCol = e.textarea.LineInfo().CharOffset
	}

	return renderCustomView(e, content, e.width, e.height, int(math.Round(e.scrollFloat)), cursorLine, cursorCol, e.lintErrs)
}

func renderCustomView(e *Editor, content string, width, height, scrollY, cursorLine, cursorCol int, lintErrs []int) string {
	lines := strings.Split(content, "\n")
	var visible []string

	start := scrollY
	end := start + height
	if end > len(lines) {
		end = len(lines)
	}
	if start > len(lines) {
		start = len(lines)
	}

	for i := start; i < end; i++ {
		rawLine := lines[i]
		l := e.hlCache.Get(i, rawLine)

		// If cursor is on this line, we inject an inverted style character
		if i == cursorLine {
			l = injectCursorANSI(l, cursorCol)
		}

		isErr := false
		for _, errLine := range lintErrs {
			if errLine == i {
				isErr = true
				break
			}
		}

		var num string
		if isErr {
			num = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(fmt.Sprintf("%3d! ", i+1))
		} else {
			num = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(fmt.Sprintf("%4d ", i+1))
		}
		visible = append(visible, num+l)
	}
	return strings.Join(visible, "\n")
}

// injectCursorANSI intelligently inserts an ANSI invert code (\x1b[7m) at the precise
// visual character index, bypassing ANSI color codes injected by Chroma.
func injectCursorANSI(str string, cursorIdx int) string {
	var inEscape bool
	var visualIdx int
	var result strings.Builder

	for i := 0; i < len(str); {
		if str[i] == '\x1b' {
			inEscape = true
			result.WriteByte(str[i])
			i++
			continue
		}

		if inEscape {
			result.WriteByte(str[i])
			if (str[i] >= 'a' && str[i] <= 'z') || (str[i] >= 'A' && str[i] <= 'Z') {
				inEscape = false
			}
			i++
			continue
		}

		// Decode rune just in case
		r, size := utf8.DecodeRuneInString(str[i:])
		if visualIdx == cursorIdx {
			// Apply inverted style!
			result.WriteString("\x1b[7m")
			result.WriteString(str[i : i+size])
			result.WriteString("\x1b[27m")
		} else {
			result.WriteString(str[i : i+size])
		}
		visualIdx++
		i += size
		_ = r
	}

	// If cursor is at the exact end of the string
	if visualIdx == cursorIdx {
		result.WriteString("\x1b[7m \x1b[27m")
	}

	return result.String()
}
