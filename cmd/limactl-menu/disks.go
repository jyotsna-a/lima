// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package main

// newDisksView returns the view for managing the disks (`limactl disk`).
// It is a placeholder until the disk view is implemented.
func newDisksView(_ *State) view {
	return &placeholderView{
		name:    "Disks",
		message: "Disk management is coming soon. For now, run `limactl disk` in a terminal.",
	}
}
