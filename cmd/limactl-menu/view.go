// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"

	"github.com/gdamore/tcell/v2"
)

// view is one screen of the menu, such as Instances, Disks, or Networks
// To add a view, implement it in a new file and add it to State.views in runMenu
// A row argument is -1 when no row is selected.
type view interface {
	title() string
	// load must not write to the terminal; the returned error is shown in the view
	load(ctx context.Context) error
	header() string
	rows() []string
	// rowKey identifies a row (e.g., by name) to keep it selected across refreshes
	rowKey(i int) string
	emptyText() string
	// selectRow is called when Enter is pressed on a row
	selectRow(i int)
	// actions returns the items of the Edit menu
	actions(row int) []action
	// handleKey returns nil if it handled the key
	handleKey(e *tcell.EventKey, row int) *tcell.EventKey
	footer() string
	help() string
}

// placeholderView only shows a message, for the views that are not implemented yet.
type placeholderView struct {
	name    string
	message string
}

func (v *placeholderView) title() string                                    { return v.name }
func (*placeholderView) load(context.Context) error                         { return nil }
func (*placeholderView) header() string                                     { return "" }
func (*placeholderView) rows() []string                                     { return nil }
func (*placeholderView) rowKey(int) string                                  { return "" }
func (v *placeholderView) emptyText() string                                { return v.message }
func (*placeholderView) selectRow(int)                                      {}
func (*placeholderView) actions(int) []action                               { return nil }
func (*placeholderView) handleKey(e *tcell.EventKey, _ int) *tcell.EventKey { return e }
func (*placeholderView) footer() string                                     { return "" }
func (*placeholderView) help() string                                       { return "" }
