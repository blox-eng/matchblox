package main

import (
	"io"

	selfupdate "github.com/blox-eng/matchblox/internal/update"
)

func updateCmd(out io.Writer) error {
	u, err := selfupdate.New(version, out)
	if err != nil {
		return err
	}
	return u.Update()
}
