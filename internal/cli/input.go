package cli

import (
	"context"
	"io"
)

// cancellableReader returns from Read when ctx ends, even while the underlying
// read blocks: Go restarts a read(2) interrupted by a signal, so a read of idle
// stdin would otherwise hold a cancelled run until the next line arrives. The
// abandoned read finishes on its own goroutine; nothing reads after
// cancellation, so the reader must not be used once it returns ctx's error.
type cancellableReader struct {
	ctx   context.Context //nolint:containedctx // bounds every Read of one run
	in    io.Reader
	buf   []byte
	reads chan readResult
}

type readResult struct {
	n   int
	err error
}

func newCancellableReader(ctx context.Context, in io.Reader) *cancellableReader {
	// Capacity 1 lets an abandoned read deliver its result and exit.
	return &cancellableReader{ctx: ctx, in: in, reads: make(chan readResult, 1)}
}

func (r *cancellableReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if cap(r.buf) < len(p) {
		r.buf = make([]byte, len(p))
	}
	buf := r.buf[:len(p)]
	go func() {
		n, err := r.in.Read(buf)
		r.reads <- readResult{n: n, err: err}
	}()
	select {
	case result := <-r.reads:
		return copy(p, buf[:result.n]), result.err
	case <-r.ctx.Done():
		return 0, r.ctx.Err()
	}
}
