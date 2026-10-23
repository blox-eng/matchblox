package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/blox-eng/matchblox/internal/config"
	"github.com/blox-eng/matchblox/internal/push"
	"github.com/blox-eng/matchblox/internal/setup"
)

// pushView is what the list knows of the sender: Off, why nothing leaves
// this host ("" when it sends), and Status, the running service's push line.
type pushView struct{ Off, Status string }

const devicesUsage = "usage: matchblox devices [rm <name|fingerprint>]"

// devicesCmd lists the keys that open this host, phones first, each phone
// with whether it buzzes and carries the island, or removes one.
func devicesCmd(args []string, home string, pv pushView, out io.Writer) error {
	switch {
	case len(args) == 0:
		return listDevices(home, pv, out)
	case len(args) >= 2 && args[0] == "rm":
		name := strings.Join(args[1:], " ") // rm iPad 2 needs no quotes
		backup, err := setup.RemoveDevice(home, name)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(out, "Removed %s. Backup: %s\n", name, backup)
		return err
	}
	return errors.New(devicesUsage)
}

func listDevices(home string, pv pushView, out io.Writer) error {
	devices, err := setup.Devices(home)
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		_, err = fmt.Fprintln(out, "No phone or laptop key opens this host. Run matchblox pair to add a phone.")
		return err
	}
	slices.SortStableFunc(devices, func(a, b setup.Device) int {
		if a.Kind == b.Kind {
			return 0
		}
		if a.Kind == "phone" {
			return -1
		}
		return 1
	})
	regs, err := setup.Registrations(home)
	if err != nil {
		return err
	}
	byID := map[string]push.Registration{}
	for _, r := range regs {
		byID[r.Device] = r
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for _, d := range devices {
		if d.Kind != "phone" {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", d.Kind, d.Name, d.Fingerprint)
			continue
		}
		// A phone with no registration is pushed nothing, and a registration
		// alone is not a buzz: a host that sends nothing says why.
		r, ok := byID[d.ID]
		buzz, island := "buzz off", "island no"
		if ok && d.ID != "" && r.Buzz && r.DeviceToken != "" {
			buzz = "buzz on"
			if pv.Off != "" {
				buzz = "buzz off (" + pv.Off + ")"
			}
		}
		if ok && d.ID != "" && r.Island != nil {
			island = "island yes"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", d.Kind, d.Name, d.Fingerprint, buzz, island)
	}
	if err := w.Flush(); err != nil || pv.Status == "" {
		return err
	}
	_, err = fmt.Fprintln(out, pv.Status)
	return err
}

// livePushView is the sender as this host has it: [push] off, a host key
// the phone cannot pin, and the push line of the running service.
func livePushView(cfg config.Push, hostKeyPath, socket string) pushView {
	var pv pushView
	if cfg.Off {
		pv.Off = "[push] off"
	} else if _, err := setup.HostKeyFingerprint(hostKeyPath); err != nil {
		pv.Off = "no host key"
	}
	if doc, ok := fromService(socket, staleAfter); ok {
		pv.Status = doc.Push
	}
	return pv
}
