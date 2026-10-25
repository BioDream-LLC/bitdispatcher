package api

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// LogRing is a slog handler that passes every record on and keeps the most recent warnings and errors in
// memory, for the support bundle. Nothing is written anywhere new.
type LogRing struct {
	next  slog.Handler
	attrs []slog.Attr
	group string
	r     *ring
}

type ring struct {
	mu    sync.Mutex
	lines []string
	at    int
	full  bool
}

// NewLogRing keeps the last n warnings and errors logged through it.
func NewLogRing(next slog.Handler, n int) *LogRing {
	return &LogRing{next: next, r: &ring{lines: make([]string, n)}}
}

func (l *LogRing) Enabled(ctx context.Context, lv slog.Level) bool { return l.next.Enabled(ctx, lv) }

func (l *LogRing) Handle(ctx context.Context, rec slog.Record) error {
	if rec.Level >= slog.LevelWarn {
		var b strings.Builder
		b.WriteString(rec.Time.UTC().Format(time.RFC3339))
		b.WriteString(" ")
		b.WriteString(rec.Level.String())
		b.WriteString(" ")
		b.WriteString(rec.Message)
		add := func(a slog.Attr) {
			b.WriteString(" ")
			if l.group != "" {
				b.WriteString(l.group + ".")
			}
			b.WriteString(a.Key + "=" + a.Value.String())
		}
		for _, a := range l.attrs {
			add(a)
		}
		rec.Attrs(func(a slog.Attr) bool { add(a); return true })
		line := b.String()
		if len(line) > 2000 {
			line = line[:2000] + "…"
		}
		l.r.mu.Lock()
		l.r.lines[l.r.at] = line
		l.r.at = (l.r.at + 1) % len(l.r.lines)
		l.r.full = l.r.full || l.r.at == 0
		l.r.mu.Unlock()
	}
	return l.next.Handle(ctx, rec)
}

func (l *LogRing) WithAttrs(as []slog.Attr) slog.Handler {
	return &LogRing{next: l.next.WithAttrs(as), attrs: append(append([]slog.Attr(nil), l.attrs...), as...), group: l.group, r: l.r}
}

func (l *LogRing) WithGroup(name string) slog.Handler {
	g := name
	if l.group != "" {
		g = l.group + "." + name
	}
	return &LogRing{next: l.next.WithGroup(name), attrs: l.attrs, group: g, r: l.r}
}

// Lines are the kept warnings and errors, oldest first.
func (l *LogRing) Lines() []string {
	l.r.mu.Lock()
	defer l.r.mu.Unlock()
	var out []string
	if l.r.full {
		out = append(out, l.r.lines[l.r.at:]...)
	}
	out = append(out, l.r.lines[:l.r.at]...)
	return out
}
