package reconciliation

import (
	"context"
	"strings"
	"time"
)

// Maintainer-owned work limits, not OCI capacity guarantees. Count raw
// occurrences and all returned cursor bytes, including duplicates and empty pages.
const (
	enumerationPages = 100
	enumerationItems = 10000
	enumerationBytes = 4 << 20
)

type enumeration struct {
	ctx     context.Context
	wait    func(context.Context, time.Duration) error
	started bool
	items   int
	bytes   int
}

func (e *enumeration) before(source Source) error {
	if e.ctx.Err() != nil {
		return failure(e.ctx, source, Canceled)
	}
	if e.started {
		if err := e.wait(e.ctx, 100*time.Millisecond); err != nil {
			return failure(e.ctx, source, ReadFailed)
		}
	}
	if e.ctx.Err() != nil {
		return failure(e.ctx, source, Canceled)
	}
	e.started = true
	return nil
}

func (e *enumeration) accept(source Source, seen map[string]bool, entries []string, next string, pageSize, textSize int) error {
	if len(entries) > pageSize || len(next) > textSize {
		return failure(e.ctx, source, InvalidPage)
	}
	if len(entries) > enumerationItems-e.items {
		return failure(e.ctx, source, ItemLimit)
	}
	for _, entry := range entries {
		if entry == "" || len(entry) > textSize {
			return failure(e.ctx, source, InvalidPage)
		}
		if len(entry) > enumerationBytes-e.bytes {
			return failure(e.ctx, source, ByteLimit)
		}
		e.bytes += len(entry)
	}
	if len(next) > enumerationBytes-e.bytes {
		return failure(e.ctx, source, ByteLimit)
	}
	e.bytes += len(next)
	e.items += len(entries)
	if next != "" {
		if seen[next] {
			return failure(e.ctx, source, CursorCycle)
		}
		seen[strings.Clone(next)] = true
	}
	if e.ctx.Err() != nil {
		return failure(e.ctx, source, Canceled)
	}
	return nil
}

func appendIdentifiers(destination, entries []string) []string {
	for _, entry := range entries {
		// Avoid retaining a large adapter buffer through a short substring.
		destination = append(destination, strings.Clone(entry))
	}
	return destination
}
