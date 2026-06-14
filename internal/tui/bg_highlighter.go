package tui

import (
	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	tea "github.com/charmbracelet/bubbletea"
)

type HighlightJob struct {
	BufferID string
	Version  int // monotonic version; stale results are discarded
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
	lexer := lexers.Get("systemverilog")
	if lexer == nil {
		lexer = lexers.Fallback
	}
	style := styles.Get("monokai")
	if style == nil {
		style = styles.Fallback
	}
	formatter := formatters.Get("terminal256")
	if formatter == nil {
		formatter = formatters.Fallback
	}

	for job := range bh.jobs {
		lines := HighlightWithLineFallback(job.Content, lexer, formatter, style)
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
