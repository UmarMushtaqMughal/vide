package parser

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
)

// SignalWave uses a sparse SoA representation for memory efficiency.
type SignalWave struct {
	Times  []uint64
	Values []string // Could be optimized to []byte if values are purely 1-bit or short
}

func (sw *SignalWave) ValueAt(t uint64) string {
	if len(sw.Times) == 0 {
		return "x"
	}
	idx := sort.Search(len(sw.Times), func(i int) bool {
		return sw.Times[i] > t
	}) - 1
	if idx < 0 {
		return "x"
	}
	return sw.Values[idx]
}

func (sw *SignalWave) Append(time uint64, val string) {
	sw.Times = append(sw.Times, time)
	sw.Values = append(sw.Values, val)
}

// VCDSignal represents a single signal.
type VCDSignal struct {
	Name  string
	Width int
	ID    string
	Wave  *SignalWave
}

// VCDData holds header info and the signal slice.
type VCDData struct {
	Timescale string
	Signals   []VCDSignal
	EndTime   uint64
}

type VCDChunk struct {
	Updates map[int]*SignalWave // index into data.Signals -> new transitions
	EndTime uint64
	Err     error
}

// VCDStreamParser handles streaming a VCD file in chunks.
type VCDStreamParser struct {
	f           *os.File
	reader      *bufio.Reader
	Data        *VCDData
	idToIndex   map[string]int
	currentTime uint64
}

// NewVCDStreamParser opens a VCD file, parses the header, and stops at the first transition.
func NewVCDStreamParser(filename string) (*VCDStreamParser, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("opening VCD file: %w", err)
	}

	p := &VCDStreamParser{
		f:         f,
		reader:    bufio.NewReaderSize(f, 1024*1024),
		Data:      &VCDData{},
		idToIndex: make(map[string]int),
	}

	err = p.parseHeader()
	if err != nil {
		f.Close()
		return nil, err
	}

	return p, nil
}

func (p *VCDStreamParser) Close() error {
	return p.f.Close()
}

func (p *VCDStreamParser) parseHeader() error {
	var scopeStack []string
	inTimescale := false
	var timescaleBuf strings.Builder

	for {
		// Peek or ReadString to avoid bufio.Scanner token too long
		lineStr, err := p.reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		line := strings.TrimSpace(lineStr)
		if line == "" {
			continue
		}

		if inTimescale {
			if strings.Contains(line, "$end") {
				before, _, _ := strings.Cut(line, "$end")
				timescaleBuf.WriteString(" ")
				timescaleBuf.WriteString(strings.TrimSpace(before))
				p.Data.Timescale = strings.TrimSpace(timescaleBuf.String())
				inTimescale = false
			} else {
				timescaleBuf.WriteString(" ")
				timescaleBuf.WriteString(line)
			}
			continue
		}

		if strings.HasPrefix(line, "$timescale") {
			rest := strings.TrimPrefix(line, "$timescale")
			if idx := strings.Index(rest, "$end"); idx >= 0 {
				p.Data.Timescale = strings.TrimSpace(rest[:idx])
			} else {
				inTimescale = true
				timescaleBuf.Reset()
				timescaleBuf.WriteString(strings.TrimSpace(rest))
			}
			continue
		}

		if strings.HasPrefix(line, "$scope") {
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
			p.parseVarLine(line, scopeStack)
			continue
		}

		if strings.HasPrefix(line, "$enddefinitions") {
			// Header is done.
			break
		}
	}
	return nil
}

func (p *VCDStreamParser) parseVarLine(line string, scopeStack []string) {
	fields := strings.Fields(line)
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

	var fullName string
	if len(scopeStack) > 0 {
		fullName = strings.Join(scopeStack, "/") + "/" + name
	} else {
		fullName = name
	}

	if _, exists := p.idToIndex[id]; !exists {
		p.idToIndex[id] = len(p.Data.Signals)
		p.Data.Signals = append(p.Data.Signals, VCDSignal{
			Name:  fullName,
			Width: width,
			ID:    id,
			Wave:  &SignalWave{},
		})
	}
}

// Stream emits VCDChunk messages over a channel asynchronously.
func (p *VCDStreamParser) Stream(ctx context.Context, chunkSize int) <-chan VCDChunk {
	ch := make(chan VCDChunk, 16)
	go func() {
		defer close(ch)
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			chunk, err := p.parseNextChunk(chunkSize)
			if chunk != nil {
				select {
				case ch <- *chunk:
				case <-ctx.Done():
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					ch <- VCDChunk{Err: err}
				}
				return
			}
		}
	}()
	return ch
}

func (p *VCDStreamParser) parseNextChunk(linesToRead int) (*VCDChunk, error) {
	chunk := &VCDChunk{
		Updates: make(map[int]*SignalWave),
	}
	linesRead := 0

	for linesRead < linesToRead {
		lineStr, err := p.reader.ReadString('\n')
		if err != nil {
			if len(chunk.Updates) > 0 || p.currentTime > p.Data.EndTime {
				chunk.EndTime = p.currentTime
				return chunk, err
			}
			return nil, err
		}

		line := strings.TrimSpace(lineStr)
		if line == "" {
			continue
		}
		linesRead++

		if line[0] == '#' {
			ts, err := strconv.ParseUint(line[1:], 10, 64)
			if err == nil {
				p.currentTime = ts
			}
			continue
		}

		if len(line) >= 2 && is1BitValue(line[0]) {
			val := strings.ToLower(string(line[0]))
			id := line[1:]
			p.addUpdate(chunk, id, val)
			continue
		}

		if (line[0] == 'b' || line[0] == 'B') && len(line) > 1 {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				binStr := parts[0][1:]
				id := parts[1]
				p.addUpdate(chunk, id, binStr)
			}
			continue
		}

		if (line[0] == 'r' || line[0] == 'R') && len(line) > 1 {
			parts := strings.Fields(line)
			if len(parts) == 2 {
				id := parts[1]
				realVal := parts[0][1:]
				p.addUpdate(chunk, id, realVal)
			}
			continue
		}
	}

	chunk.EndTime = p.currentTime
	return chunk, nil
}

func (p *VCDStreamParser) addUpdate(chunk *VCDChunk, id, val string) {
	idx, ok := p.idToIndex[id]
	if !ok {
		return
	}
	wave, exists := chunk.Updates[idx]
	if !exists {
		wave = &SignalWave{}
		chunk.Updates[idx] = wave
	}
	wave.Append(p.currentTime, val)
}

func is1BitValue(ch byte) bool {
	return ch == '0' || ch == '1' ||
		ch == 'x' || ch == 'X' ||
		ch == 'z' || ch == 'Z'
}
