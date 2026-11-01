// mbdemosvc asks one question: does the real service link for iOS, and how big
// is it next to the console? SPIKE.
package main

import "C"

import (
	"github.com/blox-eng/matchblox/internal/app"
	"github.com/blox-eng/matchblox/internal/service"
)

var (
	_ app.Options
	_ service.Service
)

//export MBServiceProbe
func MBServiceProbe() C.int { return 1 }

func main() {}
