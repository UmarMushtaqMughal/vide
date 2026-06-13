package tui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/UmarMushtaqMughal/vide/internal/parser"
	"github.com/UmarMushtaqMughal/vide/internal/tools"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model represents the state of the integrated terminal IDE.
type Model struct {
	target        string
	baseName      string
	files         []string
	activeFileIdx int
	activePane    PaneType

	// Multi-File Tabs
	tabs            []string
	activeTabIdx    int
	modifiedBuffers map[string]bool

	// File Explorer vs Hierarchy mode
	leftPaneMode int // 0 = FILE_EXPLORER, 1 = MODULE_HIERARCHY
	lastKey      string

	// File creation prompt
	fileInput   textinput.Model
	isPrompting bool

	// File contents cache
	fileContents map[string]string
	editor       Editor

	isFullScreen bool

	// Terminal / Output log
	outputLog    string
	outputScroll int

	// Waveform view
	waveView WaveformView
	vcdData  *parser.VCDData

	// Window size
	width  int
	height int

	// Status messages
	statusMsg   string
	statusStyle lipgloss.Style

	// Command Palette
	paletteInput   textinput.Model
	isPaletteOpen  bool
	paletteMatches []string
	paletteSelIdx  int

	isFormatPaletteOpen bool
	formatPaletteSelIdx int
	formatPaletteOpts   []string

	// Bootstrap
	bootstrapProg   progress.Model
	bootstrapSpin   spinner.Model
	bootstrapStatus string

	// Linting
	lintTimer int
}

// NewModel initializes and returns a pointer to a new Model.
func NewModel(target string) *Model {
	files, baseName, err := parser.GetSources(target)
	if err != nil {
		files = []string{target}
		baseName = parser.GetModuleName(target)
	}

	// Also auto-discover and append testbench files if they exist
	if !strings.HasSuffix(target, ".f") {
		tbSV := baseName + "_tb.sv"
		tbV := baseName + "_tb.v"
		if _, err := os.Stat(tbSV); err == nil {
			files = append(files, tbSV)
		} else if _, err := os.Stat(tbV); err == nil {
			files = append(files, tbV)
		}
	}

	ti := textinput.New()
	ti.Placeholder = "New filename (e.g., module.sv)"
	ti.CharLimit = 156
	ti.Width = 30

	pi := textinput.New()
	pi.Placeholder = "Search files..."
	pi.CharLimit = 100
	pi.Width = 40

	m := &Model{
		target:          target,
		baseName:        baseName,
		files:           files,
		activePane:      PaneFiles,
		tabs:            []string{}, // Initially empty, will populate below
		modifiedBuffers: make(map[string]bool),
		fileContents:    make(map[string]string),
		editor:          NewEditor(),
		waveView:        NewWaveformView(),
		fileInput:       ti,
		paletteInput:    pi,
		bootstrapProg:   progress.New(progress.WithDefaultGradient()),
		bootstrapSpin:   spinner.New(spinner.WithSpinner(spinner.Dot)),
	}

	m.bootstrapSpin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m.reloadFiles()
	m.loadWorkspace()
	
	// Add initial target to tabs if tabs are empty (no workspace loaded)
	if len(m.tabs) == 0 && len(files) > 0 {
		m.tabs = append(m.tabs, files[0])
		m.activeTabIdx = 0
	}

	m.syncEditor()

	if !tools.IsToolchainPresent() {
		m.activePane = PaneBootstrap
		m.bootstrapStatus = "Initializing Toolchain..."
	}
	m.loadWaveform()

	return m
}

type Workspace struct {
	Files         []string `json:"files"`
	Tabs          []string `json:"tabs"`
	ActiveTabIdx  int      `json:"activeTabIdx"`
	ActiveFileIdx int      `json:"activeFileIdx"`
}

func (m *Model) loadWorkspace() {
	data, err := os.ReadFile(".vide_workspace.json")
	if err == nil {
		var ws Workspace
		if err := json.Unmarshal(data, &ws); err == nil {
			if len(ws.Files) > 0 {
				m.files = ws.Files
			}
			m.tabs = ws.Tabs
			m.activeTabIdx = ws.ActiveTabIdx
			m.activeFileIdx = ws.ActiveFileIdx
			m.reloadFiles()
		}
	}
}

func (m *Model) saveWorkspace() {
	ws := Workspace{
		Files:         m.files,
		Tabs:          m.tabs,
		ActiveTabIdx:  m.activeTabIdx,
		ActiveFileIdx: m.activeFileIdx,
	}
	if data, err := json.MarshalIndent(ws, "", "  "); err == nil {
		os.WriteFile(".vide_workspace.json", data, 0644)
	}
}

func (m *Model) reloadFiles() {
	for _, path := range m.files {
		data, err := os.ReadFile(path)
		if err == nil {
			m.fileContents[path] = string(data)
		} else {
			m.fileContents[path] = fmt.Sprintf("Error reading file: %v", err)
		}
	}
}

func (m *Model) syncEditor() {
	if len(m.tabs) == 0 {
		m.editor.SetContent("", "")
		return
	}
	if m.activeTabIdx >= len(m.tabs) {
		m.activeTabIdx = 0
	}
	current := m.tabs[m.activeTabIdx]
	m.editor.SetContent(m.fileContents[current], current)
}

func (m *Model) loadWaveform() {
	vcdPath := m.baseName + ".vcd"
	if _, err := os.Stat(vcdPath); err == nil {
		data, err := parser.ParseVCD(vcdPath)
		if err == nil {
			m.vcdData = data
			m.waveView.SetData(data)
		}
	}
}

// Init initializes the Bubble Tea program.
func (m *Model) Init() tea.Cmd {
	var cmds []tea.Cmd
	cmds = append(cmds, textinput.Blink)
	if m.activePane == PaneBootstrap {
		cmds = append(cmds, m.bootstrapSpin.Tick)
		cmds = append(cmds, startBootstrapCmd())
	}
	return tea.Batch(cmds...)
}

func startBootstrapCmd() tea.Cmd {
	return func() tea.Msg {
		ch := make(chan tools.ProgressMsg)
		go tools.DownloadAndExtract(ch)
		return waitForProgress(ch)()
	}
}

func waitForProgress(ch chan tools.ProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return bootstrapCompleteMsg{}
		}
		return bootstrapProgressMsg{
			Msg: msg,
			Ch:  ch,
		}
	}
}

type bootstrapProgressMsg struct {
	Msg tools.ProgressMsg
	Ch  chan tools.ProgressMsg
}

type bootstrapCompleteMsg struct{}

// Msg types for async operations
type simResultMsg struct {
	output string
	err    error
}

type synthResultMsg struct {
	output string
	err    error
}

type lintTickMsg int

type lintResultMsg struct {
	errors []int
}

func runLinterCmd(content, ext string) tea.Cmd {
	return func() tea.Msg {
		tmpfile, err := os.CreateTemp("", "vide_lint_*"+ext)
		if err != nil {
			return lintResultMsg{}
		}
		defer os.Remove(tmpfile.Name())
		tmpfile.WriteString(content)
		tmpfile.Close()

		cmd := exec.Command(tools.GetBinPath("iverilog"), "-t", "null", tmpfile.Name())
		out, _ := cmd.CombinedOutput()
		
		var errLines []int
		lines := strings.Split(string(out), "\n")
		for _, l := range lines {
			parts := strings.SplitN(l, ":", 3)
			if len(parts) >= 3 {
				var lineNum int
				fmt.Sscanf(parts[1], "%d", &lineNum)
				if lineNum > 0 {
					errLines = append(errLines, lineNum-1)
				}
			}
		}
		return lintResultMsg{errors: errLines}
	}
}

func runSimCmd(files []string, target string) tea.Cmd {
	return func() tea.Msg {
		if err := tools.CheckTools("iverilog", "vvp"); err != nil {
			return simResultMsg{err: err}
		}

		baseName := strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))

		if !strings.HasSuffix(target, ".f") {
			tbSV := baseName + "_tb.sv"
			tbV := baseName + "_tb.v"
			
			addIfMissing := func(tb string) {
				if _, err := os.Stat(tb); err == nil {
					found := false
					for _, f := range files {
						if f == tb {
							found = true
							break
						}
					}
					if !found {
						files = append(files, tb)
					}
				}
			}
			addIfMissing(tbSV)
			addIfMissing(tbV)
		}

		for _, f := range files {
			if _, err := os.Stat(f); os.IsNotExist(err) {
				return simResultMsg{err: fmt.Errorf("source file '%s' not found", f)}
			}
		}

		outFile := baseName + ".vvp"

		hasSV := false
		for _, f := range files {
			if strings.HasSuffix(f, ".sv") {
				hasSV = true
				break
			}
		}

		args := []string{}
		if hasSV {
			args = append(args, "-g2012")
		}
		args = append(args, "-o", outFile)
		args = append(args, files...)

		var outBuf bytes.Buffer
		compileCmd := exec.Command(tools.GetBinPath("iverilog"), args...)
		compileCmd.Stdout = &outBuf
		compileCmd.Stderr = &outBuf
		if err := compileCmd.Run(); err != nil {
			return simResultMsg{
				output: outBuf.String(),
				err:    fmt.Errorf("compilation failed"),
			}
		}

		simCmd := exec.Command(tools.GetBinPath("vvp"), outFile)
		var simBuf bytes.Buffer
		simCmd.Stdout = &simBuf
		simCmd.Stderr = &simBuf
		if err := simCmd.Run(); err != nil {
			return simResultMsg{
				output: outBuf.String() + "\n" + simBuf.String(),
				err:    fmt.Errorf("simulation runtime error"),
			}
		}

		return simResultMsg{
			output: outBuf.String() + "\n" + simBuf.String(),
			err:    nil,
		}
	}
}

func runSynthCmd(files []string, target string) tea.Cmd {
	return func() tea.Msg {
		// Tools are guaranteed by bootstrap

		_, topModule, _ := parser.GetSources(target) // Still need topModule name for synth
		if topModule == "" {
			topModule = strings.TrimSuffix(filepath.Base(target), filepath.Ext(target))
		}

		hasSV := false
		for _, f := range files {
			if strings.HasSuffix(f, ".sv") {
				hasSV = true
				break
			}
		}

		readCmd := "read_verilog"
		if hasSV {
			readCmd = "read_verilog -sv"
		}

		var loadParts []string
		for _, f := range files {
			if strings.HasSuffix(f, "_tb.sv") || strings.HasSuffix(f, "_tb.v") {
				continue // Do not synthesize testbenches
			}
			loadParts = append(loadParts, fmt.Sprintf("%s %s", readCmd, f))
		}
		loadScript := strings.Join(loadParts, "; ")
		yosysScript := fmt.Sprintf("%s; synth -top %s; stat", loadScript, topModule)

		yosysCmd := exec.Command(tools.GetBinPath("yosys"), "-p", yosysScript)
		var outBuf bytes.Buffer
		yosysCmd.Stdout = &outBuf
		yosysCmd.Stderr = &outBuf
		if err := yosysCmd.Run(); err != nil {
			return synthResultMsg{
				output: outBuf.String(),
				err:    fmt.Errorf("synthesis failed"),
			}
		}

		return synthResultMsg{
			output: outBuf.String(),
			err:    nil,
		}
	}
}

// Update handles incoming messages and events.
func (m *Model) Update(teaMsg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	if m.activePane == PaneBootstrap {
		switch msg := teaMsg.(type) {
		case tea.KeyMsg:
			if msg.String() == "q" || msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		case spinner.TickMsg:
			var sCmd tea.Cmd
			m.bootstrapSpin, sCmd = m.bootstrapSpin.Update(msg)
			return m, sCmd
		case bootstrapProgressMsg:
			if msg.Msg.Err != nil {
				m.bootstrapStatus = "Error: " + msg.Msg.Err.Error()
				return m, nil
			}
			m.bootstrapStatus = msg.Msg.Status
			var pCmd tea.Cmd
			pCmd = m.bootstrapProg.SetPercent(msg.Msg.Percent)
			return m, tea.Batch(pCmd, waitForProgress(msg.Ch))
		case bootstrapCompleteMsg:
			m.activePane = PaneFiles
			m.statusMsg = "Toolchain installed successfully!"
			m.statusStyle = StyleSimSuccess
			return m, nil
		case progress.FrameMsg:
			var pModel tea.Model
			var pCmd tea.Cmd
			pModel, pCmd = m.bootstrapProg.Update(msg)
			m.bootstrapProg = pModel.(progress.Model)
			return m, pCmd
		case tea.WindowSizeMsg:
			m.width = msg.Width
			m.height = msg.Height
			m.editor.SetSize(msg.Width-2, msg.Height-4)
			return m, nil
		}
		return m, nil
	}

	if m.outputLog != "" {
		if keyMsg, ok := teaMsg.(tea.KeyMsg); ok {
			if keyMsg.String() == "q" || keyMsg.String() == "ctrl+c" {
				m.saveWorkspace()
				return m, tea.Quit
			}
		}
	}

	if m.isPaletteOpen {
		switch msg := teaMsg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.isPaletteOpen = false
				m.paletteInput.Blur()
				return m, nil
			case "enter":
				if len(m.paletteMatches) > 0 && m.paletteSelIdx >= 0 && m.paletteSelIdx < len(m.paletteMatches) {
					fileToAdd := m.paletteMatches[m.paletteSelIdx]
					found := -1
					for i, t := range m.tabs {
						if t == fileToAdd {
							found = i
							break
						}
					}
					if found >= 0 {
						m.activeTabIdx = found
					} else {
						m.tabs = append(m.tabs, fileToAdd)
						m.activeTabIdx = len(m.tabs) - 1
					}
					m.syncEditor()
					m.isPaletteOpen = false
					m.paletteInput.Blur()
					m.activePane = PaneCode
					m.statusMsg = "Opened " + filepath.Base(fileToAdd)
					m.statusStyle = StyleSimSuccess
					return m, nil
				}
			case "up", "ctrl+k":
				if m.paletteSelIdx > 0 {
					m.paletteSelIdx--
				}
				return m, nil
			case "down", "ctrl+j":
				if m.paletteSelIdx < len(m.paletteMatches)-1 {
					m.paletteSelIdx++
				}
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.paletteInput, cmd = m.paletteInput.Update(teaMsg)
		m.updatePaletteMatches()
		return m, cmd
	}

	if m.isFormatPaletteOpen {
		switch msg := teaMsg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.isFormatPaletteOpen = false
				return m, nil
			case "enter":
				if len(m.waveView.signals) > 0 {
					sig := m.waveView.signals[m.waveView.selectedIdx]
					m.waveView.formats[sig.Name] = m.formatPaletteOpts[m.formatPaletteSelIdx]
				}
				m.isFormatPaletteOpen = false
				return m, nil
			case "up", "k":
				if m.formatPaletteSelIdx > 0 {
					m.formatPaletteSelIdx--
				}
				return m, nil
			case "down", "j":
				if m.formatPaletteSelIdx < len(m.formatPaletteOpts)-1 {
					m.formatPaletteSelIdx++
				}
				return m, nil
			}
		}
		return m, nil
	}

	if m.isPrompting {
		switch msg := teaMsg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.isPrompting = false
				m.fileInput.Blur()
				m.statusMsg = ""
				return m, nil
			case "enter":
				val := m.fileInput.Value()
				if val != "" {
					// Create the file
					if _, err := os.Stat(val); os.IsNotExist(err) {
						os.WriteFile(val, []byte(""), 0644)
					}
					m.files = append(m.files, val)
					m.reloadFiles()
					m.activeFileIdx = len(m.files) - 1
					
					// Open it
					m.tabs = append(m.tabs, val)
					m.activeTabIdx = len(m.tabs) - 1
					m.syncEditor()
					m.activePane = PaneCode
					m.statusMsg = "Created and opened " + val
					m.statusStyle = StyleSimSuccess
				}
				m.isPrompting = false
				m.fileInput.Blur()
				return m, nil
			}
		}
		var cmd tea.Cmd
		m.fileInput, cmd = m.fileInput.Update(teaMsg)
		return m, cmd
	}

	switch msg := teaMsg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q":
			if m.activePane == PaneCode && m.editor.isEditing {
				break
			}
			m.saveWorkspace()
			return m, tea.Quit

		case "ctrl+c":
			m.saveWorkspace()
			return m, tea.Quit

		case "alt+q":
			m.saveWorkspace()
			return m, tea.Quit

		case "ctrl+p":
			if m.activePane == PaneCode && m.editor.isEditing {
				break
			}
			m.isPaletteOpen = true
			m.paletteInput.SetValue("")
			m.paletteInput.Focus()
			m.updatePaletteMatches()
			return m, textinput.Blink

		case "alt+z":
			if m.activePane == PaneCode && m.editor.isEditing {
				break
			}
			// Toggle leftPaneMode
			m.leftPaneMode = 1 - m.leftPaneMode
			if m.leftPaneMode == 1 {
				m.statusMsg = "Module Hierarchy View"
			} else {
				m.statusMsg = "File Explorer View"
			}
			m.statusStyle = StyleSimSuccess

		case "esc":
			if m.activePane == PaneCode && m.editor.isEditing {
				m.editor.Blur()
				m.fileContents[m.getCurrentFile()] = m.editor.GetContent()
				m.statusMsg = "Saved locally. Press 's' to simulate."
				m.statusStyle = StyleSimSuccess
			}

		case "tab":
			if m.activePane == PaneCode && m.editor.isEditing {
				// pass to editor
				break
			}
			m.activePane = (m.activePane + 1) % 4
			m.editor.Blur()
			m.statusMsg = ""

		case "shift+tab":
			if m.activePane == PaneCode && m.editor.isEditing {
				break
			}
			m.activePane = (m.activePane + 3) % 4
			m.editor.Blur()
			m.statusMsg = ""

		case "g":
			if m.activePane == PaneCode && m.editor.isEditing {
				break
			}
			m.lastKey = "g"
			// Wait for next key

		case "t", "T":
			if m.activePane == PaneCode && m.editor.isEditing {
				break
			}
			if m.lastKey == "g" {
				// Tab switching
				if len(m.tabs) > 0 {
					if msg.String() == "t" {
						m.activeTabIdx = (m.activeTabIdx + 1) % len(m.tabs)
					} else { // "T"
						m.activeTabIdx = (m.activeTabIdx - 1 + len(m.tabs)) % len(m.tabs)
					}
					m.syncEditor()
					m.statusMsg = "Switched tab"
					m.statusStyle = StyleSimSuccess
				}
				m.lastKey = ""
				break
			}
			
			// Get the active file
			activeFile := m.target
			if len(m.tabs) > 0 {
				activeFile = m.tabs[m.activeTabIdx]
			}
			
			// Save active buffer before generating TB so the file on disk is fresh
			if m.activePane == PaneCode && len(m.tabs) > 0 {
				m.fileContents[activeFile] = m.editor.GetContent()
				os.WriteFile(activeFile, []byte(m.editor.GetContent()), 0644)
			}

			// Generate TB for the active file
			tbFile, err := tools.GenerateTB(activeFile)
			if err != nil {
				m.statusMsg = fmt.Sprintf("Error generating TB: %v", err)
				m.statusStyle = StyleSimError
			} else {
				m.statusMsg = fmt.Sprintf("Generated %s", tbFile)
				m.statusStyle = StyleSimSuccess
				
				// Append new TB file to m.files if not present
				found := false
				for _, f := range m.files {
					if f == tbFile {
						found = true
						break
					}
				}
				if !found {
					m.files = append(m.files, tbFile)
				}
				m.reloadFiles()
			}

		case "i":
			if m.activePane == PaneCode && !m.editor.isEditing {
				m.editor.Focus()
				m.statusMsg = "-- INSERT MODE --"
				m.statusStyle = StyleSimRunning
				return m, nil
			}

		case "enter":
			if m.activePane == PaneFiles {
				// Open selected file in a tab
				if len(m.files) > 0 && m.activeFileIdx < len(m.files) {
					fileToAdd := m.files[m.activeFileIdx]
					// Check if already in tabs
					found := -1
					for i, t := range m.tabs {
						if t == fileToAdd {
							found = i
							break
						}
					}
					if found >= 0 {
						m.activeTabIdx = found
					} else {
						m.tabs = append(m.tabs, fileToAdd)
						m.activeTabIdx = len(m.tabs) - 1
					}
					m.syncEditor()
					m.activePane = PaneCode
					m.statusMsg = "Opened " + filepath.Base(fileToAdd)
					m.statusStyle = StyleSimSuccess
				}
			} else if m.activePane == PaneCode && !m.editor.isEditing {
				m.editor.Focus()
				m.statusMsg = "-- INSERT MODE --"
				m.statusStyle = StyleSimRunning
				return m, nil
			}

		case "F":
			if m.activePane == PaneCode && m.editor.isEditing {
				break
			}
			m.isFullScreen = !m.isFullScreen
			m.updatePaneSizes()
			m.statusMsg = ""

		case "s":
			if m.activePane == PaneCode && m.editor.isEditing {
				break // insert 's'
			}
			if m.activePane == PaneCode && len(m.tabs) > 0 {
				m.fileContents[m.tabs[m.activeTabIdx]] = m.editor.GetContent()
				os.WriteFile(m.tabs[m.activeTabIdx], []byte(m.editor.GetContent()), 0644)
			}
			m.statusMsg = "Simulating..."
			m.statusStyle = StyleSimRunning
			m.reloadFiles()
			return m, runSimCmd(m.files, m.target)

		case "y":
			if m.activePane == PaneCode && m.editor.isEditing {
				break // insert 'y'
			}
			if m.activePane == PaneCode && len(m.tabs) > 0 {
				m.fileContents[m.tabs[m.activeTabIdx]] = m.editor.GetContent()
				os.WriteFile(m.tabs[m.activeTabIdx], []byte(m.editor.GetContent()), 0644)
			}
			m.statusMsg = "Synthesizing..."
			m.statusStyle = StyleSimRunning
			m.reloadFiles()
			return m, runSynthCmd(m.files, m.target)

		case "n":
			if m.activePane == PaneFiles {
				m.isPrompting = true
				m.fileInput.SetValue("")
				m.fileInput.Focus()
				m.statusMsg = "-- NEW FILE --"
				m.statusStyle = StyleSimRunning
				return m, textinput.Blink
			}

		default:
			// Route navigation keys based on active pane
			if m.activePane == PaneCode && m.editor.isEditing {
				break
			}
			// Clear lastKey if it's not g
			if msg.String() != "g" {
				m.lastKey = ""
			}
			m.handleNavigation(msg.String())
		}
		
		if m.activePane == PaneCode && m.editor.isEditing {
			var eCmd tea.Cmd
			m.editor, eCmd = m.editor.Update(msg)
			
			// Schedule lint
			m.lintTimer++
			currentTimer := m.lintTimer
			lintCmd := tea.Tick(1*time.Second, func(t time.Time) tea.Msg {
				return lintTickMsg(currentTimer)
			})
			
			cmd = tea.Batch(cmd, eCmd, lintCmd)
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionRelease && msg.Button == tea.MouseButtonLeft {
			th := m.height * 55 / 100
			lw := m.width * 25 / 100
			if msg.Y >= 1 && msg.Y < th+1 {
				if msg.X < lw {
					m.activePane = PaneFiles
					if msg.Y >= 2 && msg.Y-2 < len(m.files) {
						m.activeFileIdx = msg.Y - 2
						m.syncEditor()
					}
				} else {
					m.activePane = PaneCode
					if msg.Y == 1 {
						tabWidthApprox := 15
						tabIdx := (msg.X - lw) / tabWidthApprox
						if tabIdx >= 0 && tabIdx < len(m.tabs) {
							m.activeTabIdx = tabIdx
							m.syncEditor()
						}
					}
				}
			} else if msg.Y > th+1 && msg.Y < m.height-1 {
				if msg.X < lw {
					m.activePane = PaneTerminal
				} else {
					m.activePane = PaneWaveform
				}
			}
		}

	case lintTickMsg:
		if int(msg) == m.lintTimer && len(m.tabs) > 0 && m.activePane != PaneBootstrap {
			return m, runLinterCmd(m.editor.GetContent(), filepath.Ext(m.getCurrentFile()))
		}

	case lintResultMsg:
		m.editor.SetErrors(msg.errors)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.updatePaneSizes()

	case simResultMsg:
		m.outputLog = msg.output
		if msg.err != nil {
			m.statusMsg = "Simulation Failed"
			m.statusStyle = StyleSimError
		} else {
			m.statusMsg = "Simulation Success"
			m.statusStyle = StyleSimSuccess
			m.loadWaveform()
		}
		m.outputScroll = 0

	case synthResultMsg:
		if msg.err != nil {
			m.outputLog = msg.output
			m.statusMsg = "Synthesis Failed"
			m.statusStyle = StyleSimError
		} else {
			// Extract Yosys stats clean lines
			var statLines []string
			for _, line := range strings.Split(msg.output, "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.Contains(trimmed, "Number of wires:") ||
					strings.Contains(trimmed, "Number of cells:") ||
					strings.Contains(trimmed, "Chip area") ||
					strings.Contains(trimmed, "printing statistics") {
					statLines = append(statLines, "   "+trimmed)
				}
			}
			if len(statLines) > 0 {
				m.outputLog = "Synthesis Statistics:\n" + strings.Join(statLines, "\n")
			} else {
				m.outputLog = msg.output
			}
			m.statusMsg = "Synthesis Success"
			m.statusStyle = StyleSimSuccess
		}
		m.outputScroll = 0
	}

	return m, cmd
}

func (m *Model) handleNavigation(key string) {
	switch m.activePane {
	case PaneFiles:
		switch key {
		case "up", "k":
			if m.activeFileIdx > 0 {
				m.activeFileIdx--
				m.syncEditor()
			}
		case "down", "j":
			if m.activeFileIdx < len(m.files)-1 {
				m.activeFileIdx++
				m.syncEditor()
			}
		}

	case PaneCode:
		// When not editing, we can still pass up/down to editor to scroll
		switch key {
		case "up", "k":
			m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyUp})
		case "down", "j":
			m.editor, _ = m.editor.Update(tea.KeyMsg{Type: tea.KeyDown})
		}

	case PaneTerminal:
		lines := strings.Split(m.outputLog, "\n")
		bh := m.height - 2 - (m.height * 55 / 100)
		maxScroll := len(lines) - (bh - 2)
		if maxScroll < 0 {
			maxScroll = 0
		}

		switch key {
		case "up", "k":
			if m.outputScroll > 0 {
				m.outputScroll--
			}
		case "down", "j":
			if m.outputScroll < maxScroll {
				m.outputScroll++
			}
		}

	case PaneWaveform:
		switch key {
		case "left", "h":
			m.waveView.CursorLeft()
		case "right", "l":
			m.waveView.CursorRight()
		case "[":
			m.waveView.EdgeLeft()
		case "]":
			m.waveView.EdgeRight()
		case "+":
			m.waveView.ZoomIn()
		case "-":
			m.waveView.ZoomOut()
		case "up", "k":
			m.waveView.SelectUp()
		case "down", "j":
			m.waveView.SelectDown()
		case " ", "f":
			m.isFormatPaletteOpen = true
			m.formatPaletteSelIdx = 0
			m.formatPaletteOpts = []string{"hex", "bin", "dec", "udec"}
		}
	}
}

func (m *Model) updatePaletteMatches() {
	query := strings.ToLower(m.paletteInput.Value())
	var matches []string
	for _, f := range m.files {
		if query == "" || strings.Contains(strings.ToLower(f), query) {
			matches = append(matches, f)
		}
	}
	m.paletteMatches = matches
	if m.paletteSelIdx >= len(matches) {
		m.paletteSelIdx = len(matches) - 1
	}
	if m.paletteSelIdx < 0 && len(matches) > 0 {
		m.paletteSelIdx = 0
	}
}

func (m *Model) getCurrentFile() string {
	if len(m.files) == 0 {
		return ""
	}
	if m.activeFileIdx >= len(m.files) {
		m.activeFileIdx = 0
	}
	return m.files[m.activeFileIdx]
}

func (m *Model) updatePaneSizes() {
	var cw, ch, ww, wh int
	if m.isFullScreen {
		cw, ch = m.width, m.height-2
		ww, wh = m.width, m.height-2
	} else {
		th := m.height * 55 / 100
		bh := m.height - 2 - th
		lw := m.width * 25 / 100
		rw := m.width - lw
		cw, ch = rw, th
		ww, wh = rw, bh
	}

	// Propagate size to waveform view
	m.waveView.SetSize(ww-2, wh-2)
	
	// Propagate size to editor
	m.editor.SetSize(cw-2, ch-4) // Subtract 2 more for the tab bar
}

// View constructs the overall text layout of the application.
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing Vide Studio..."
	}

	// Layout parameters
	th := m.height * 55 / 100
	bh := m.height - 2 - th

	lw := m.width * 25 / 100
	rw := m.width - lw

	// Title bar
	titleBarText := fmt.Sprintf(" VIDE STUDIO — %s ", filepath.Base(m.target))
	title := StyleTitle.Render(titleBarText)
	header := title + strings.Repeat(" ", m.width-lipgloss.Width(title))

	// Pane 1: File List or Module Hierarchy
	var fileListPane, codePane, terminalPane, waveformPane string

	if m.activePane == PaneBootstrap {
		msg := fmt.Sprintf("\n\n%s %s\n\n%s\n\n", m.bootstrapSpin.View(), m.bootstrapStatus, m.bootstrapProg.View())
		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Width(60).
			Align(lipgloss.Center).
			Render(msg)
		
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box, lipgloss.WithWhitespaceChars(" "))
	}

	if !m.isFullScreen || m.activePane == PaneFiles {
		var fileListContent string
		
		if m.isPrompting {
			fileListContent = "Create new file:\n\n" + m.fileInput.View()
		} else if m.leftPaneMode == 0 {
			// FILE EXPLORER
			var fileListItems []string
			for i, f := range m.files {
				displayName := filepath.Base(f)
				if i == m.activeFileIdx {
					fileListItems = append(fileListItems, StyleActiveFile.Render("> "+displayName))
				} else {
					fileListItems = append(fileListItems, StyleFileName.Render("  "+displayName))
				}
			}
			fileListContent = strings.Join(fileListItems, "\n")
		} else {
			// MODULE HIERARCHY
			var hierarchyItems []string
			hierarchyItems = append(hierarchyItems, StyleTitle.Render(" Module Hierarchy "))
			for _, f := range m.files {
				code := m.fileContents[f]
				instances := parser.ParseHierarchy(code)
				if len(instances) > 0 {
					base := filepath.Base(f)
					hierarchyItems = append(hierarchyItems, StyleFileName.Render("▾ "+base))
					for _, inst := range instances {
						hierarchyItems = append(hierarchyItems, "  ├─ "+inst.InstanceName+" ("+inst.ModuleName+")")
					}
				}
			}
			if len(hierarchyItems) == 1 {
				hierarchyItems = append(hierarchyItems, "  No instantiations found.")
			}
			fileListContent = strings.Join(hierarchyItems, "\n")
		}
		
		fw, fh := lw, th
		if m.isFullScreen {
			fw, fh = m.width, m.height-2
		}
		fileListPane = getPaneStyle(m.activePane == PaneFiles).
			Width(fw - 2).
			Height(fh - 2).
			Render(fileListContent)
	}

	// Pane 2: Code Viewer
	if !m.isFullScreen || m.activePane == PaneCode {
		cw, ch := rw, th
		if m.isFullScreen {
			cw, ch = m.width, m.height-2
		}

		// Render Tab Bar
		var tabStrs []string
		for i, t := range m.tabs {
			base := filepath.Base(t)
			if i == m.activeTabIdx {
				tabStrs = append(tabStrs, lipgloss.NewStyle().Background(lipgloss.Color("4")).Foreground(lipgloss.Color("0")).Padding(0, 1).Render(base))
			} else {
				tabStrs = append(tabStrs, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Padding(0, 1).Render(base))
			}
		}
		tabBar := lipgloss.JoinHorizontal(lipgloss.Top, tabStrs...)
		if len(m.tabs) == 0 {
			tabBar = "No open files"
		}

		codeContent := m.editor.View()
		fullCodeView := lipgloss.JoinVertical(lipgloss.Left, tabBar, codeContent)
		
		codePane = getPaneStyle(m.activePane == PaneCode).
			Width(cw - 2).
			Height(ch - 2).
			Render(fullCodeView)
	}

	// Pane 3: Terminal / Output
	if !m.isFullScreen || m.activePane == PaneTerminal {
		tw, th2 := lw, bh
		if m.isFullScreen {
			tw, th2 = m.width, m.height-2
		}
		var termContent string
		if m.outputLog != "" {
			lines := strings.Split(m.outputLog, "\n")
			startLine := m.outputScroll
			endLine := startLine + (th2 - 2)
			if endLine > len(lines) {
				endLine = len(lines)
			}
			var visibleLines []string
			for idx := startLine; idx < endLine; idx++ {
				line := lines[idx]
				if len(line) > tw-4 {
					line = line[:tw-4] + "..."
				}
				visibleLines = append(visibleLines, line)
			}
			termContent = strings.Join(visibleLines, "\n")
		} else {
			termContent = "Terminal ready.\nPress 's' to simulate\nPress 'y' to synthesize"
		}
		terminalPane = getPaneStyle(m.activePane == PaneTerminal).
			Width(tw - 2).
			Height(th2 - 2).
			Render(termContent)
	}

	// Pane 4: Waveform Viewer
	if !m.isFullScreen || m.activePane == PaneWaveform {
		ww, wh := rw, bh
		if m.isFullScreen {
			ww, wh = m.width, m.height-2
		}
		waveformContent := m.waveView.Render()
		waveformPane = getPaneStyle(m.activePane == PaneWaveform).
			Width(ww - 2).
			Height(wh - 2).
			Render(waveformContent)
	}

	// Compose layout
	var mainGrid string
	if m.isFullScreen {
		switch m.activePane {
		case PaneFiles:
			mainGrid = fileListPane
		case PaneCode:
			mainGrid = codePane
		case PaneTerminal:
			mainGrid = terminalPane
		case PaneWaveform:
			mainGrid = waveformPane
		}
	} else {
		topRow := lipgloss.JoinHorizontal(lipgloss.Top, fileListPane, codePane)
		bottomRow := lipgloss.JoinHorizontal(lipgloss.Top, terminalPane, waveformPane)
		mainGrid = lipgloss.JoinVertical(lipgloss.Left, topRow, bottomRow)
	}

	// Status bar / help bar
	status := " [Tab] Cycle Pane  [F] Fullscreen  [s] Sim  [y] Synth  [t] Gen TB  [ctrl+p] Palette  [q] Quit"
	if m.activePane == PaneWaveform {
		status += "  [←→] Scroll  [+/-] Zoom  [ / ] Edges"
	}

	statusBarWidth := m.width
	if m.statusMsg != "" {
		renderedStatus := m.statusStyle.Render(" " + m.statusMsg + " ")
		statusBarWidth -= lipgloss.Width(renderedStatus)
		status = status + strings.Repeat(" ", statusBarWidth-lipgloss.Width(status)) + renderedStatus
	} else {
		status = status + strings.Repeat(" ", statusBarWidth-lipgloss.Width(status))
	}
	statusBar := StyleStatusBar.Render(status)

	mainView := lipgloss.JoinVertical(lipgloss.Left, header, mainGrid, statusBar)

	if m.isPaletteOpen {
		var lines []string
		lines = append(lines, StyleTitle.Render(" [ COMMAND PALETTE ] "))
		lines = append(lines, m.paletteInput.View())
		lines = append(lines, "")
		for i, match := range m.paletteMatches {
			if i == m.paletteSelIdx {
				lines = append(lines, StyleActiveFile.Render("> "+match))
			} else {
				lines = append(lines, "  "+match)
			}
			if i > 10 {
				break // Limit matches shown
			}
		}
		paletteBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Render(strings.Join(lines, "\n"))
		
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, paletteBox, lipgloss.WithWhitespaceChars(" "))
	}

	if m.isFormatPaletteOpen {
		var lines []string
		lines = append(lines, StyleTitle.Render(" [ FORMAT PALETTE ] "))
		lines = append(lines, "")
		for i, opt := range m.formatPaletteOpts {
			label := ""
			switch opt {
			case "hex": label = "Hexadecimal"
			case "bin": label = "Binary"
			case "dec": label = "Decimal (Signed)"
			case "udec": label = "Decimal (Unsigned)"
			}
			if i == m.formatPaletteSelIdx {
				lines = append(lines, StyleActiveFile.Render("> "+label))
			} else {
				lines = append(lines, "  "+label)
			}
		}
		paletteBox := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 2).
			Render(strings.Join(lines, "\n"))
		
		return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, paletteBox, lipgloss.WithWhitespaceChars(" "))
	}

	return mainView
}

func getPaneStyle(active bool) lipgloss.Style {
	if active {
		return StyleActivePane
	}
	return StyleInactivePane
}
