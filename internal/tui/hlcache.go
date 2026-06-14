package tui

import (
	"hash/fnv"
	"sync"
)

// CachedLine holds a single line's raw and highlighted version.
type CachedLine struct {
	rawHash     uint64 // FNV-64a hash of raw content
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

// ReplaceAll replaces the entire cache with new highlighted lines.
func (c *HighlightCache) ReplaceAll(lines []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure capacity
	if len(lines) > len(c.lines) {
		newLines := make([]CachedLine, len(lines))
		copy(newLines, c.lines)
		c.lines = newLines

		newDirty := make([]bool, len(lines))
		copy(newDirty, c.dirty)
		c.dirty = newDirty
	}

	for i, hl := range lines {
		// Just store the highlighted result and clear dirty flag.
		// We could compute hash of raw text here but the background
		// worker operates on the whole file at a specific version.
		c.lines[i].highlighted = hl
		c.dirty[i] = false
	}
}

// Get returns the highlighted version of a line.
// If dirty or missing, it falls back to raw line.
func (c *HighlightCache) Get(lineNum int, raw string) string {
	h := fnvHash(raw)

	c.mu.RLock()
	if lineNum < len(c.lines) && !c.dirty[lineNum] && c.lines[lineNum].rawHash == h {
		cached := c.lines[lineNum].highlighted
		c.mu.RUnlock()
		return cached
	}
	c.mu.RUnlock()

	// If cache misses or is dirty, return raw text. The background worker
	// will eventually update the cache with the highlighted version.
	return raw
}

func fnvHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}
