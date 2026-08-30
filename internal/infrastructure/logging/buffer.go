package logging

import (
	"io"
	"strings"
	"sync"
	"time"
)

// Entry is one bounded runtime log record exposed by the management API.
type Entry struct {
	ID      uint64    `json:"id"`
	Time    time.Time `json:"time"`
	Service string    `json:"service"`
	Stream  string    `json:"stream,omitempty"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
}

// Query selects records from a Buffer. ID is an exclusive lower bound.
type Query struct {
	Service string
	Level   string
	Query   string
	Since   uint64
	Limit   int
}

// Buffer keeps recent records in memory to avoid unbounded writes on router flash.
type Buffer struct {
	mu      sync.RWMutex
	max     int
	nextID  uint64
	entries []Entry
}

func NewBuffer(max int) *Buffer {
	if max <= 0 {
		max = 1000
	}
	return &Buffer{max: max, entries: make([]Entry, 0, max)}
}

func (b *Buffer) Add(service, stream, level, message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	if level == "" {
		level = DetectLevel(message)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	b.entries = append(b.entries, Entry{ID: b.nextID, Time: time.Now().UTC(), Service: service, Stream: stream, Level: level, Message: message})
	if len(b.entries) > b.max {
		b.entries = b.entries[len(b.entries)-b.max:]
	}
}

func (b *Buffer) Entries(query Query) []Entry {
	if query.Limit <= 0 || query.Limit > b.max {
		query.Limit = b.max
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]Entry, 0, query.Limit)
	needle := strings.ToLower(strings.TrimSpace(query.Query))
	for index := len(b.entries) - 1; index >= 0 && len(result) < query.Limit; index-- {
		entry := b.entries[index]
		if entry.ID <= query.Since || (query.Service != "" && entry.Service != query.Service) || (query.Level != "" && entry.Level != query.Level) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(entry.Message), needle) {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func (b *Buffer) LatestID() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.nextID
}

// Writer turns arbitrary process/logger writes into complete line records.
func (b *Buffer) Writer(service, stream string) io.Writer {
	return &lineWriter{buffer: b, service: service, stream: stream}
}

func DetectLevel(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "panic"), strings.Contains(lower, "fatal"), strings.Contains(lower, "error"), strings.Contains(lower, "failed"), strings.Contains(lower, "failure"), strings.Contains(lower, "unavailable"), strings.Contains(lower, "refused"), strings.Contains(lower, "timeout"), strings.Contains(lower, "unreachable"), strings.Contains(lower, "进程退出"), strings.Contains(lower, "exit status"), strings.Contains(lower, "signal:"):
		return "error"
	case strings.Contains(lower, "warn"):
		return "warn"
	case strings.Contains(lower, "debug"):
		return "debug"
	default:
		return "info"
	}
}

type lineWriter struct {
	mu      sync.Mutex
	buffer  *Buffer
	service string
	stream  string
	pending string
}

func (w *lineWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending += string(data)
	for {
		index := strings.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		w.buffer.Add(w.service, w.stream, "", strings.TrimSuffix(w.pending[:index], "\r"))
		w.pending = w.pending[index+1:]
	}
	return len(data), nil
}
