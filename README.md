# Vide - Verilog IDE

A lightweight, terminal-based Verilog/SystemVerilog development environment built with Go and Bubble Tea. It features an integrated IDE layout with auto-simulation, syntax highlighting, waveform inspection, module hierarchy navigation, and live linting.

## Features

- **Multi-File Integration:** Edit multiple files in tabbed buffers seamlessly.
- **Module Hierarchy Navigation:** View instantiated modules and project structure dynamically.
- **Advanced Waveform Viewer:** Inspect simulation outputs interactively with zooming, format switching, and edge searching directly in the terminal.
- **Command Palette:** Quickly navigate files using fuzzy search.
- **Live Background Linting:** Detect syntax errors continuously in the background and view them instantly in the editor gutter.
- **Auto-completion Snippets:** Generate standard Verilog boilerplate (`module`, `always_ff`, etc.) instantly via `<Tab>`.
- **Auto Testbench Generation:** Automatically parse logic ports and scaffold testbenches.
- **Workspace Persistence:** Remembers open tabs and explorer state across sessions.

## Requirements

Vide is designed to be completely frictionless. On its first run, it will automatically download and isolate the required OSS CAD Suite toolchain (Icarus Verilog, Yosys) in the background.

- **Go 1.20+** - Only required if building from source.

## Installation

The easiest way to use Vide is to download a pre-compiled binary. 

1. Go to the [GitHub Releases page](https://github.com/UmarMushtaqMughal/vide/releases).
2. Download the `.zip` (Windows) or `.tar.gz` (Linux/macOS) for your system architecture. Native Linux packages (`.deb`, `.rpm`, `.apk`) are also available.
3. Extract the `vide` executable.
4. (Optional) Add it to your system PATH:
   - **Linux/macOS:** `sudo mv vide /usr/local/bin/`
   - **Windows:** Move `vide.exe` to a directory included in your Environment Variables PATH.

### Building from Source

If you prefer to compile the IDE yourself:

```bash
git clone https://github.com/UmarMushtaqMughal/vide.git
cd vide
go build -o vide main.go
```

## Usage

Start the IDE by pointing it to a Verilog file or a directory:
```bash
vide <filename>
```

### IDE Keyboard Shortcuts

**Global Navigation & Actions:**
- `Tab` / `Shift+Tab`: Cycle focus between panes (Explorer, Editor, Terminal, Waveform)
- `F`: Toggle fullscreen mode for the active pane
- `Alt+Z`: Toggle between File Explorer and Module Hierarchy view
- `Ctrl+P`: Open Command Palette for fuzzy file finding
- `s`: Compile and run simulation (generates waveform data)
- `y`: Run synthesis check
- `t`: Auto-generate testbench for the active file
- `q` / `Ctrl+C` / `Alt+Q`: Quit the IDE (Workspace state is saved automatically)

**Editor (Insert Mode):**
- `Esc`: Exit insert mode and save the buffer
- `Tab`: Trigger auto-completion snippet (e.g., type `module`, then hit `Tab`)

**Waveform Viewer:**
- `h` / `l` or `Left` / `Right`: Scroll trace cursor horizontally
- `+` / `-`: Zoom in and out on the time axis
- `[` / `]`: Jump to the previous or next logic transition edge
- `j` / `k` or `Up` / `Down`: Select signals in the inspector
- `Space` or `f`: Open the Data Format Palette to change signal representation (Binary, Hex, etc.)

## Project Structure

- `main.go`: Application entrypoint
- `internal/tui/`: Contains Bubble Tea rendering and state logic (Model, Editor, Waveform)
- `internal/parser/`: Tools for parsing VCD outputs and Verilog hierarchy
- `internal/tools/`: Integration with Icarus Verilog and synthesis commands

## License

This project is open-source. Please refer to the repository for license details.

## Author

UmarMushtaqMughal
