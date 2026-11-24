// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"github.com/lima-vm/lima/v2/pkg/gendoc"
	"github.com/lima-vm/lima/v2/pkg/limactlutil"
	"github.com/lima-vm/lima/v2/pkg/version"
)

var (
	cPrim = tcell.NewRGBColor(0x32, 0xCD, 0x32)
	cSec  = tcell.NewRGBColor(0x4E, 0x9A, 0x06)
	cBord = tcell.NewRGBColor(0x55, 0x55, 0x55)
	cTxt  = tcell.ColorDefault
	cDim  = tcell.ColorDarkCyan

	// sActive is the style of the activated buttons and the selected dropdown options.
	sActive = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(cPrim)
	// sField is the style of the form fields, readable on both dark and light terminal themes.
	sField = tcell.StyleDefault.Foreground(tcell.ColorBlack).Background(tcell.ColorLightGray)
)

type State struct {
	limactl    string
	ctx        context.Context
	app        *tview.Application
	list       *tview.List
	header     *tview.TextView
	panel      *tview.Flex
	footer     *tview.TextView
	flex       *tview.Flex
	menuBar    *tview.TextView
	openedMenu int
	// status is the status line shown above the footer until a key is pressed.
	status *tview.TextView
	// createView is the root of the create form, restored after an error message.
	createView *tview.Flex

	// views are the views of the menu, in the order of the View menu
	views []view
	// cur is the index of the current view in views
	cur int
	// selected is the key of the selected row of each view, restored when switching back to the view
	selected []string
	// rowCount is the number of rows of the current view shown in the list
	rowCount int
	// loadErr is the error of the last load of the current view
	loadErr error
	// instances is the view of the instances, also used by the create form
	instances *instancesView
}

func main() {
	if err := newApp().Execute(); err != nil {
		logrus.Fatal(err)
	}
}

func newApp() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "limactl-menu",
		Short:         "TUI menu for Lima instances (EXPERIMENTAL)",
		Version:       strings.TrimPrefix(version.Version, "v"),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runMenu,
		// The hidden generate-doc command must not add a completion command
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		Annotations: map[string]string{
			// Show "limactl menu" rather than "limactl-menu" in the usage and in the docs
			cobra.CommandDisplayNameAnnotation: "limactl menu",
		},
	}
	cmd.AddCommand(newGenDocCommand())
	return cmd
}

func newGenDocCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "generate-doc DIR",
		Short:  "Generate documentation pages",
		Args:   cobra.ExactArgs(1),
		RunE:   genDocAction,
		Hidden: true,
	}
	cmd.Flags().String("content-mode", gendoc.ContentModeCLI, "Content to generate (cli: the cli-reference pages for docsy)")
	return cmd
}

func genDocAction(cmd *cobra.Command, args []string) error {
	contentMode, err := cmd.Flags().GetString("content-mode")
	if err != nil {
		return err
	}
	if contentMode != gendoc.ContentModeCLI {
		return fmt.Errorf("unknown content mode %q", contentMode)
	}
	return gendoc.Docsy(cmd.Root(), args[0])
}

func runMenu(cmd *cobra.Command, _ []string) error {
	limactl, err := limactlutil.Path()
	if err != nil {
		return err
	}
	s := &State{limactl: limactl, ctx: cmd.Context()}
	s.instances = newInstancesView(s)
	s.setViews(s.instances, newDisksView(s), newNetworksView(s))
	return s.run()
}

// setViews sets the views of the menu, in the order of the View menu.
func (s *State) setViews(views ...view) {
	s.views = views
	s.selected = make([]string, len(views))
	s.cur = 0
}

func (s *State) run() error {
	s.app = tview.NewApplication().EnableMouse(true)
	s.setupStyles()
	s.buildUI()
	s.loadView()
	s.app.SetRoot(s.flex, true).SetFocus(s.list)
	return s.app.Run()
}

func (s *State) setupStyles() {
	tview.Styles.PrimitiveBackgroundColor = tcell.ColorDefault
	tview.Styles.ContrastBackgroundColor = tcell.ColorDefault
	tview.Styles.PrimaryTextColor = cTxt
	tview.Styles.SecondaryTextColor = cDim
	tview.Styles.TertiaryTextColor = cDim
	tview.Styles.InverseTextColor = tcell.ColorBlack
	tview.Styles.ContrastSecondaryTextColor = cTxt
}

// buildUI builds the widgets of the main screen. s.app must be set.
func (s *State) buildUI() {
	s.list = tview.NewList().
		SetHighlightFullLine(true).
		SetMainTextColor(cTxt).
		SetSelectedStyle(tcell.StyleDefault.Reverse(true))
	s.list.SetInputCapture(s.handleListInput)

	s.header = tview.NewTextView().SetTextColor(cDim)

	s.panel = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(s.header, 1, 0, false).
		AddItem(s.list, 0, 1, true)
	s.panel.SetBorder(true).
		SetBorderColor(cBord).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(cPrim).
		SetBorderPadding(1, 1, 1, 1)

	s.footer = tview.NewTextView().SetDynamicColors(true)

	s.status = tview.NewTextView().
		SetDynamicColors(true).
		SetText(" Press [::b]F9[::-] to open the menu bar, or [::b]F1[::-] to show the usage")

	s.openedMenu = -1
	s.menuBar = s.newMenuBar()

	s.flex = tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(s.menuBar, 1, 0, false).
		AddItem(s.panel, 0, 1, true).
		AddItem(s.status, 1, 1, false).
		AddItem(s.footer, 1, 1, false)

	s.app.SetInputCapture(s.handleGlobalKey)
}

func (s *State) currentView() view {
	return s.views[s.cur]
}

// loadView loads the current view and shows its rows.
func (s *State) loadView() {
	s.loadErr = s.currentView().load(s.ctx)
	s.populateList()
}

func (s *State) populateList() {
	v := s.currentView()
	s.panel.SetTitle(" " + v.title() + " ")
	s.header.SetText(v.header())
	s.footer.SetText(fmtFooter(v))
	s.list.Clear()
	s.rowCount = 0
	switch rows := v.rows(); {
	case s.loadErr != nil:
		s.list.AddItem("[red]"+tview.Escape("Error: "+s.loadErr.Error())+"[-]", "", 0, nil)
	case len(rows) == 0:
		s.list.AddItem(v.emptyText(), "", 0, nil)
	default:
		s.rowCount = len(rows)
		for i, row := range rows {
			s.list.AddItem(row, "", 0, func() {
				v.selectRow(i)
			})
		}
	}
	s.list.SetCurrentItem(0)
}

// fmtFooter returns the footer: the keys of the view between the keys common to all the views.
func fmtFooter(v view) string {
	footer := " [::r]F1[::-] Help  "
	if vf := v.footer(); vf != "" {
		footer += vf + "  "
	}
	return footer + "[::r]F5[::-] Refresh  [::r]F9[::-] Menu  [::r]F10[::-] Quit"
}

// currentRow returns the index of the selected row of the current view, or -1 if no row is selected.
func (s *State) currentRow() int {
	idx := s.list.GetCurrentItem()
	if idx >= 0 && idx < s.rowCount {
		return idx
	}
	return -1
}

// currentKey returns the key of the selected row of the current view, or "" if no row is selected.
func (s *State) currentKey() string {
	if row := s.currentRow(); row >= 0 {
		return s.currentView().rowKey(row)
	}
	return ""
}

// selectKey selects the row with the given key in the current view
// The selection is not changed if there is no such row.
func (s *State) selectKey(key string) {
	if key == "" {
		return
	}
	v := s.currentView()
	for i := range s.rowCount {
		if v.rowKey(i) == key {
			s.list.SetCurrentItem(i)
			return
		}
	}
}

// switchView switches to the i-th view, restoring the row that was selected when leaving it.
func (s *State) switchView(i int) {
	if i < 0 || i >= len(s.views) || i == s.cur {
		return
	}
	s.selected[s.cur] = s.currentKey()
	s.cur = i
	s.loadView()
	s.selectKey(s.selected[i])
}

func (s *State) handleListInput(e *tcell.EventKey) *tcell.EventKey {
	if e.Key() != tcell.KeyRune {
		return e
	}

	switch e.Rune() {
	case 'j':
		return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	case 'k':
		return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	case 'q', 'Q':
		s.app.Stop()
		return nil
	default:
		return e
	}
}

func (s *State) handleGlobalKey(e *tcell.EventKey) *tcell.EventKey {
	if s.status != nil {
		s.flex.RemoveItem(s.status)
		s.status = nil
	}

	switch e.Key() {
	case tcell.KeyCtrlC, tcell.KeyF10:
		s.app.Stop()
		return nil
	case tcell.KeyCtrlN:
		return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
	case tcell.KeyCtrlP:
		return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
	case tcell.KeyF9:
		if s.openedMenu >= 0 {
			s.closeMenu()
			return nil
		}
	}

	// The shortcuts below are only for the list; other screens
	// (e.g., the input fields of the create form) receive the keys as is
	if s.app.GetFocus() != s.list {
		return e
	}

	if i := menuKey(s.menus(), e); i >= 0 {
		s.openMenu(i)
		return nil
	}

	switch {
	case e.Key() == tcell.KeyF9:
		s.openMenu(0)
		return nil
	case e.Key() == tcell.KeyF1, e.Rune() == 'h', e.Rune() == '?':
		s.showHelp()
		return nil
	case e.Key() == tcell.KeyF5:
		s.refreshList()
		return nil
	}

	return s.currentView().handleKey(e, s.currentRow())
}

func (s *State) showHelp() {
	help := `[green]Keybindings:[-]

  [green]F1[-] / [green]?[-] / [green]h[-]   Help (this screen)
  [green]F5[-]           Refresh the list
  [green]F9[-]           Open the menu bar (Left / Right: switch menus, Esc: close)
  [green]Alt[-]+letter   Open a menu, e.g., Alt+V for View
  [green]F10[-] / [green]q[-]      Quit`
	if vh := s.currentView().help(); vh != "" {
		help += fmt.Sprintf("\n\n[green]%s:[-]\n\n%s", s.currentView().title(), vh)
	}
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetText(help)
	buttons := newFormStyled().
		AddButton("Show limactl --help", s.showUsage).
		AddButton("Cancel", s.backToList).
		SetCancelFunc(s.backToList)
	buttons.SetBorderPadding(0, 0, 0, 0)
	buttons.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		return formArrowKey(buttons, e)
	})
	helpView := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(tv, 0, 1, false).
		AddItem(buttons, 1, 0, true)
	helpView.SetBorder(true).
		SetBorderColor(cBord).
		SetTitle(" Help ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(cPrim).
		SetBorderPadding(1, 1, 1, 1)
	s.app.SetRoot(helpView, true).SetFocus(buttons)
}

func (s *State) showUsage() {
	out, err := exec.CommandContext(s.ctx, s.limactl, "--help").CombinedOutput()
	if err != nil {
		s.showTextView(" Error ", "Failed to get the usage: "+tview.Escape(err.Error()))
		return
	}
	s.showTextView(" limactl --help ", tview.Escape(string(out)))
}

// refreshList reloads the current view, keeping the selected row.
func (s *State) refreshList() {
	key := s.currentKey()
	s.loadView()
	s.selectKey(key)
}

// action is an item of the actions menu.
type action struct {
	name string
	fn   func()
}

// showActions shows the actions menu with the given actions and a "Cancel" item.
func (s *State) showActions(title string, actions []action) {
	modal := tview.NewList().
		SetSelectedStyle(tcell.StyleDefault.Reverse(true)).
		SetHighlightFullLine(true).
		SetMainTextColor(cTxt)
	modal.SetBorder(true).
		SetBorderColor(cBord).
		SetTitle(title).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(cSec).
		SetBorderPadding(1, 1, 1, 1)
	modal.SetInputCapture(s.handleListInput)

	for _, a := range append(actions, action{"Cancel", nil}) {
		modal.AddItem("  "+a.name, "", 0, func() {
			s.backToList()
			if a.fn != nil {
				a.fn()
			}
		})
	}
	modal.SetDoneFunc(s.backToList)

	s.app.SetRoot(modal, true).SetFocus(modal)
}

func (s *State) showTextView(title, text string) {
	tv := tview.NewTextView().
		SetDynamicColors(true).
		SetWrap(true).
		SetWordWrap(true).
		SetScrollable(true).
		SetText(text)
	tv.SetBorder(true).
		SetBorderColor(cBord).
		SetTitle(title).
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(cPrim).
		SetBorderPadding(1, 1, 1, 1)

	s.app.SetRoot(tv, true)

	tv.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyEscape || e.Rune() == 'q' || e.Rune() == 'Q' {
			s.backToList()
			return nil
		}
		return e
	})
}

func (s *State) confirm(text string, onYes func()) {
	m := tview.NewModal().
		SetText(text).
		AddButtons([]string{"Yes", "No"}).
		SetDoneFunc(func(i int, _ string) {
			s.backToList()
			if i == 0 {
				onYes()
			}
		})
	styleModal(m)
	s.app.SetRoot(m, false).SetFocus(m)
}

// styleModal sets the border and button styles of the modal.
func styleModal(m *tview.Modal) {
	m.SetBorderColor(cBord)

	m.SetButtonStyle(
		tcell.StyleDefault.
			Foreground(tcell.ColorDefault).
			Background(tcell.ColorDefault),
	)

	m.SetButtonActivatedStyle(sActive)
}

// execCmd runs `limactl ACTION NAME`, and then selects the row with the given name.
func (s *State) execCmd(action, name string) {
	s.runSuspended(name, func() error {
		return s.runLimactl(nil, action, name)
	})
}

// runSuspended suspends the menu while running fn, and then refreshes the current view
// and selects the row with the given key.
func (s *State) runSuspended(key string, fn func() error) {
	suspend := s.app.Suspend(func() {
		if err := fn(); err != nil {
			fmt.Fprintf(os.Stderr, "%v\nPress Enter to go back to the menu.\n", err)
			if _, err = bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
				fmt.Fprintf(os.Stderr, "couldn't read enter input: %v\n", err)
				return
			}
		}

		s.refreshList()
	})

	s.selectKey(key)

	if !suspend {
		s.showTextView(" ERROR ", "Unable to suspend the menu")
		return
	}
}

// runLimactl runs limactl on the terminal. When stdin is nil, os.Stdin is used.
func (s *State) runLimactl(stdin io.Reader, args ...string) error {
	cmd := exec.CommandContext(context.Background(), s.limactl, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("limactl %s failed: %w", strings.Join(args, " "), err)
	}
	return nil
}
