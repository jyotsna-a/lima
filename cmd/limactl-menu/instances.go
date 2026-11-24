// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/docker/go-units"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/lima-vm/lima/v2/pkg/limatype"
)

// instancesView is the view for managing the instances.
type instancesView struct {
	s     *State
	insts []*limatype.Instance
}

func newInstancesView(s *State) *instancesView {
	return &instancesView{s: s}
}

func (*instancesView) title() string { return "Instances" }

func (v *instancesView) load(ctx context.Context) error {
	insts, err := listInstances(ctx, v.s.limactl)
	if err != nil {
		return err
	}
	v.insts = insts
	return nil
}

func (*instancesView) header() string { return fmtHeader() }

func (v *instancesView) rows() []string {
	rows := make([]string, len(v.insts))
	for i, in := range v.insts {
		rows[i] = fmtInst(in)
	}
	return rows
}

func (v *instancesView) rowKey(i int) string { return v.insts[i].Name }

func (*instancesView) emptyText() string {
	return "No Lima instances found. Press [::b]F7[::-] or [::b]c[::-] to create an instance."
}

func (v *instancesView) selectRow(i int) { v.showActions(v.insts[i]) }

func (*instancesView) footer() string {
	return "[::r]F2[::-] Start/Stop  [::r]F3[::-] Info  [::r]F7[::-] Create  [::r]F8[::-] Delete"
}

func (*instancesView) help() string {
	return `  [green]F2[-] / [green]S[-]       Start / Stop toggle
  [green]F3[-] / [green]i[-]       Show instance info
  [green]F7[-] / [green]c[-]       Create a new instance
  [green]F8[-] / [green]d[-]       Delete instance
  [green]Enter[-]        Open actions menu
  [green]s[-]            Shell into instance
  [green]r[-]            Restart instance`
}

func (v *instancesView) handleKey(e *tcell.EventKey, row int) *tcell.EventKey {
	s := v.s
	if e.Key() == tcell.KeyF7 || e.Rune() == 'c' || e.Rune() == 'C' {
		s.showCreateForm()
		return nil
	}

	if row < 0 {
		return e
	}
	in := v.insts[row]

	switch e.Key() {
	case tcell.KeyF2:
		v.toggleStartStop(in)
		return nil
	case tcell.KeyF3:
		v.showInfo(in)
		return nil
	case tcell.KeyF8:
		v.confirmDelete(in)
		return nil
	case tcell.KeyRune:
		switch e.Rune() {
		case 's':
			s.execCmd("shell", in.Name)
			return nil
		case 'i', 'I':
			v.showInfo(in)
			return nil
		case 'S':
			v.toggleStartStop(in)
			return nil
		case 'r', 'R':
			v.confirmRestart(in)
			return nil
		case 'd', 'D':
			v.confirmDelete(in)
			return nil
		}
	}
	return e
}

func (v *instancesView) toggleStartStop(in *limatype.Instance) {
	if in.Status == "Running" {
		v.s.confirm(fmt.Sprintf("Confirm stop '%s'?", in.Name), func() { v.s.execCmd("stop", in.Name) })
	} else {
		v.s.execCmd("start", in.Name)
	}
}

func (v *instancesView) confirmRestart(in *limatype.Instance) {
	v.s.confirm(fmt.Sprintf("Confirm restart '%s'?", in.Name), func() { v.s.execCmd("restart", in.Name) })
}

func (v *instancesView) confirmDelete(in *limatype.Instance) {
	v.s.confirm(fmt.Sprintf("DELETE instance '%s'?\nThis cannot be undone.", in.Name), func() { v.s.execCmd("delete", in.Name) })
}

// actions returns the items of the Edit menu: creating an instance, and the actions for the selected instance.
func (v *instancesView) actions(row int) []action {
	acts := []action{{"Create an instance...", v.s.showCreateForm}}
	if row < 0 {
		return acts
	}
	return append(acts, v.instanceActions(v.insts[row])...)
}

func (v *instancesView) showActions(in *limatype.Instance) {
	v.s.showActions(fmt.Sprintf(" Actions for %s ", in.Name), v.instanceActions(in))
}

// instanceActions returns the actions for the instance.
func (v *instancesView) instanceActions(in *limatype.Instance) []action {
	s := v.s
	return []action{
		{"Shell", func() { s.execCmd("shell", in.Name) }},
		{"Start / Stop Toggle", func() { v.toggleStartStop(in) }},
		{"Restart", func() { v.confirmRestart(in) }},
		{"Show Info", func() { v.showInfo(in) }},
		{"Show Ports", func() { v.showPorts(in) }},
		{"Protect", func() { s.execCmd("protect", in.Name) }},
		{"Unprotect", func() {
			s.confirm(fmt.Sprintf("Confirm unprotect '%s'?", in.Name), func() { s.execCmd("unprotect", in.Name) })
		}},
		{"Delete", func() { v.confirmDelete(in) }},
	}
}

func (v *instancesView) showInfo(in *limatype.Instance) {
	prot := "No"
	if in.Protected {
		prot = "Yes"
	}
	var b strings.Builder
	fmt.Fprintf(&b, `Status:       %s
Hostname:     %s
VM Type:      %s
Architecture: %s
CPUs:         %d
Memory:       %s
Disk:         %s
SSH Port:     %d
SSH Address:  %s
Protected:    %s
Lima Version: %s`, statusTag(in.Status), in.Hostname, in.VMType, in.Arch, in.CPUs, units.BytesSize(float64(in.Memory)), units.BytesSize(float64(in.Disk)), in.SSHLocalPort, in.SSHAddress, prot, in.LimaVersion)

	if len(in.Errors) > 0 {
		b.WriteString("\n\nErrors:\n")
		for _, e := range in.Errors {
			fmt.Fprintf(&b, "  - %s\n", e.Error())
		}
	}
	if in.Message != "" {
		fmt.Fprintf(&b, "\n\nMessage:\n  %s", in.Message)
	}

	v.s.showTextView(" Info ", b.String())
}

func (v *instancesView) showPorts(in *limatype.Instance) {
	s := v.s
	out, err := exec.CommandContext(s.ctx, s.limactl, "list", "--format", `{{range .PortForwards}}{{.GuestIP}}:{{.GuestPort}} -> {{.HostIP}}:{{.HostPort}}{{"\n"}}{{end}}`, in.Name).CombinedOutput()
	if err != nil {
		s.showTextView(" Error ", "Failed to get ports: "+err.Error())
		return
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		p = "No port forwards configured"
	}
	s.showTextView(" Port Forwards ", "Port Forwards for "+in.Name+"\n\n"+p)
}

const (
	// rowFormat lays out the columns of a menu row: NAME, STATUS, ARCH, CPUS,
	// MEMORY and VMTYPE. fmtHeader uses the same format, so that the column
	// titles stay aligned with the values.
	rowFormat = "%-16s  %-10s  %-8s  %-4s  %-8s  %s"

	// statusIconWidth is the display width of the status icon plus the two
	// spaces after it. The header is indented by the same amount.
	statusIconWidth = 3
)

func fmtHeader() string {
	return strings.Repeat(" ", statusIconWidth) +
		fmt.Sprintf(rowFormat, "NAME", "STATUS", "ARCH", "CPUS", "MEMORY", "VMTYPE")
}

func fmtInst(in *limatype.Instance) string {
	icon := map[string]string{
		"Running":    "[green]●[-]",
		"Stopped":    "[red]●[-]",
		"Broken":     "[yellow]●[-]",
		"Installing": "[blue]◐[-]",
	}[in.Status]
	if icon == "" {
		icon = "[gray]○[-]"
	}

	name := in.Name
	if len(name) > 16 {
		name = name[:13] + "..."
	}

	cpus := strconv.Itoa(in.CPUs)
	memory := units.BytesSize(float64(in.Memory))

	vmType := in.VMType
	if vmType == "" {
		vmType = "-"
	}

	prot := ""
	if in.Protected {
		prot = " " + tview.Escape("[PROTECTED]")
	}

	return fmt.Sprintf("%s  "+rowFormat+"%s", icon, name, in.Status, in.Arch, cpus, memory, vmType, prot)
}

func listInstances(ctx context.Context, limactl string) ([]*limatype.Instance, error) {
	cmd := exec.CommandContext(ctx, limactl, "list", "--json")
	// Buffer stderr rather than writing it to the terminal, so that warnings such as
	// "No instance found. Run `limactl create` ..." do not reappear when the menu
	// suspends the TUI to run another limactl command.
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	var res []*limatype.Instance
	dec := json.NewDecoder(stdout)
	for dec.More() {
		var in limatype.Instance
		if err := dec.Decode(&in); err != nil {
			return nil, err
		}
		res = append(res, &in)
	}
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("failed to run `limactl list --json`: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	slices.SortFunc(res, func(a, b *limatype.Instance) int {
		if (a.Status == "Running") != (b.Status == "Running") {
			if a.Status == "Running" {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return res, nil
}

func statusTag(st string) string {
	return map[string]string{
		"Running":    "[green]RUNNING[-]",
		"Stopped":    "[red]STOPPED[-]",
		"Broken":     "[yellow]BROKEN[-]",
		"Installing": "[blue]INSTALLING[-]",
	}[st]
}
