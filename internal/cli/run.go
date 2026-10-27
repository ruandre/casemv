package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/ruandre/casemv/internal/casing"
	"github.com/ruandre/casemv/internal/rename"
)

// output buffers writes, since a syscall per line costs more than converting
// the line, and latches the first write error so a closed pipe is reported
// once, after the work is done, instead of at every line.
type output struct {
	w   *bufio.Writer
	err error
}

func newOutput(w io.Writer) *output { return &output{w: bufio.NewWriter(w)} }

func (o *output) printf(format string, args ...any) {
	if o.err != nil {
		return
	}
	_, o.err = fmt.Fprintf(o.w, format, args...)
}

// flush writes buffered output now.
func (o *output) flush() {
	if o.err != nil {
		return
	}
	o.err = o.w.Flush()
}

func (o *output) failed() bool { return o.err != nil }

func (o *output) result() error {
	o.flush()
	if o.err == nil {
		return nil
	}
	return fmt.Errorf("write output: %w", o.err)
}

func run(ctx context.Context, w io.Writer, in io.Reader, s settings) error {
	out := newOutput(w)
	if s.text {
		return convertLines(ctx, out, in, s)
	}
	return renameTree(ctx, out, s)
}

func renameTree(ctx context.Context, out *output, s settings) error {
	start := time.Now()
	plan, err := rename.Build(ctx, s.scanOptions(), s.namer())
	if err != nil {
		return err
	}

	if s.dryRun {
		if !s.quiet {
			for _, r := range plan.Renames {
				out.printf("%s -> %s\n", displayPath(r.From), displayPath(filepath.Base(r.To)))
			}
			out.printf("dry run: %s would be renamed\n", plural(len(plan.Renames)))
		}
		return out.result()
	}

	// Written as each rename lands, so a failure or kill part-way through
	// never leaves a listing claiming work that did not happen, nor hides work
	// that did.
	var report func(rename.Rename)
	if !s.quiet {
		report = func(r rename.Rename) {
			out.printf("%s -> %s\n", displayPath(r.From), displayPath(filepath.Base(r.To)))
			out.flush()
		}
	}

	done, err := plan.Apply(ctx, report)
	if err != nil {
		if errors.Is(err, context.Canceled) {
			return err
		}
		return fmt.Errorf("%w; %s renamed before the failure", err, plural(done))
	}
	if !s.quiet {
		out.printf("%s renamed in %s\n", plural(done), time.Since(start).Round(time.Microsecond))
	}
	return out.result()
}

func convertLines(ctx context.Context, out *output, in io.Reader, s settings) error {
	reader := bufio.NewReader(in)
	for {
		// Flush before a read that may block, so streamed input sees each line.
		if reader.Buffered() == 0 {
			out.flush()
		}
		line, isLast, err := readLine(ctx, reader)
		if err != nil {
			out.flush()
			return err
		}
		if isLast && line == "" {
			break
		}
		out.printf("%s\n", convertLine(s.convention, line))
		if out.failed() || isLast {
			break
		}
	}
	return out.result()
}

// readLine returns the next raw line and whether input ended with it.
func readLine(ctx context.Context, reader *bufio.Reader) (line string, isLast bool, err error) {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", false, ctxErr
	}
	line, readErr := reader.ReadString('\n')
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", false, ctxErr
	}
	isLast = errors.Is(readErr, io.EOF)
	if readErr != nil && !isLast {
		return "", false, fmt.Errorf("read input: %w", readErr)
	}
	return line, isLast, nil
}

// convertLine formats line without its terminator; blank lines stay blank.
func convertLine(convention casing.Convention, line string) string {
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return ""
	}
	return convention.FormatText(line)
}

func plural(n int) string {
	if n == 1 {
		return "1 entry"
	}
	return fmt.Sprintf("%d entries", n)
}
