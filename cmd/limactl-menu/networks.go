// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package main

// newNetworksView returns the view for managing the networks (`limactl network`)
// It is a placeholder until the network view is implemented.
func newNetworksView(_ *State) view {
	return &placeholderView{
		name:    "Networks",
		message: "Network management is coming soon. For now, run `limactl network` in a terminal.",
	}
}
