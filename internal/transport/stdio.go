package transport

import (
	"bufio"
	"errors"
	"io"
)

// Stdio is a Conn over a reader and a writer, such as the stdin and stdout
// of `matchblox serve --stdio`. Close closes both when they can be closed.
func Stdio(r io.Reader, w io.Writer) Conn {
	return &conn{w: w, r: bufio.NewReader(r), close: func() error {
		var errs []error
		for _, x := range []any{w, r} {
			if c, ok := x.(io.Closer); ok {
				errs = append(errs, c.Close())
			}
		}
		return errors.Join(errs...)
	}}
}
