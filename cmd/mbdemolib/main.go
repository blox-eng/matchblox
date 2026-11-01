// mbdemolib is the demo as a C library for the app. SPIKE.
package main

/*
#include <stdint.h>
*/
import "C"

import (
	"io"
	"unsafe"

	tea "charm.land/bubbletea/v2"

	"github.com/blox-eng/matchblox/internal/demo"
)

var (
	inW  *io.PipeWriter
	outR *io.PipeReader
	prog *tea.Program
)

//export MBDemoStart
func MBDemoStart(cols, rows C.int) {
	inR, iw := io.Pipe()
	or, outW := io.Pipe()
	inW, outR = iw, or
	p, c := demo.Program(inR, outW, int(cols), int(rows))
	prog = p
	go func() {
		defer c.Close()
		_, _ = p.Run()
		_ = outW.Close()
	}()
}

//export MBDemoWrite
func MBDemoWrite(p *C.char, n C.int) {
	if inW != nil {
		_, _ = inW.Write(C.GoBytes(unsafe.Pointer(p), n))
	}
}

//export MBDemoRead
func MBDemoRead(p *C.char, n C.int) C.int {
	if outR == nil {
		return -1
	}
	got, err := outR.Read(unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n)))
	if err != nil {
		return -1
	}
	return C.int(got)
}

//export MBDemoResize
func MBDemoResize(cols, rows C.int) {
	if prog != nil {
		prog.Send(tea.WindowSizeMsg{Width: int(cols), Height: int(rows)})
	}
}

func main() {}
