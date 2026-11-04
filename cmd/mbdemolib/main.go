// mbdemolib is the demo as a C library, for the app's terminal view.
package main

import "C"

import (
	"io"
	"sync"
	"unsafe"

	"github.com/blox-eng/matchblox/internal/demo"
)

// run is the demo that is on, with the app's ends of its terminal.
type run struct {
	demo  *demo.Demo
	in    *io.PipeWriter
	out   *io.PipeReader
	ended chan struct{}
}

var (
	mu      sync.Mutex
	current *run
	lastErr string
)

func on() *run {
	mu.Lock()
	defer mu.Unlock()
	return current
}

// MBDemoStart returns 0, or 1 with the reason in MBDemoError.
//
//export MBDemoStart
func MBDemoStart(tree *C.char, cols, rows C.int) C.int {
	return C.int(start(C.GoString(tree), int(cols), int(rows)))
}

// MBDemoError copies why the last run failed, at its start or after it, with
// a NUL at the end, and returns its length, cut to fit. Empty: it did not.
//
//export MBDemoError
func MBDemoError(buf *C.char, n C.int) C.int {
	if buf == nil || n <= 0 {
		return 0
	}
	dst := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(n))
	copied := copy(dst[:n-1], errorText())
	dst[copied] = 0
	return C.int(copied)
}

//export MBDemoWrite
func MBDemoWrite(p *C.char, n C.int) {
	if p != nil && n > 0 {
		write(C.GoBytes(unsafe.Pointer(p), n))
	}
}

// MBDemoRead blocks for what the console draws, and returns -1 at the end.
// At -1, MBDemoError says why, if the console died.
//
//export MBDemoRead
func MBDemoRead(p *C.char, n C.int) C.int {
	if p == nil || n <= 0 {
		return -1
	}
	return C.int(read(unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n))))
}

//export MBDemoResize
func MBDemoResize(cols, rows C.int) {
	if r := on(); r != nil {
		r.demo.Resize(int(cols), int(rows))
	}
}

// MBDemoStop ends the demo and removes its temp copy.
//
//export MBDemoStop
func MBDemoStop() { stop() }

//export MBDemoDone
func MBDemoDone() C.int {
	if r := on(); r != nil && r.demo.Done() {
		return 1
	}
	return 0
}

func start(tree string, cols, rows int) int {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	d, err := demo.Start(demo.Options{Tree: tree, In: inR, Out: outW, Cols: cols, Rows: rows})
	if err != nil {
		setError(err)
		return 1
	}
	if old := on(); old != nil {
		<-old.ended // the demo has ended, as this one started: its last word comes first
	}
	r := &run{demo: d, in: inW, out: outR, ended: make(chan struct{})}
	mu.Lock()
	current, lastErr = r, ""
	mu.Unlock()
	go func() {
		defer close(r.ended)
		// The reason comes before the end of the reads, which is when the app
		// asks for it. Closing the pipes ends the console's input reader and
		// any write, however the run ended.
		if err := d.Wait(); err != nil {
			setError(err)
		}
		_ = inW.Close()
		_ = outW.Close()
	}()
	return 0
}

func setError(err error) {
	mu.Lock()
	defer mu.Unlock()
	lastErr = err.Error()
}

func errorText() string {
	mu.Lock()
	defer mu.Unlock()
	return lastErr
}

func write(b []byte) {
	if r := on(); r != nil {
		_, _ = r.in.Write(b)
	}
}

func read(b []byte) int {
	r := on()
	if r == nil {
		return -1
	}
	got, err := r.out.Read(b)
	if err != nil {
		return -1
	}
	return got
}

// stop closes the pipes first: a read or a write that waits returns, so it
// cannot hold the stop.
func stop() {
	mu.Lock()
	r := current
	current = nil
	mu.Unlock()
	if r == nil {
		return
	}
	_ = r.out.Close()
	_ = r.in.Close()
	r.demo.Stop()
	<-r.ended
}

func main() {}
