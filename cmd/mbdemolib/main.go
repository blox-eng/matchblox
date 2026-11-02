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
	demo *demo.Demo
	in   *io.PipeWriter
	out  *io.PipeReader
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
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	d, err := demo.Start(demo.Options{Tree: C.GoString(tree), In: inR, Out: outW, Cols: int(cols), Rows: int(rows)})
	mu.Lock()
	defer mu.Unlock()
	if err != nil {
		lastErr = err.Error()
		return 1
	}
	current, lastErr = &run{demo: d, in: inW, out: outR}, ""
	go func() {
		_ = d.Wait() // a console that quits ends the reads, after what it drew
		_ = outW.Close()
	}()
	return 0
}

// MBDemoError copies the last start's error, NUL-terminated, and returns its
// length, cut to fit.
//
//export MBDemoError
func MBDemoError(buf *C.char, n C.int) C.int {
	if buf == nil || n <= 0 {
		return 0
	}
	mu.Lock()
	defer mu.Unlock()
	dst := unsafe.Slice((*byte)(unsafe.Pointer(buf)), int(n))
	copied := copy(dst[:n-1], lastErr)
	dst[copied] = 0
	return C.int(copied)
}

//export MBDemoWrite
func MBDemoWrite(p *C.char, n C.int) {
	if r := on(); r != nil && n > 0 {
		_, _ = r.in.Write(C.GoBytes(unsafe.Pointer(p), n))
	}
}

// MBDemoRead blocks for what the console draws, and returns -1 at the end.
//
//export MBDemoRead
func MBDemoRead(p *C.char, n C.int) C.int {
	r := on()
	if r == nil || n <= 0 {
		return -1
	}
	got, err := r.out.Read(unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n)))
	if err != nil {
		return -1
	}
	return C.int(got)
}

//export MBDemoResize
func MBDemoResize(cols, rows C.int) {
	if r := on(); r != nil {
		r.demo.Resize(int(cols), int(rows))
	}
}

// MBDemoStop ends the demo and removes its temp copy. A read or a write that
// waits returns first, so it cannot hold the stop.
//
//export MBDemoStop
func MBDemoStop() {
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
}

//export MBDemoDone
func MBDemoDone() C.int {
	if r := on(); r != nil && r.demo.Done() {
		return 1
	}
	return 0
}

func main() {}
