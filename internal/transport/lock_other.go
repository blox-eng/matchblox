//go:build !unix

package transport

import "os"

// lock is a no-op where flock does not exist; Listen still refuses a
// socket that answers.
func lock(string) (*os.File, error) { return nil, nil }
