# Vide - Verilog IDE

A lightweight, command-line Verilog/SystemVerilog development environment with auto-simulation, synthesis checking, and an integrated tmux-based IDE.

## Features

- 🚀 **Quick Module Creation** - Generate Verilog/SystemVerilog boilerplate
- 🧪 **Auto Testbench Generation** - Parse ports and create testbenches automatically
- 🔄 **Watch Mode** - Auto-recompile and simulate on file changes
- 📊 **GTKWave Integration** - Launch waveform viewer automatically
- 🔬 **Synthesis Checking** - Verify logic with Yosys
- 📈 **Schematic Viewer** - Visualize your design hierarchy
- 🖥️ **Integrated IDE** - Tmux-based environment with mouse support and easy navigation

## Requirements

### System Dependencies

The following tools must be installed on your system:

- **Python 3.6 or higher** - Core scripting language
- **Icarus Verilog (iverilog)** - For compilation and simulation
- **GTKWave** - Waveform viewer
- **Yosys** - Synthesis and schematic generation
- **xdot** - DOT file viewer for schematics
- **tmux** - Terminal multiplexer for IDE mode
- **nvim/neovim** - Text editor for IDE mode

### Python Dependencies

Vide uses only Python standard library modules:
- `sys`
- `os`
- `re`
- `subprocess`
- `shutil`
- `time`
- `tempfile`

No additional Python packages are required.

## Installation

### Ubuntu/Debian

```bash
sudo apt-get update
sudo apt-get install -y python3 iverilog gtkwave yosys xdot tmux neovim
```

### Fedora/RHEL

```bash
sudo dnf install -y python3 iverilog gtkwave yosys xdot tmux neovim
```

### Arch Linux

```bash
sudo pacman -S python iverilog gtkwave yosys xdot tmux neovim
```

### macOS

```bash
brew install python icarus-verilog gtkwave yosys tmux neovim
brew install graphviz  # includes xdot
```

### Setting up Vide

1. Clone the repository:
```bash
git clone https://github.com/UmarMushtaqMughal/vide.git
cd vide
```

2. Make the script executable:
```bash
chmod +x vide
```

3. (Optional) Add to PATH for easy access:
```bash
sudo ln -s $(pwd)/vide /usr/local/bin/vide
```

## Usage

### Basic Commands

```bash
vide <command> <filename/filelist> [flags]
```

### Command Reference

#### `new` - Create a New Module

Creates a new Verilog or SystemVerilog file with boilerplate code.

```bash
vide new counter.v          # Creates Verilog module
vide new counter.sv         # Creates SystemVerilog module
vide new alu                # Creates alu.sv by default
```

#### `tb` - Generate Testbench

Automatically parses module ports and generates a testbench.

```bash
vide tb counter.v           # Creates counter_tb.v
vide tb counter.sv          # Creates counter_tb.sv
```

#### `sim` - Run Simulation

Compiles and runs simulation, then opens GTKWave.

```bash
vide sim counter.v          # Simulate single file
vide sim files.f            # Simulate multiple files from list
```

#### `watch` - Auto-Simulation on Save

Monitors files and re-runs simulation automatically when changes are detected.

```bash
vide watch counter.v        # Watch mode for single file
vide watch files.f          # Watch mode for file list
```

Press `Ctrl+C` to stop watching.

#### `synth` - Synthesis Check

Runs Yosys synthesis to check logic and show statistics.

```bash
vide synth counter.v        # Check synthesis
vide synth files.f          # Check synthesis for multiple files
```

#### `show` - View Schematic

Generates and displays circuit schematic using Yosys and xdot.

```bash
vide show counter.v             # Gate-level schematic (flattened)
vide show counter.v --prep      # Abstract RTL schematic
vide show counter.v --hier      # Hierarchy view (boxes)
```

**Flags:**
- `--prep`: Abstract RTL representation
- `--hier`: Hierarchical box view

#### `ide` - Integrated Development Environment

Launches a tmux-based IDE with code editor, auto-simulation, and terminal.

```bash
vide ide counter.v          # Open IDE for single file
vide ide files.f            # Open IDE for multiple files
```

**IDE Shortcuts:**
- `Alt + Arrow Keys` - Navigate between panes
- `Alt + z` - Toggle fullscreen on current pane
- `Alt + Shift + Arrow` - Resize panes
- `Alt + q` - Quit IDE
- Mouse enabled for clicking and resizing

The IDE layout includes:
- **Left pane**: Watch mode (auto-simulation)
- **Right top pane**: Neovim editor with files in tabs
- **Right bottom pane**: Command terminal

## File Lists (.f files)

For multi-file projects, create a `.f` file listing all source files:

```
# files.f
counter.v
decoder.v
top.sv
```

Then use it with any command:
```bash
vide sim files.f
vide ide files.f
```

## Examples

### Quick Start Example

```bash
# Create a new module
vide new counter.sv

# Edit the file (add your logic)
# ...

# Generate testbench
vide tb counter.sv

# Run simulation
vide sim counter.sv

# Check synthesis
vide synth counter.sv

# View schematic
vide show counter.sv

# Open in IDE for development
vide ide counter.sv
```

### Multi-File Project

```bash
# Create file list
echo "alu.sv" > design.f
echo "control.sv" >> design.f
echo "top.sv" >> design.f

# Open IDE with all files
vide ide design.f

# Synthesis check
vide synth design.f
```

## Project Structure

```
vide/
├── vide          # Main executable script
└── README.md     # This file
```

## Troubleshooting

### Command Not Found

Make sure all required tools are installed:
```bash
which iverilog gtkwave yosys xdot tmux nvim
```

### Compilation Errors

- Ensure your Verilog/SystemVerilog syntax is correct
- For SystemVerilog, make sure files have `.sv` extension
- Check that all files in `.f` lists exist and are readable

### GTKWave Not Opening

- Verify GTKWave is installed: `which gtkwave`
- Check that VCD file was generated in the working directory

### IDE Issues

- Ensure tmux and neovim are installed
- If panes don't respond to Alt+Arrow, your terminal may not support it
- Try using the default tmux prefix (`Ctrl+b`) instead

## License

This project is open source. Please check the repository for license details.

## Contributing

Contributions are welcome! Please submit issues and pull requests on GitHub.

## Author

UmarMushtaqMughal

## Links

- GitHub: https://github.com/UmarMushtaqMughal/vide
