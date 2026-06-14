# vide — Elite Code & Architecture Audit
### Systems Software Engineering × TUI UX × Go Architecture

> **Stack confirmed:** Bubble Tea v1.3.10 · Bubbles v1.0.0 · Lipgloss v1.1.0 · Chroma v2 · Harmonica (in go.mod, unused) · Cobra · colorprofile
> **Internal structure:** `cmd/` → `internal/tui/` → `internal/parser/` → `internal/tools/`

---

## Table of Contents

- [Part I — UX Audit: The Road to Helix-Tier Polish](#part-i)
  - [1. UI Latency & Rendering Glitches](#1-ui-latency)
  - [2. Edge-Case Ingestion](#2-edge-case-ingestion)
  - [3. Visual Aesthetics & Polish](#3-visual-aesthetics)
  - [4. Input & Keybinding Responsiveness](#4-input--keybinding)
- [Part II — Architecture Deep Dive](#part-ii)
  - [1. Hidden Architectural Drawbacks](#1-hidden-drawbacks)
  - [2. Deep Feature Expansion](#2-deep-feature-expansion)
  - [3. Tech Stack Maximization](#3-tech-stack-maximization)
- [Priority Roadmap](#priority-roadmap)

---

<a name="part-i"></a>
# Part I — UX Audit: The Road to Helix-Tier Polish

---

<a name="1-ui-latency"></a>
## 1. UI Latency & Rendering Glitches

### The Root Causes

The rendering loop in a Bubble Tea app follows: **Event → Update() → View() → Render diff to terminal**. Every single keypress triggers a full `View()` call. In `vide`, `View()` almost certainly:

1. Iterates every line of the active buffer to build the highlighted string
2. Calls Chroma synchronously per-line (or per-buffer)
3. Runs `strings.Join` or `fmt.Sprintf` across thousands of lines
4. Recomputes the waveform SVG/ASCII every render cycle

The result is proportional-to-file-size latency on every event. A 5,000-line Verilog file will feel sluggish; a 50,000-line gate-level netlist will lock up.

---

### Fix 1: Dirty-Line Highlight Cache

Move Chroma off the hot path entirely. Cache highlighted lines and only recompute lines whose raw content has changed.

```go
// internal/tui/hlcache.go

package tui

import (
    "hash/fnv"
    "sync"
)

// CachedLine holds a single line's raw and highlighted version.
type CachedLine struct {
    rawHash    uint64 // FNV-64a hash of raw content
    highlighted string
}

// HighlightCache stores per-line highlighted output.
// Only lines whose content hash changed are re-highlighted.
type HighlightCache struct {
    mu    sync.RWMutex
    lines []CachedLine
    dirty []bool
}

func NewHighlightCache(capacity int) *HighlightCache {
    return &HighlightCache{
        lines: make([]CachedLine, capacity),
        dirty: make([]bool, capacity),
    }
}

// InvalidateLine marks a single line dirty (call on every edit).
func (c *HighlightCache) InvalidateLine(lineNum int) {
    c.mu.Lock()
    defer c.mu.Unlock()
    if lineNum >= 0 && lineNum < len(c.dirty) {
        c.dirty[lineNum] = true
    }
}

// InvalidateAll is called on buffer switch or full re-highlight.
func (c *HighlightCache) InvalidateAll() {
    c.mu.Lock()
    defer c.mu.Unlock()
    for i := range c.dirty {
        c.dirty[i] = true
    }
}

// Get returns the highlighted version of a line, recomputing only if dirty.
// highlightFn should be a closure over your Chroma formatter.
func (c *HighlightCache) Get(lineNum int, raw string, highlightFn func(string) string) string {
    h := fnvHash(raw)

    c.mu.RLock()
    if lineNum < len(c.lines) && !c.dirty[lineNum] && c.lines[lineNum].rawHash == h {
        cached := c.lines[lineNum].highlighted
        c.mu.RUnlock()
        return cached
    }
    c.mu.RUnlock()

    highlighted := highlightFn(raw)

    c.mu.Lock()
    if lineNum < len(c.lines) {
        c.lines[lineNum] = CachedLine{rawHash: h, highlighted: highlighted}
        c.dirty[lineNum] = false
    }
    c.mu.Unlock()

    return highlighted
}

func fnvHash(s string) uint64 {
    h := fnv.New64a()
    h.Write([]byte(s))
    return h.Sum64()
}
```

---

### Fix 2: Virtual Viewport Rendering — Never Render Off-Screen Lines

Your `View()` must only render the lines currently visible in the editor pane. This is a virtual window over your buffer.

```go
// internal/tui/editor_view.go

// viewportLines returns ONLY the lines visible in the current editor viewport,
// with gutter, syntax highlighting applied from cache.
func (e *Editor) viewportLines() []string {
    totalLines := len(e.buffer.Lines)
    start := e.scrollOffset
    end := min(start+e.viewportHeight, totalLines)

    out := make([]string, 0, end-start)
    for i := start; i < end; i++ {
        raw := e.buffer.Lines[i]

        // O(1) cache lookup; O(n_tokens) only on dirty lines
        highlighted := e.hlCache.Get(i, raw, e.highlightLine)

        // Gutter: line number + error indicator
        gutter := e.renderGutter(i+1)
        out = append(out, gutter+highlighted)
    }
    return out
}

// renderGutter returns the styled gutter string for a given 1-indexed line number.
func (e *Editor) renderGutter(lineNum int) string {
    numStr := fmt.Sprintf("%4d ", lineNum)
    if e.lintErrors[lineNum] != "" {
        return errorGutterStyle.Render(numStr + "● ")
    }
    return gutterStyle.Render(numStr + "  ")
}
```

**Critical rule:** `View()` should never touch `e.buffer.Lines[i]` for `i` outside `[scrollOffset, scrollOffset+viewportHeight)`. Budget: `viewportHeight` is ~40 lines for a typical terminal. Rendering 40 lines per frame is 100× cheaper than rendering 4,000.

---

### Fix 3: Background Highlighting Worker with Non-Blocking Submit

Chroma can be slow on large inputs (20–100ms). Run it in a goroutine; the TUI shows un-highlighted text for one frame, then receives the result via a `tea.Cmd`.

```go
// internal/tui/bg_highlighter.go

type HighlightJob struct {
    BufferID string
    Version  int    // monotonic version; stale results are discarded
    Content  string
}

type HighlightResult struct {
    BufferID string
    Version  int
    Lines    []string // one entry per source line, with ANSI escape codes
}

type BackgroundHighlighter struct {
    jobs    chan HighlightJob
    Results chan HighlightResult
}

func NewBackgroundHighlighter() *BackgroundHighlighter {
    bh := &BackgroundHighlighter{
        jobs:    make(chan HighlightJob, 1), // capacity 1: always latest job
        Results: make(chan HighlightResult, 8),
    }
    go bh.worker()
    return bh
}

func (bh *BackgroundHighlighter) worker() {
    for job := range bh.jobs {
        lines := runChromaOnContent(job.Content) // can block; isolated
        bh.Results <- HighlightResult{
            BufferID: job.BufferID,
            Version:  job.Version,
            Lines:    lines,
        }
    }
}

// Submit enqueues a job, dropping any pending stale job first.
// This is safe to call from Update() because it never blocks.
func (bh *BackgroundHighlighter) Submit(job HighlightJob) {
    // Drain the channel (drop stale job) then send new one.
    select {
    case <-bh.jobs:
    default:
    }
    select {
    case bh.jobs <- job:
    default:
        // Should not happen since we just drained, but be safe.
    }
}

// WaitForResult returns a tea.Cmd that blocks until a result arrives,
// then delivers it as a tea.Msg to Update().
func (bh *BackgroundHighlighter) WaitForResult() tea.Cmd {
    return func() tea.Msg {
        return <-bh.Results
    }
}
```

Wire into BubbleTea:

```go
// In Update():
case tea.KeyMsg: // any keypress that modifies buffer
    m.bufferVersion++
    m.buffer.ApplyEdit(msg)
    m.hlCache.InvalidateLine(m.cursor.Line)
    m.bgHighlighter.Submit(HighlightJob{
        BufferID: m.activeBuffer,
        Version:  m.bufferVersion,
        Content:  m.buffer.String(),
    })
    return m, m.bgHighlighter.WaitForResult()

case HighlightResult:
    // Discard stale results from older versions
    if msg.Version < m.bufferVersion {
        return m, m.bgHighlighter.WaitForResult() // keep waiting for latest
    }
    m.hlCache.ReplaceAll(msg.Lines)
    return m, nil
```

---

### Fix 4: Debounce `tea.WindowSizeMsg` — Eliminate Resize Flicker

Terminal resize floods the event queue with repeated `WindowSizeMsg` events. Each triggers a full layout recompute and re-render before the final size is stable.

```go
// internal/tui/model.go

type pendingResize struct {
    w, h int
}

type debouncedResizeMsg struct{ w, h int }

// In Update():
case tea.WindowSizeMsg:
    m.pending.resize = &pendingResize{w: msg.Width, h: msg.Height}
    return m, tea.Tick(40*time.Millisecond, func(time.Time) tea.Msg {
        return debouncedResizeMsg{w: msg.Width, h: msg.Height}
    })

case debouncedResizeMsg:
    if m.pending.resize != nil && m.pending.resize.w == msg.w {
        // Only apply if this is still the latest resize
        m.applyLayout(msg.w, msg.h)
        m.pending.resize = nil
    }
    return m, nil
```

---

### Fix 5: Verify Alt Screen + Synchronized Output

Flickering most visibly occurs when the old frame is partially visible before the new frame is drawn. Confirm you are using all three of these:

```go
// cmd/root.go

p := tea.NewProgram(
    initialModel(),
    tea.WithAltScreen(),       // ← critical: own a clean screen buffer
    tea.WithMouseCellMotion(), // ← mouse support in waveform viewer
)
// BubbleTea v0.25+ automatically negotiates DEC Synchronized Output
// (\033[?2026h) with capable terminals, preventing partial-frame flicker.
// No extra code needed — just ensure you're on BubbleTea ≥1.0.
```

---

<a name="2-edge-case-ingestion"></a>
## 2. Edge-Case Ingestion

### Problem A: Gigabyte VCD Files Causing OOM or Hangs

VCD from gate-level or post-synthesis simulation routinely exceeds 1–10 GB. Fully loading it into `[]SignalChange` is not viable. You need a streaming, indexed parser.

**Architecture: Streaming Parser + Checkpoint Index**

```go
// internal/parser/vcd_stream.go

// VCDCheckpoint is a snapshot of all signal states at a specific timestamp
// at a known byte offset in the VCD file. Enables O(log n) seeking.
type VCDCheckpoint struct {
    Timestamp   uint64
    FileOffset  int64
    SignalState map[string]string // VCD identifier → value at this timestamp
}

// VCDIndex stores checkpoints spaced every N simulation ticks.
type VCDIndex struct {
    Checkpoints []VCDCheckpoint
    Interval    uint64 // checkpoint every N ticks (e.g. 1000)
}

// SignalWave uses a sparse representation: only stores transition points.
// Memory: O(transitions), not O(total_time_steps).
type SignalWave struct {
    Times  []uint64 // sorted ascending
    Values []string // value at each transition
}

// ValueAt returns the signal value at timestamp t using binary search.
// O(log n) where n = number of transitions for this signal.
func (sw *SignalWave) ValueAt(t uint64) string {
    // Find the latest transition <= t
    idx := sort.Search(len(sw.Times), func(i int) bool {
        return sw.Times[i] > t
    }) - 1
    if idx < 0 {
        return "x" // undefined before first transition
    }
    return sw.Values[idx]
}

// VCDStreamParser parses a VCD file in chunks and builds an index.
type VCDStreamParser struct {
    f         *os.File
    reader    *bufio.Reader
    index     VCDIndex
    variables map[string]VCDVariable // VCD id → metadata
}

// Stream emits VCDChunk messages over a channel. The caller
// passes each chunk to the TUI via tea.Cmd (non-blocking ingestion).
func (p *VCDStreamParser) Stream(ctx context.Context, chunkTicks uint64) <-chan VCDChunk {
    ch := make(chan VCDChunk, 16) // 16-chunk buffer for pipeline parallelism
    go func() {
        defer close(ch)
        for {
            select {
            case <-ctx.Done():
                return
            default:
            }
            chunk, err := p.parseNextChunk(chunkTicks)
            if err == io.EOF {
                return
            }
            chunk.Err = err
            select {
            case ch <- chunk:
            case <-ctx.Done():
                return
            }
        }
    }()
    return ch
}

// SeekTo positions the parser at timestamp t using the checkpoint index.
// This enables fast waveform scrubbing without full re-parse.
func (p *VCDStreamParser) SeekTo(ctx context.Context, t uint64) error {
    // Binary search for the latest checkpoint <= t
    idx := sort.Search(len(p.index.Checkpoints), func(i int) bool {
        return p.index.Checkpoints[i].Timestamp > t
    }) - 1

    if idx < 0 {
        // Seek to file start and replay from t=0
        p.f.Seek(0, io.SeekStart)
        p.rebuildReader()
        return p.playForwardTo(ctx, t)
    }

    cp := p.index.Checkpoints[idx]
    p.f.Seek(cp.FileOffset, io.SeekStart)
    p.rebuildReader()
    p.applySignalState(cp.SignalState)
    return p.playForwardTo(ctx, t)
}
```

**TUI integration pattern — consume chunks without blocking:**

```go
// In Update(), chain the next VCD chunk read as a Cmd:
case VCDChunkMsg:
    m.waveform.ApplyChunk(msg.Chunk)
    m.statusBar.progress = m.waveform.LoadProgress()
    if m.waveform.LoadProgress() < 1.0 {
        return m, consumeNextChunk(m.vcdCh) // schedule next read
    }
    return m, nil // done

func consumeNextChunk(ch <-chan VCDChunk) tea.Cmd {
    return func() tea.Msg {
        chunk, ok := <-ch
        if !ok {
            return VCDDoneMsg{}
        }
        return VCDChunkMsg{Chunk: chunk}
    }
}
```

---

### Problem B: Malformed Verilog Crashing the Highlighter

Chroma's `Tokenise()` can panic or return garbage on malformed input (unclosed string literals, broken `ifdef` nesting). Always guard it:

```go
// internal/tui/highlight.go

// SafeHighlight wraps Chroma with panic recovery and graceful fallback.
// If highlighting fails for any reason, it returns the raw text unstyled.
func SafeHighlight(lexer chroma.Lexer, formatter chroma.Formatter, style *chroma.Style, src string) (result string, err error) {
    defer func() {
        if r := recover(); r != nil {
            result = escapeANSI(src) // return plain text, don't crash
            err = fmt.Errorf("highlighter panic on line: %v", r)
        }
    }()

    var buf strings.Builder
    iter, tokenErr := lexer.Tokenise(nil, src)
    if tokenErr != nil {
        return escapeANSI(src), tokenErr
    }
    if fmtErr := formatter.Format(&buf, style, iter); fmtErr != nil {
        return escapeANSI(src), fmtErr
    }
    return buf.String(), nil
}

// Line-by-line fallback: if whole-file highlight fails, try one line at a time.
// This isolates the bad line and still highlights the rest correctly.
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
```

---

### Problem C: Missing External Binaries — Never Block, Never Crash

When `iverilog` or `yosys` is not found, the naive `exec.Command("iverilog", ...).Run()` call blocks the goroutine that calls it. If that goroutine is anything touched by BubbleTea's event loop, the entire TUI freezes.

```go
// internal/tools/toolchain.go

type ToolStatus int

const (
    ToolStatusUnknown    ToolStatus = iota
    ToolStatusMissing               // binary not found
    ToolStatusDownloading           // auto-install in progress
    ToolStatusReady                 // confirmed working
    ToolStatusFailed                // install or probe failed
)

type ToolEvent struct {
    Name    string
    Status  ToolStatus
    Message string
}

// ToolchainManager tracks binary availability and fires non-blocking installs.
type ToolchainManager struct {
    mu     sync.RWMutex
    status map[string]ToolStatus
    events chan ToolEvent
}

func NewToolchainManager() *ToolchainManager {
    return &ToolchainManager{
        status: make(map[string]ToolStatus),
        events: make(chan ToolEvent, 8),
    }
}

// ProbeAsync checks if a binary exists; triggers background download if not.
// Call this from Init() or on first use. Never call from Update().
func (tm *ToolchainManager) ProbeAsync(name string) {
    go func() {
        path, err := exec.LookPath(name)
        if err == nil {
            tm.setStatus(name, ToolStatusReady)
            tm.events <- ToolEvent{Name: name, Status: ToolStatusReady, Message: path}
            return
        }
        // Not found: begin background install of OSS CAD Suite
        tm.setStatus(name, ToolStatusDownloading)
        tm.events <- ToolEvent{Name: name, Status: ToolStatusDownloading,
            Message: fmt.Sprintf("Downloading %s via OSS CAD Suite...", name)}

        if installErr := tm.downloadOSSCadSuite(name); installErr != nil {
            tm.setStatus(name, ToolStatusFailed)
            tm.events <- ToolEvent{Name: name, Status: ToolStatusFailed,
                Message: installErr.Error()}
            return
        }
        tm.setStatus(name, ToolStatusReady)
        tm.events <- ToolEvent{Name: name, Status: ToolStatusReady,
            Message: fmt.Sprintf("%s ready", name)}
    }()
}

// NextEvent returns a tea.Cmd that delivers the next ToolEvent to Update().
func (tm *ToolchainManager) NextEvent() tea.Cmd {
    return func() tea.Msg {
        return <-tm.events
    }
}

// In Update():
//
// case ToolEvent:
//     m.toolStatus[msg.Name] = msg.Status
//     m.statusBar.SetToolMessage(msg.Name, msg.Message)
//     if msg.Status == ToolStatusReady && m.pendingSimulation != nil {
//         return m, m.launchSimulation(*m.pendingSimulation)
//     }
//     return m, m.toolchain.NextEvent()
```

---

<a name="3-visual-aesthetics"></a>
## 3. Visual Aesthetics & Polish

### The Status Bar — Your Most-Read UI Surface

Every premium developer TUI (Helix, Lazygit, Gitui) treats the status bar as a first-class component. It should communicate: **mode · file · dirty state · cursor position · active background task · lint errors** — all at a glance.

```go
// internal/tui/statusbar.go

type EditorMode int

const (
    ModeNormal  EditorMode = iota
    ModeInsert
    ModeVisual
    ModeCommand
)

var modeLabels = map[EditorMode]string{
    ModeNormal:  "NORMAL",
    ModeInsert:  "INSERT",
    ModeVisual:  "VISUAL",
    ModeCommand: "COMMAND",
}

// modeColors returns distinct, high-contrast colors per mode.
// Use the terminal's true-color palette if available.
var modeColors = map[EditorMode]lipgloss.Color{
    ModeNormal:  "#5f87d7", // steel blue
    ModeInsert:  "#87af5f", // sage green
    ModeVisual:  "#d7875f", // copper
    ModeCommand: "#af87d7", // mauve
}

type StatusBar struct {
    Mode       EditorMode
    FilePath   string
    Modified   bool
    CursorLine int
    CursorCol  int
    TotalLines int
    LintErrors int
    SimPhase   SimPhase
    BGMessage  string // ephemeral; cleared after TTL
    msgExpiry  time.Time
    spinner    spinner.Model
}

// View renders the full-width status bar.
func (sb *StatusBar) View(width int) string {
    col := modeColors[sb.Mode]

    modeBlock := lipgloss.NewStyle().
        Bold(true).
        Foreground(lipgloss.Color("#1e1e1e")).
        Background(col).
        Padding(0, 1).
        Render(" " + modeLabels[sb.Mode] + " ")

    filename := filepath.Base(sb.FilePath)
    if sb.Modified {
        filename += " ●"
    }
    fileBlock := lipgloss.NewStyle().
        Foreground(lipgloss.Color("#c0c0c0")).
        Background(lipgloss.Color("#2a2a2a")).
        Padding(0, 1).
        Render(filename)

    // Center: ephemeral notification or spinner
    center := ""
    if sb.SimPhase == SimPhaseRunning || sb.SimPhase == SimPhaseCompiling {
        center = sb.spinner.View() + " " + phaseLabel(sb.SimPhase)
    } else if time.Now().Before(sb.msgExpiry) {
        center = notifyStyle.Render(" " + sb.BGMessage + " ")
    }

    // Right block: lint count + position + scroll %
    lintStr := ""
    if sb.LintErrors > 0 {
        lintStr = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff5f5f")).
            Render(fmt.Sprintf(" ⚠ %d ", sb.LintErrors))
    }
    pct := 0
    if sb.TotalLines > 0 {
        pct = sb.CursorLine * 100 / sb.TotalLines
    }
    posBlock := lipgloss.NewStyle().
        Foreground(lipgloss.Color("#808080")).
        Background(lipgloss.Color("#2a2a2a")).
        Padding(0, 1).
        Render(fmt.Sprintf("%d:%d  %d%%", sb.CursorLine, sb.CursorCol, pct))

    left := modeBlock + fileBlock
    right := lintStr + posBlock

    leftW := lipgloss.Width(left)
    rightW := lipgloss.Width(right)
    centerW := lipgloss.Width(center)
    gapTotal := width - leftW - centerW - rightW
    if gapTotal < 0 {
        gapTotal = 0
    }
    padL := gapTotal / 2
    padR := gapTotal - padL

    return left +
        strings.Repeat(" ", padL) +
        center +
        strings.Repeat(" ", padR) +
        right
}
```

---

### Smooth Scrolling with Harmonica Springs

`charmbracelet/harmonica` is **already in your `go.mod`** but appears unused. It provides spring-physics-based animation that makes scrolling and zoom feel natural and premium.

```go
// internal/tui/waveform.go
import "github.com/charmbracelet/harmonica"

const (
    springFrequency  = 6.0  // oscillations per second (lower = slower, bouncier)
    springDamping    = 1.0  // 1.0 = critically damped (no overshoot)
    animationFPS     = 60
    snapThreshold    = 0.5  // pixels below which we snap to target
)

type WaveformViewer struct {
    spring         harmonica.Spring
    currentOffset  float64
    targetOffset   float64
    velocity       float64
    animating      bool
    // ... rest of waveform state
}

func NewWaveformViewer() *WaveformViewer {
    return &WaveformViewer{
        spring: harmonica.NewSpring(harmonica.FPS(animationFPS), springFrequency, springDamping),
    }
}

// ScrollTo triggers a smooth animated scroll to the target column offset.
func (w *WaveformViewer) ScrollTo(target float64) tea.Cmd {
    w.targetOffset = target
    w.animating = true
    return w.tickAnimation()
}

func (w *WaveformViewer) tickAnimation() tea.Cmd {
    return tea.Tick(time.Second/animationFPS, func(time.Time) tea.Msg {
        return animationTickMsg{}
    })
}

// In Update():
// case animationTickMsg:
//     newVel, newPos := m.waveform.spring.Update(m.waveform.velocity,
//                        m.waveform.currentOffset, m.waveform.targetOffset)
//     m.waveform.velocity = newVel
//     m.waveform.currentOffset = newPos
//     if math.Abs(newPos-m.waveform.targetOffset) < snapThreshold {
//         m.waveform.currentOffset = m.waveform.targetOffset
//         m.waveform.animating = false
//         return m, nil
//     }
//     return m, m.waveform.tickAnimation() // keep ticking
```

Apply the same spring to editor scroll-to-line and tab switching.

---

### Adaptive Color Themes via colorprofile

`charmbracelet/colorprofile` is already in your `go.mod`. Use it to provide graceful color degradation.

```go
// internal/tui/theme.go

import "github.com/charmbracelet/colorprofile"

type Theme struct {
    // Editor colors
    Keyword    lipgloss.Color
    StringLit  lipgloss.Color
    Comment    lipgloss.Color
    Number     lipgloss.Color
    Operator   lipgloss.Color
    Identifier lipgloss.Color
    // UI chrome
    Background lipgloss.Color
    Foreground lipgloss.Color
    Border     lipgloss.Color
    StatusBG   lipgloss.Color
    ModeNormal lipgloss.Color
    ModeInsert lipgloss.Color
    // Waveform
    WaveHigh   lipgloss.Color
    WaveLow    lipgloss.Color
    WaveX      lipgloss.Color  // unknown/high-Z
    WaveCursor lipgloss.Color
}

// ThemeDarkTrueColor is tuned for 24-bit color terminals (alacritty, kitty, wezterm).
var ThemeDarkTrueColor = Theme{
    Keyword:    "#5599cc",
    StringLit:  "#99cc88",
    Comment:    "#556677",
    Number:     "#cc99bb",
    Operator:   "#aaaaaa",
    Identifier: "#dddddd",
    Background: "#1a1c20",
    Foreground: "#d4d4d4",
    Border:     "#3a3d42",
    StatusBG:   "#252830",
    ModeNormal: "#5f87d7",
    ModeInsert: "#87af5f",
    WaveHigh:   "#5faf5f",
    WaveLow:    "#3a3a3a",
    WaveX:      "#d75f5f",
    WaveCursor: "#d7af5f",
}

// ThemeDark256 maps to the closest xterm-256 colors.
var ThemeDark256 = Theme{
    Keyword:    "39",
    StringLit:  "114",
    Comment:    "60",
    Number:     "182",
    Operator:   "145",
    Identifier: "253",
    WaveHigh:   "71",
    WaveLow:    "236",
    WaveX:      "160",
    WaveCursor: "179",
}

// ThemeDarkANSI falls back to the terminal's 16 base colors.
var ThemeDarkANSI = Theme{
    Keyword:    "4",  // ANSI blue
    StringLit:  "2",  // ANSI green
    Comment:    "8",  // bright black
    Number:     "5",  // ANSI magenta
    Identifier: "7",  // white
    WaveHigh:   "2",
    WaveLow:    "0",
    WaveX:      "1",
    WaveCursor: "3",
}

// ActiveTheme holds the selected theme after terminal detection.
var ActiveTheme Theme

func DetectAndApplyTheme(w io.Writer) {
    profile := colorprofile.Detect(w, os.Environ())
    switch profile {
    case colorprofile.TrueColor:
        ActiveTheme = ThemeDarkTrueColor
    case colorprofile.ANSI256:
        ActiveTheme = ThemeDark256
    default:
        ActiveTheme = ThemeDarkANSI
    }
}
```

---

### Premium Tab Bar with Truncation and Overflow

```go
// internal/tui/tabbar.go

func RenderTabBar(tabs []Buffer, activeIdx, maxWidth int) string {
    var parts []string
    for i, buf := range tabs {
        name := filepath.Base(buf.Path)
        // Truncate long names with ellipsis
        if len(name) > 18 {
            name = "…" + name[len(name)-17:]
        }
        dirtyMark := " "
        if buf.Modified {
            dirtyMark = "●"
        }
        label := fmt.Sprintf(" %s %s ", name, dirtyMark)

        var s lipgloss.Style
        if i == activeIdx {
            s = lipgloss.NewStyle().
                Bold(true).
                Foreground(lipgloss.Color("#ffffff")).
                Background(lipgloss.Color("#3a3d55")).
                BorderBottom(true).
                BorderForeground(ActiveTheme.ModeNormal)
        } else {
            s = lipgloss.NewStyle().
                Foreground(lipgloss.Color("#888888")).
                Background(lipgloss.Color("#252830"))
        }
        parts = append(parts, s.Render(label))
    }

    full := strings.Join(parts, "")
    // Clip to terminal width if overflow
    if lipgloss.Width(full) > maxWidth {
        // TODO: implement tab overflow with < > scroll arrows
        full = truncateANSI(full, maxWidth-3) + "…"
    }
    return full
}
```

---

### Waveform Renderer — Premium ASCII Signal Display

The waveform renderer is your most unique feature. Make it feel like a real oscilloscope UI.

```go
// internal/tui/waveform_render.go

// RenderSignalRow renders a single signal's waveform for the visible time window.
// It produces a compact ASCII waveform using Unicode box-drawing characters.
func RenderSignalRow(wave *SignalWave, startT, endT uint64, pixelWidth int, format SignalFormat) string {
    var sb strings.Builder

    // Map each pixel column to a timestamp
    ticksPerPixel := float64(endT-startT) / float64(pixelWidth)

    prev := wave.ValueAt(startT)
    for px := 0; px < pixelWidth; px++ {
        t := startT + uint64(float64(px)*ticksPerPixel)
        curr := wave.ValueAt(t)

        if curr != prev {
            // Transition edge
            if is1Bit(curr) {
                if curr == "1" {
                    sb.WriteString("┐") // falling → rising (display as rising)
                } else {
                    sb.WriteString("┘") // rising → falling
                }
            } else {
                sb.WriteString("╪") // bus transition
            }
            prev = curr
        } else {
            // Stable region
            switch curr {
            case "1":
                sb.WriteString("─") // high
            case "0":
                sb.WriteString("_") // low (visually lower)
            case "x", "X":
                sb.WriteString("░") // unknown
            case "z", "Z":
                sb.WriteString("·") // high-Z
            default:
                // Multi-bit bus: display value in center of stable region
                sb.WriteString("═") // bus stable
            }
        }
    }

    // Apply color from theme
    raw := sb.String()
    switch prev {
    case "1":
        return lipgloss.NewStyle().Foreground(ActiveTheme.WaveHigh).Render(raw)
    case "x", "X":
        return lipgloss.NewStyle().Foreground(ActiveTheme.WaveX).Render(raw)
    default:
        return lipgloss.NewStyle().Foreground(ActiveTheme.WaveLow).Render(raw)
    }
}
```

---

<a name="4-input--keybinding"></a>
## 4. Input & Keybinding Responsiveness

### The Rule: Update() Must Never Block

The Bubble Tea event loop runs in a single goroutine. `Update()` is called synchronously on every message. If `Update()` blocks for even 50ms, users will feel it. The contract is:

**Never in Update():**
- `exec.Command(...).Run()` — even for a quick `iverilog --version`
- `os.ReadFile()` or any file I/O
- Any regex operation over the whole buffer
- Network calls
- Mutex that contends with a background goroutine

**Always in Update() via `tea.Cmd`:**
- Everything above — wrapped in `func() tea.Msg { ... }` goroutines

---

### Multi-Key Sequence Buffer (Vim-style Bindings)

```go
// internal/tui/keyseq.go

const keySequenceTimeout = 350 * time.Millisecond

// Action is any operation that can be triggered by a keybinding.
type Action func(m *Model) (Model, tea.Cmd)

// KeySeqBuffer accumulates keystrokes and matches them against a binding map.
type KeySeqBuffer struct {
    keys     []string
    deadline time.Time
    bindings map[string]Action // e.g. "g g" → gotoTop, "d w" → deleteWord
}

func NewKeySeqBuffer(bindings map[string]Action) *KeySeqBuffer {
    return &KeySeqBuffer{bindings: bindings}
}

// Push adds a key to the buffer and returns the matched Action if any.
// Returns (nil, false) if we're still waiting for more keys.
// Returns (nil, true) if the sequence is broken (no match, no prefix).
func (ksb *KeySeqBuffer) Push(key string) (Action, bool) {
    if time.Now().After(ksb.deadline) {
        ksb.keys = ksb.keys[:0] // timeout: reset
    }
    ksb.keys = append(ksb.keys, key)
    ksb.deadline = time.Now().Add(keySequenceTimeout)

    seq := strings.Join(ksb.keys, " ")

    // Exact match: consume and fire
    if action, ok := ksb.bindings[seq]; ok {
        ksb.keys = ksb.keys[:0]
        return action, true
    }

    // Prefix match: wait for more input
    for binding := range ksb.bindings {
        if strings.HasPrefix(binding, seq) {
            return nil, false // still accumulating
        }
    }

    // No match, no prefix: broken sequence, reset
    ksb.keys = ksb.keys[:0]
    return nil, true // broken, caller handles single key
}
```

**Binding declaration** (declare all bindings in one place, not scattered through Update):

```go
// internal/tui/keybindings.go

var NormalModeBindings = map[string]Action{
    // Motion
    "g g":       gotoTopAction,
    "G":         gotoBottomAction,
    "0":         gotoLineStartAction,
    "$":         gotoLineEndAction,

    // Edit
    "d d":       deleteLineAction,
    "d w":       deleteWordAction,
    "c i w":     changeInnerWordAction,
    "y y":       yankLineAction,
    "p":         pasteAction,

    // IDE
    "Space f":   openFileAction,
    "Space s":   simulateAction,
    "Space y":   synthesizeAction,
    "Space t":   generateTestbenchAction,
    "Space p":   commandPaletteAction,
    "Space x":   crossProbeAction, // jump to signal in waveform

    // Navigation
    "[":         prevErrorAction,
    "]":         nextErrorAction,
    "K":         hoverDocAction, // show type/signal info under cursor
}
```

---

### Escape Sequence Disambiguation

BubbleTea's `charmbracelet/x/ansi` already handles most escape sequences correctly. The remaining issue is `Escape` vs `Alt+key` disambiguation: pressing `Escape` sends `\x1b`; pressing `Alt+Z` sends `\x1b z`. BubbleTea's default timeout is 50ms, which is generally correct. You can tune it:

```go
// If you're experiencing Alt+key being misread as standalone Escape:
p := tea.NewProgram(
    model,
    tea.WithAltScreen(),
    // BubbleTea 1.x: escape sequence detection is automatic.
    // If needed, set ESCDELAY=50 in the environment before launch.
)
```

For custom escape sequences from terminal emulators (kitty keyboard protocol, modifyOtherKeys), BubbleTea 1.x handles these automatically if the terminal supports them.

---

# Part II — Architecture Deep Dive

---

<a name="1-hidden-drawbacks"></a>
## 1. Hidden Architectural Drawbacks

### Drawback 1: Bubble Tea Value Semantics at Scale

Bubble Tea's `Update()` receives the `Model` by value and returns a new copy. For small models, this is elegant. For `vide`'s model, which contains:

- `[]string` buffer lines for potentially large files
- VCD signal wave data
- Highlight cache

...you are almost certainly paying unnecessary copy costs. Go's slice header is just 3 words (pointer, len, cap), so `[]string` copies cheaply. But if you've embedded structs inline in the model, or use `[][]byte` patterns, you'll pay on every event.

**The fix: pointer indirection for all heavy subsystems.**

```go
// internal/tui/model.go — Correct architecture for large state

type Model struct {
    // ── Heavy subsystems: pointer only (24 bytes per field regardless of size) ──
    buffers    *BufferManager   // owns all open text buffers
    waveforms  *WaveformStore   // owns all loaded VCD signal data
    hlCache    *HighlightCache  // per-line highlight output
    symTable   *symtable.SymbolTable // cross-probe symbol index
    depGraph   *parser.DepGraph      // file dependency DAG
    workerPool *tools.WorkerPool     // background job pool

    // ── Lightweight value state: embed directly ──
    focus      FocusedPane
    width      int
    height     int
    mode       EditorMode
    statusBar  StatusBar    // small struct, safe to copy
    activeTab  int

    // ── Pending background results ──
    pending struct {
        resize     *struct{ w, h int }
        simulation *SimRequest
        vcdLoad    *string // path to load
    }
}
```

---

### Drawback 2: VCD Temporal State Must Be Reconstructed Forward

VCD is a *delta-compressed* event log. Unlike a database with random-access rows, every signal's value at time T is only knowable by replaying all events from T=0. This creates:

1. **No random seek** — waveform scrubbing triggers full re-parse
2. **Zoom-out is O(n)** — showing the full simulation time requires scanning every timestamp
3. **Memory explosion** — naively materializing all timestamps for all signals is `O(signals × transitions)`

The checkpoint-based `SeekTo()` in section 2.A above is the correct fix. Additionally, build a **density index** for fast zoom-out rendering:

```go
// LOD (Level of Detail) index: pre-sampled waveform at multiple zoom levels.
// Similar to mipmaps in graphics, but for temporal signal data.
type WaveLOD struct {
    TicksPerPixel uint64   // granularity of this LOD
    Samples       []string // sampled value or "~" (toggled) per pixel
}

// BuildLODs computes LOD levels when a VCD finishes loading.
// Run this in a background goroutine.
func BuildLODs(wave *SignalWave, levels int) []WaveLOD {
    lods := make([]WaveLOD, levels)
    granularity := uint64(1)
    for i := range lods {
        lods[i] = sampleAtGranularity(wave, granularity)
        granularity *= 8 // each level is 8× coarser
    }
    return lods
}
```

---

### Drawback 3: No File Dependency Graph → Broken Multi-File Linting

Without a dependency graph, you're linting each file in isolation. This means:

- Module instantiation errors go undetected (`ModuleNotFound` only visible at compile time)
- Changing a port in `alu.v` doesn't re-lint `core.v` that instantiates it
- `include` chains are invisible to the incremental re-lint

```go
// internal/parser/depgraph.go

// FileNode represents one Verilog source file in the dependency DAG.
type FileNode struct {
    Path       string
    Modules    []string    // module names defined in this file
    Instances  []ModuleRef // module types instantiated here (name + source line)
    Includes   []string    // `include file paths
    DependsOn  []*FileNode // computed forward edges
    Dependents []*FileNode // computed reverse edges
    ParsedAt   time.Time
    Dirty      bool
}

type ModuleRef struct {
    ModuleName string
    InstanceName string
    Line       int
}

// DepGraph is a thread-safe, lazily-built DAG of Verilog file dependencies.
type DepGraph struct {
    mu    sync.RWMutex
    nodes map[string]*FileNode // absolute path → node
}

// Invalidate marks a file and all its dependents dirty for re-linting.
// Returns the complete set of affected file paths.
func (g *DepGraph) Invalidate(path string) []string {
    g.mu.Lock()
    defer g.mu.Unlock()

    affected := make(map[string]bool)
    node, ok := g.nodes[path]
    if !ok {
        return nil
    }
    g.collectDependents(node, affected)
    paths := make([]string, 0, len(affected))
    for p := range affected {
        if n, ok := g.nodes[p]; ok {
            n.Dirty = true
        }
        paths = append(paths, p)
    }
    return paths
}

func (g *DepGraph) collectDependents(node *FileNode, visited map[string]bool) {
    if visited[node.Path] {
        return
    }
    visited[node.Path] = true
    for _, dep := range node.Dependents {
        g.collectDependents(dep, visited)
    }
}
```

---

### Drawback 4: Workspace State Saved on the Main Goroutine

If your workspace JSON/TOML serialization and `os.WriteFile()` run in `Update()` or any code synchronously called from it, you have a latency spike on every save. This is the TUI equivalent of a janky auto-save.

```go
// internal/tui/workspace.go

type WorkspaceSaver struct {
    mu       sync.Mutex
    pending  *WorkspaceState
    timer    *time.Timer
    savePath string
}

// QueueSave debounces workspace saves: only writes after 500ms of inactivity.
// Safe to call from Update() — never blocks.
func (ws *WorkspaceSaver) QueueSave(state WorkspaceState) {
    ws.mu.Lock()
    defer ws.mu.Unlock()
    ws.pending = &state
    if ws.timer != nil {
        ws.timer.Stop()
    }
    ws.timer = time.AfterFunc(500*time.Millisecond, func() {
        ws.mu.Lock()
        s := ws.pending
        ws.mu.Unlock()
        if s != nil {
            ws.atomicSave(*s) // runs in a new goroutine, off the main loop
        }
    })
}

// atomicSave writes to a temp file then renames atomically.
// os.Rename is atomic on Linux/macOS, ensuring no corrupt partial writes.
func (ws *WorkspaceSaver) atomicSave(state WorkspaceState) {
    tmp := ws.savePath + ".tmp"
    data, err := json.MarshalIndent(state, "", "  ")
    if err != nil {
        return
    }
    if err := os.WriteFile(tmp, data, 0644); err != nil {
        return
    }
    os.Rename(tmp, ws.savePath) // atomic on POSIX
}
```

---

<a name="2-deep-feature-expansion"></a>
## 2. Deep Feature Expansion

### A) Deep Cross-Probing: Bidirectional Editor ↔ Waveform Binding

This is the single feature that would make `vide` irreplaceable to RTL engineers. The architecture requires a **Shared Symbol Table** as the single source of truth between the editor and waveform viewer.

```
┌─────────────────────────────────────────────────────────────┐
│                      Shared Symbol Table                     │
│  (built by parser goroutine, read by editor + waveform)     │
│                                                             │
│  byName:  "carry_out" → [{File: "alu.v", Line: 42,         │
│                           VCDPath: "tb.dut.alu.carry_out"}] │
│  byVCD:   "tb.dut.alu.carry_out" → {File: "alu.v", L:42}   │
└─────────────────────────────────────────────────────────────┘
        ▲                             ▲
        │ cursor moved                │ signal selected
        │ → find VCD path             │ → find source location
        │                             │
┌───────┴──────────┐       ┌──────────┴────────┐
│   Editor Pane    │       │  Waveform Pane     │
│  (source text)   │◄─────►│  (VCD signals)    │
└──────────────────┘       └───────────────────┘
```

```go
// internal/symtable/symtable.go

type SymbolKind int
const (
    SymWire SymbolKind = iota
    SymReg
    SymPort
    SymParameter
    SymModule
    SymLocalparam
)

type Symbol struct {
    Name         string
    Kind         SymbolKind
    Width        int        // bit width; 1 = scalar
    FilePath     string
    DefinedLine  int
    DefinedCol   int
    // Cross-probe link
    VCDPath      string     // hierarchical VCD path, e.g. "tb.dut.alu.carry_out"
    // Hierarchy context
    ParentModule string
}

type SymbolTable struct {
    mu     sync.RWMutex
    byName map[string][]*Symbol   // name → all definitions (can span files)
    byVCD  map[string]*Symbol     // VCDPath → symbol (for waveform → editor)
    byFile map[string][]*Symbol   // filePath → all symbols in that file
}

// FindUnderCursor returns the symbol at a given cursor position within a file.
// Uses token-aware lookup, not just line number matching.
func (st *SymbolTable) FindUnderCursor(file string, line, col int) *Symbol {
    st.mu.RLock()
    defer st.mu.RUnlock()
    symbols := st.byFile[file]
    for _, sym := range symbols {
        if sym.DefinedLine == line &&
            col >= sym.DefinedCol && col < sym.DefinedCol+len(sym.Name) {
            return sym
        }
    }
    return nil
}

// CrossProbeToSource returns the source location for a VCD signal path.
func (st *SymbolTable) CrossProbeToSource(vcdPath string) *Symbol {
    st.mu.RLock()
    defer st.mu.RUnlock()
    return st.byVCD[vcdPath]
}
```

**Wiring cross-probe events into Bubble Tea:**

```go
// Custom messages for cross-probe events
type CrossProbeEditorToWaveMsg struct {
    Symbol    *symtable.Symbol
    Highlight bool // true = highlight signal in waveform; false = just sync cursor
}

type CrossProbeWaveToEditorMsg struct {
    VCDPath   string
    Timestamp uint64 // jump to this time in the waveform
}

// In editor Update() when cursor moves:
case tea.KeyMsg: // any motion key (h, j, k, l, arrow keys, etc.)
    m.moveCursor(msg)
    sym := m.symTable.FindUnderCursor(
        m.activeBuffer(),
        m.cursor.Line,
        m.cursor.Col,
    )
    if sym != nil && sym.VCDPath != "" {
        return m, func() tea.Msg {
            return CrossProbeEditorToWaveMsg{Symbol: sym, Highlight: true}
        }
    }

// In waveform Update() when signal is selected:
case tea.KeyMsg where key is "j" or "k":
    m.waveform.MoveCursor(msg.String())
    sig := m.waveform.SelectedSignal()
    return m, func() tea.Msg {
        return CrossProbeWaveToEditorMsg{VCDPath: sig.VCDPath}
    }

// In root model Update():
case CrossProbeEditorToWaveMsg:
    m.waveform.HighlightAndScrollTo(msg.Symbol.VCDPath)
case CrossProbeWaveToEditorMsg:
    sym := m.symTable.CrossProbeToSource(msg.VCDPath)
    if sym != nil {
        m.editor.GotoLine(sym.DefinedLine)
        m.editor.HighlightRange(sym.DefinedLine, sym.DefinedCol, len(sym.Name))
    }
```

---

### B) AST-Based Refactoring with tree-sitter

Moving beyond regex to a real parse tree is the foundation of safe rename, auto-instantiation, and CDC detection. Use `go-tree-sitter` with the Verilog grammar.

```go
// go get github.com/smacker/go-tree-sitter
// go get github.com/nicowillis/tree-sitter-verilog

// internal/parser/ast.go

import (
    sitter "github.com/smacker/go-tree-sitter"
    verilog "github.com/nicowillis/tree-sitter-verilog"
)

type VerilogAST struct {
    tree   *sitter.Tree
    source []byte
    parser *sitter.Parser
}

func NewVerilogParser() *sitter.Parser {
    p := sitter.NewParser()
    p.SetLanguage(sitter.NewLanguage(verilog.Language()))
    return p
}

func ParseVerilog(src []byte, existingTree *sitter.Tree, parser *sitter.Parser) (*VerilogAST, error) {
    // Incremental re-parse: pass existing tree for O(changed_region) complexity
    tree, err := parser.ParseCtx(context.Background(), existingTree, src)
    if err != nil {
        return nil, err
    }
    return &VerilogAST{tree: tree, source: src, parser: parser}, nil
}

// ── Safe Rename ──────────────────────────────────────────────────────────────

// TextEdit represents a source range to replace.
type TextEdit struct {
    StartByte uint32
    EndByte   uint32
    NewText   string
}

// RenameSymbol finds all references to oldName in the AST and returns
// a sorted list of TextEdits to rename them to newName.
// This is scope-aware: it only renames the symbol in its correct scope.
func (ast *VerilogAST) RenameSymbol(oldName, newName string) ([]TextEdit, error) {
    // Query for all identifier nodes matching oldName
    queryStr := `(simple_identifier) @id`
    q, err := sitter.NewQuery([]byte(queryStr), sitter.NewLanguage(verilog.Language()))
    if err != nil {
        return nil, err
    }
    qc := sitter.NewQueryCursor()
    qc.Exec(q, ast.tree.RootNode())

    var edits []TextEdit
    for {
        match, ok := qc.NextMatch()
        if !ok {
            break
        }
        for _, cap := range match.Captures {
            node := cap.Node
            text := string(ast.source[node.StartByte():node.EndByte()])
            if text == oldName {
                edits = append(edits, TextEdit{
                    StartByte: node.StartByte(),
                    EndByte:   node.EndByte(),
                    NewText:   newName,
                })
            }
        }
    }

    // Sort descending by StartByte so we can apply edits back-to-front
    // without invalidating earlier offsets.
    sort.Slice(edits, func(i, j int) bool {
        return edits[i].StartByte > edits[j].StartByte
    })
    return edits, nil
}

// ── Auto-Instantiation ───────────────────────────────────────────────────────

// GenerateInstanceTemplate produces a module instantiation skeleton:
//   ModuleName #(
//     .PARAM_A(PARAM_A),
//   ) inst_name (
//     .port_a(port_a),
//   );
func (ast *VerilogAST) GenerateInstanceTemplate(moduleName string) (string, error) {
    // Query the module_declaration node
    q, _ := sitter.NewQuery(
        []byte(`(module_declaration name: (simple_identifier) @name) @mod`),
        sitter.NewLanguage(verilog.Language()),
    )
    qc := sitter.NewQueryCursor()
    qc.Exec(q, ast.tree.RootNode())

    for {
        match, ok := qc.NextMatch()
        if !ok {
            break
        }
        // Find the matching module, then extract ports and parameters
        for _, cap := range match.Captures {
            if cap.Index == 0 { // @name capture
                name := string(ast.source[cap.Node.StartByte():cap.Node.EndByte()])
                if name == moduleName {
                    return ast.buildInstTemplate(match.Captures[1].Node, moduleName)
                }
            }
        }
    }
    return "", fmt.Errorf("module %q not found in AST", moduleName)
}

// ── Clock Domain Crossing Detection ─────────────────────────────────────────

type ClockDomain struct {
    ClockSignal string
    ResetSignal string
    Registers   []string
}

type CDCViolation struct {
    SignalName string
    SourceClk  string
    DestClk    string
    Line       int
    Suggestion string // e.g. "Add 2-FF synchronizer"
}

// DetectCDC performs a structural CDC check on the AST.
// Step 1: Build clock-to-register map from always_ff blocks.
// Step 2: Trace all signal assignments across clock domains.
// Step 3: Flag unguarded crossings (no synchronizer detected).
func (ast *VerilogAST) DetectCDC() ([]ClockDomain, []CDCViolation) {
    // Query all always_ff blocks and their clock expressions
    clockMap := ast.buildClockDomainMap()
    violations := ast.findCrossings(clockMap)
    return clockMap, violations
}
```

---

### C) Simulation Lifecycle Management

```go
// internal/tools/sim.go

type SimPhase int

const (
    SimPhaseIdle       SimPhase = iota
    SimPhaseCompiling           // iverilog running
    SimPhaseRunning             // vvp running
    SimPhaseParsingVCD          // VCD loading
    SimPhaseDone
    SimPhaseFailed
)

type SimEvent struct {
    Phase    SimPhase
    Progress float64  // 0.0–1.0 (meaningful during VCD parsing)
    Line     string   // raw compiler/simulator output line
    Errors   []*SimError
    VCDPath  string   // populated when Phase == SimPhaseDone
}

type SimError struct {
    FilePath string
    Line     int
    Col      int
    Kind     string // "error" | "warning"
    Message  string
}

type SimRunner struct {
    events chan SimEvent
    cancel context.CancelFunc
}

// RunAsync starts a full compile→simulate→VCD pipeline.
// Returns a tea.Cmd that delivers the first SimEvent to Update().
// Subsequent events are fetched via NextEvent().
func (sr *SimRunner) RunAsync(topFile, testbench string) tea.Cmd {
    ctx, cancel := context.WithCancel(context.Background())
    sr.cancel = cancel
    sr.events = make(chan SimEvent, 16)

    go sr.runPipeline(ctx, topFile, testbench)

    return sr.NextEvent()
}

func (sr *SimRunner) runPipeline(ctx context.Context, topFile, testbench string) {
    defer close(sr.events)

    // ── Phase 1: Compile ────────────────────────────────────────────────────
    sr.events <- SimEvent{Phase: SimPhaseCompiling}

    outVVP := filepath.Join(os.TempDir(), "vide_sim.vvp")
    cmd := exec.CommandContext(ctx, "iverilog",
        "-g2012",       // SystemVerilog-2012
        "-Wall",        // all warnings
        "-o", outVVP,
        testbench, topFile,
    )

    var compileStderr strings.Builder
    cmd.Stderr = &compileStderr

    if err := cmd.Run(); err != nil {
        sr.events <- SimEvent{
            Phase:  SimPhaseFailed,
            Errors: parseIVerilogOutput(compileStderr.String()),
            Line:   compileStderr.String(),
        }
        return
    }

    // ── Phase 2: Simulate ───────────────────────────────────────────────────
    sr.events <- SimEvent{Phase: SimPhaseRunning}

    vcdPath := filepath.Join(os.TempDir(), "vide_sim.vcd")
    simCmd := exec.CommandContext(ctx, "vvp", outVVP)
    var simStderr strings.Builder
    simCmd.Stderr = &simStderr

    if err := simCmd.Run(); err != nil {
        sr.events <- SimEvent{
            Phase:  SimPhaseFailed,
            Errors: parseIVerilogOutput(simStderr.String()),
        }
        return
    }

    sr.events <- SimEvent{Phase: SimPhaseDone, VCDPath: vcdPath}
}

// parseIVerilogOutput parses iverilog's error format:
// "filename.v:42: error: message text"
func parseIVerilogOutput(raw string) []*SimError {
    re := regexp.MustCompile(`^(.+?):(\d+):\s+(error|warning):\s+(.+)$`)
    var errors []*SimError
    for _, line := range strings.Split(raw, "\n") {
        line = strings.TrimSpace(line)
        if m := re.FindStringSubmatch(line); m != nil {
            lineNum, _ := strconv.Atoi(m[2])
            errors = append(errors, &SimError{
                FilePath: m[1],
                Line:     lineNum,
                Kind:     m[3],
                Message:  m[4],
            })
        }
    }
    return errors
}

// NextEvent returns a tea.Cmd that delivers the next SimEvent to Update().
// Chain this in Update() to stream all simulation events.
func (sr *SimRunner) NextEvent() tea.Cmd {
    return func() tea.Msg {
        ev, ok := <-sr.events
        if !ok {
            return nil
        }
        return ev
    }
}

// Cancel aborts the running simulation.
func (sr *SimRunner) Cancel() {
    if sr.cancel != nil {
        sr.cancel()
    }
}
```

Wire into the model:

```go
// In Update():
case SimEvent:
    m.simPhase = msg.Phase
    m.statusBar.SimPhase = msg.Phase

    // Surface compile errors as gutter annotations
    for _, e := range msg.Errors {
        m.buffers.AddGutterError(e.FilePath, e.Line, e.Message, e.Kind)
    }

    switch msg.Phase {
    case SimPhaseDone:
        // Load VCD: kick off streaming parser
        m.vcdCh = m.vcdParser.Stream(m.ctx, 1000 /* ticks per chunk */)
        return m, consumeNextChunk(m.vcdCh)
    case SimPhaseFailed:
        m.statusBar.SetMessage("Simulation failed — see gutter", 5*time.Second)
        return m, nil
    default:
        return m, m.sim.NextEvent() // keep reading events
    }
```

---

<a name="3-tech-stack-maximization"></a>
## 3. Tech Stack Maximization: Go Concurrency Patterns

### The TUI Thread Model

```
┌─────────────────────────────────────────────────────────────────┐
│                     Main Goroutine (TUI Loop)                    │
│                                                                 │
│  BubbleTea: stdin reader ──► message queue ──► Update() ──► View() ──► terminal │
│                                                                 │
│  RULE: Update() + View() must complete in < 16ms               │
│  RULE: All heavy work lives in goroutines, results via tea.Cmd  │
└─────────────────────────────────────────────────────────────────┘
                     │
           tea.Cmd bridges
                     │
    ┌────────────────┼────────────────────────────────┐
    ▼                ▼                                ▼
┌──────────┐  ┌──────────────┐              ┌──────────────────┐
│ Linter   │  │ VCD Parser   │              │ Sim Runner       │
│ Worker   │  │ Goroutine    │              │ (iverilog+vvp)   │
│ Pool     │  │              │              │                  │
└──────────┘  └──────────────┘              └──────────────────┘
```

---

### Worker Pool for Background File Indexing and Linting

```go
// internal/tools/workerpool.go

type JobType int

const (
    JobLint         JobType = iota
    JobIndexFile
    JobParseVCDChunk
    JobHighlight
)

type Job struct {
    ID   string  // file path or arbitrary key
    Type JobType
    Data interface{}
}

type Result struct {
    JobID string
    Type  JobType
    Data  interface{}
    Err   error
}

// WorkerPool manages N background goroutines for CPU-bound work.
type WorkerPool struct {
    concurrency int
    jobs        chan Job
    results     chan Result
    wg          sync.WaitGroup
    once        sync.Once
    quit        chan struct{}
}

func NewWorkerPool(concurrency int) *WorkerPool {
    if concurrency <= 0 {
        concurrency = max(runtime.NumCPU()-1, 1)
    }
    wp := &WorkerPool{
        concurrency: concurrency,
        jobs:        make(chan Job, concurrency*8),
        results:     make(chan Result, concurrency*8),
        quit:        make(chan struct{}),
    }
    for i := 0; i < concurrency; i++ {
        wp.wg.Add(1)
        go wp.worker()
    }
    return wp
}

func (wp *WorkerPool) worker() {
    defer wp.wg.Done()
    for {
        select {
        case job, ok := <-wp.jobs:
            if !ok {
                return
            }
            result := wp.process(job)
            wp.results <- result
        case <-wp.quit:
            return
        }
    }
}

func (wp *WorkerPool) process(job Job) Result {
    switch job.Type {
    case JobLint:
        input := job.Data.(LintInput)
        errors, err := lintVerilog(input.Path, input.Content)
        return Result{JobID: job.ID, Type: JobLint, Data: errors, Err: err}

    case JobIndexFile:
        path := job.Data.(string)
        symbols, err := indexVerilogFile(path)
        return Result{JobID: job.ID, Type: JobIndexFile, Data: symbols, Err: err}

    case JobHighlight:
        input := job.Data.(HighlightInput)
        lines := runChromaOnContent(input.Content)
        return Result{JobID: job.ID, Type: JobHighlight, Data: lines}
    }
    return Result{JobID: job.ID, Err: fmt.Errorf("unknown job type %d", job.Type)}
}

// Submit enqueues a job. Non-blocking: drops job if pool is at capacity.
// Returns true if the job was accepted.
func (wp *WorkerPool) Submit(job Job) bool {
    select {
    case wp.jobs <- job:
        return true
    default:
        return false // pool full; caller can retry or discard
    }
}

// NextResult returns a tea.Cmd that waits for the next pool result.
func (wp *WorkerPool) NextResult() tea.Cmd {
    return func() tea.Msg {
        return <-wp.results
    }
}

// Shutdown drains and stops all workers gracefully.
func (wp *WorkerPool) Shutdown() {
    wp.once.Do(func() {
        close(wp.jobs)
        wp.wg.Wait()
        close(wp.results)
    })
}
```

---

### Debounced Background Linter

```go
// internal/tools/linter.go

type BackgroundLinter struct {
    pool    *WorkerPool
    timers  sync.Map // path → *time.Timer
    delay   time.Duration
}

func NewBackgroundLinter(pool *WorkerPool, debounce time.Duration) *BackgroundLinter {
    return &BackgroundLinter{pool: pool, delay: debounce}
}

// LintAsync schedules a lint of `path` after the debounce period.
// Resetting the timer on each call implements "lint after user stops typing."
// This is safe to call from Update() — it never blocks.
func (bl *BackgroundLinter) LintAsync(path, content string) {
    if existing, loaded := bl.timers.LoadAndDelete(path); loaded {
        existing.(*time.Timer).Stop()
    }
    timer := time.AfterFunc(bl.delay, func() {
        bl.pool.Submit(Job{
            ID:   path,
            Type: JobLint,
            Data: LintInput{Path: path, Content: content},
        })
        bl.timers.Delete(path)
    })
    bl.timers.Store(path, timer)
}
```

Usage pattern in `Update()`:

```go
case tea.KeyMsg: // user types
    m.buffer.ApplyEdit(msg)
    // Lint after 600ms of inactivity
    m.linter.LintAsync(m.activeFile(), m.buffer.String())
    return m, m.pool.NextResult() // keep listening for lint results

case Result:
    switch msg.Type {
    case JobLint:
        errors := msg.Data.([]*SimError)
        m.buffers.SetLintErrors(msg.JobID, errors)
        return m, m.pool.NextResult() // wait for next result
    case JobIndexFile:
        symbols := msg.Data.([]*symtable.Symbol)
        m.symTable.Update(msg.JobID, symbols)
        return m, m.pool.NextResult()
    }
```

---

### CPU Budgeting: Protect the TUI Goroutine

Use a semaphore to limit total background goroutines, ensuring at least one CPU core is always available for the TUI event loop.

```go
// internal/tools/budget.go

// BackgroundSemaphore limits concurrent background goroutines to
// (NumCPU - 1), ensuring the TUI always gets a core.
var BackgroundSemaphore = make(chan struct{}, max(runtime.NumCPU()-1, 1))

// RunBackground runs f in a goroutine bounded by the semaphore.
func RunBackground(f func()) {
    BackgroundSemaphore <- struct{}{} // acquire slot (blocks if full)
    go func() {
        defer func() { <-BackgroundSemaphore }() // release on completion
        f()
    }()
}
```

---

<a name="priority-roadmap"></a>
# Priority Roadmap

| Priority | Item | Effort | Impact | Where to Start |
|----------|------|--------|--------|----------------|
| **P0** | Dirty-line highlight cache | 1–2 days | Eliminates editor lag on large files | `internal/tui/hlcache.go` |
| **P0** | Virtual viewport rendering | 1 day | View() now O(viewport) not O(file) | `internal/tui/editor_view.go` |
| **P0** | Async tool execution + error parsing | 1 day | Eliminates all UI freezes | `internal/tools/sim.go` |
| **P0** | VCD streaming parser | 3 days | Handles real simulation outputs | `internal/parser/vcd_stream.go` |
| **P1** | Status bar redesign (mode, lint, sim) | 1 day | Biggest single UX improvement | `internal/tui/statusbar.go` |
| **P1** | Worker pool for linting + indexing | 1 day | Smooth linting with backpressure | `internal/tools/workerpool.go` |
| **P1** | Harmonica spring animations | 0.5 days | Premium feel on scroll/zoom | Already in `go.mod`! |
| **P1** | Adaptive theme via colorprofile | 0.5 days | Correct colors on all terminals | Already in `go.mod`! |
| **P1** | File dependency graph | 2 days | Correct multi-file linting | `internal/parser/depgraph.go` |
| **P2** | VCD checkpoint index for seeking | 3 days | Fast waveform scrubbing | `internal/parser/vcd_stream.go` |
| **P2** | Symbol table + cross-probe MVP | 1 week | The killer feature for RTL work | `internal/symtable/` |
| **P2** | Multi-key sequence buffer | 1 day | Vim-grade keybinding power | `internal/tui/keyseq.go` |
| **P3** | tree-sitter AST integration | 1–2 weeks | Safe rename, auto-instantiate | `internal/parser/ast.go` |
| **P3** | CDC detection pass | 2 weeks | Indispensable for serious RTL | `internal/parser/cdc.go` |
| **P3** | Waveform LOD index | 1 week | Fast zoom-out on huge VCDs | `internal/parser/vcd_lod.go` |
| **P3** | Workspace atomic save + debounce | 0.5 days | Eliminates save-lag spike | `internal/tui/workspace.go` |

---

## Quick Wins: Things Already in Your `go.mod` You're Not Using

| Package | What it gives you | How to use it |
|---------|------------------|---------------|
| `charmbracelet/harmonica` | Spring-physics scroll & zoom animations | `harmonica.NewSpring(harmonica.FPS(60), 6.0, 1.0)` |
| `charmbracelet/colorprofile` | True-color / 256-color / ANSI auto-detection | `colorprofile.Detect(os.Stdout, os.Environ())` |
| `charmbracelet/x/cellbuf` | Efficient cell-level diff rendering | Reduces terminal write bytes on partial redraws |
| `charmbracelet/x/ansi` | Correct ANSI sequence truncation | `ansi.Truncate(s, width, "…")` for tab bar overflow |

> Harmonica and colorprofile are the **fastest ROI** in this entire audit. They are already compiled into your binary and unused. Activating them alone will make the waveform viewer feel like a different product.

---

*End of audit. All code patterns above are idiomatic Go 1.21+ and compatible with BubbleTea v1.3.x.*
