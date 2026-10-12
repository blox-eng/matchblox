package ships

import (
	"bufio"
	"errors"
	"io"
	"os"
	"time"
)

// maxLine bounds one transcript line held in memory. A longer line is a
// tool's output, never a command we read, and is skipped.
const maxLine = 8 << 20

// transcript is how far one transcript has been read, persisted so a
// restart reads only the bytes written since.
type transcript struct {
	Offset  int64              `json:"offset"`
	Session SessionRef         `json:"session"`
	Seen    time.Time          `json:"seen"` // the last scan it belonged to a live session
	Pending map[string]pending `json:"pending,omitempty"`
}

// read reads the complete lines written since t.Offset and returns their
// evidence. The file streams through a bounded buffer: a transcript seen
// for the first time is read whole once, then only its new bytes. A
// shorter file than the offset was rewritten and is read again.
func read(path string, t *transcript) ([]found, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() < t.Offset {
		t.Offset, t.Pending = 0, nil
	}
	if st.Size() == t.Offset {
		return nil, nil
	}
	if _, err := f.Seek(t.Offset, io.SeekStart); err != nil {
		return nil, err
	}
	if t.Pending == nil {
		t.Pending = map[string]pending{}
	}
	r := bufio.NewReaderSize(io.LimitReader(f, st.Size()-t.Offset), 1<<20)
	var (
		out  []found
		line []byte
		long bool
		pos  = t.Offset
	)
	for {
		chunk, err := r.ReadSlice('\n')
		pos += int64(len(chunk))
		if errors.Is(err, bufio.ErrBufferFull) {
			if long = long || len(line)+len(chunk) > maxLine; !long {
				line = append(line, chunk...)
			}
			continue
		}
		if err != nil {
			break // a last line without its newline is read when it has one
		}
		whole := chunk
		if len(line) > 0 {
			whole = append(line, chunk...)
		}
		if !long && mayHold(whole, t.Pending) {
			out = append(out, parseLine(whole, t.Pending)...)
		}
		line, long = line[:0], false
		t.Offset = pos
	}
	if len(t.Pending) == 0 {
		t.Pending = nil
	}
	return out, nil
}
