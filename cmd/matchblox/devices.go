package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"text/tabwriter"

	"github.com/blox-eng/matchblox/internal/setup"
)

const devicesUsage = "usage: matchblox devices [rm <name|fingerprint>]"

// devicesCmd lists the keys that open this host, phones first, or removes one.
func devicesCmd(args []string, home string, out io.Writer) error {
	switch {
	case len(args) == 0:
		return listDevices(home, out)
	case len(args) == 2 && args[0] == "rm":
		backup, err := setup.RemoveDevice(home, args[1])
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Removed %s. Backup: %s\n", args[1], backup)
		return err
	}
	return errors.New(devicesUsage)
}

func listDevices(home string, out io.Writer) error {
	devices, err := setup.Devices(home)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		_, err = fmt.Fprintln(out, "No phone or laptop key opens this host. Run matchblox pair to add a phone.")
		return err
	}
	slices.SortStableFunc(devices, func(a, b setup.Device) int {
		switch {
		case a.Kind == b.Kind:
			return 0
		case a.Kind == "phone":
			return -1
		}
		return 1
	})
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, d := range devices {
		fmt.Fprintf(w, "%s\t%s\t%s\n", d.Kind, d.Name, d.Fingerprint)
	}
	return w.Flush()
}
