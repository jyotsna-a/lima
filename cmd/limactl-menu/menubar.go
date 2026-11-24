// SPDX-FileCopyrightText: Copyright The Lima Authors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"

	"github.com/lima-vm/lima/v2/pkg/version"
)

// menu is a menu of the menu bar, such as "File".
type menu struct {
	name string
	// items returns the items of the menu. It is called each time the menu is opened,
	// so that the items can depend on the current view and the selected row
	items func() []action
}

// menus returns the menus of the menu bar.
func (s *State) menus() []menu {
	return []menu{
		{"File", s.fileMenu},
		{"Edit", s.editMenu},
		{"View", s.viewMenu},
		{"Help", s.helpMenu},
	}
}

func (s *State) fileMenu() []action {
	return []action{
		{"Open a template file...", s.showOpenTemplate},
		{"Exit", s.app.Stop},
	}
}

// editMenu has the actions of the current view for the selected row.
func (s *State) editMenu() []action {
	return s.currentView().actions(s.currentRow())
}

func (s *State) viewMenu() []action {
	items := make([]action, 0, len(s.views)+1)
	for i, v := range s.views {
		mark := "  "
		if i == s.cur {
			mark = "• "
		}
		items = append(items, action{mark + v.title(), func() { s.switchView(i) }})
	}
	return append(items, action{"  Refresh", s.refreshList})
}

func (s *State) helpMenu() []action {
	return []action{
		{"Help", s.showHelp},
		{"About", s.showAbout},
	}
}

// menuBarStarts returns the column of each menu name in the menu bar.
func menuBarStarts(menus []menu) []int {
	starts := make([]int, len(menus))
	x := 1
	for i, m := range menus {
		starts[i] = x
		x += len(m.name) + 2
	}
	return starts
}

// fmtMenuBar returns the text of the menu bar, with the open menu highlighted
// open is -1 when no menu is open.
func fmtMenuBar(menus []menu, open int) string {
	var b strings.Builder
	b.WriteString(" ")
	for i, m := range menus {
		if i == open {
			fmt.Fprintf(&b, "[::r] %s [::-]", m.name)
		} else {
			fmt.Fprintf(&b, " %s ", m.name)
		}
	}
	return b.String()
}

// menuAt returns the index of the menu at the given column of the menu bar, or -1.
func menuAt(menus []menu, x int) int {
	for i, start := range menuBarStarts(menus) {
		if x >= start && x < start+len(menus[i].name)+2 {
			return i
		}
	}
	return -1
}

// newMenuBar returns the menu bar shown at the top of the main screen.
func (s *State) newMenuBar() *tview.TextView {
	bar := tview.NewTextView().SetDynamicColors(true)
	bar.SetText(fmtMenuBar(s.menus(), -1))
	bar.SetMouseCapture(func(a tview.MouseAction, e *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if a != tview.MouseLeftClick {
			return a, e
		}
		x, _ := e.Position()
		bx, _, _, _ := bar.GetRect()
		if i := menuAt(s.menus(), x-bx); i >= 0 {
			s.openMenu(i)
		}
		return tview.MouseConsumed, nil
	})
	return bar
}

// openMenu opens the i-th menu of the menu bar as a dropdown over the main screen.
func (s *State) openMenu(i int) {
	menus := s.menus()
	i = (i + len(menus)) % len(menus)
	s.openedMenu = i
	s.menuBar.SetText(fmtMenuBar(menus, i))

	items := menus[i].items()
	if len(items) == 0 {
		items = []action{{"(No actions)", nil}}
	}

	dropdown := tview.NewList().
		SetSelectedStyle(tcell.StyleDefault.Reverse(true)).
		SetHighlightFullLine(true).
		SetMainTextColor(cTxt).
		ShowSecondaryText(false)
	dropdown.SetBorder(true).SetBorderColor(cBord)
	width := 0
	for _, a := range items {
		width = max(width, len(a.name))
		dropdown.AddItem(" "+a.name+" ", "", 0, func() {
			s.closeMenu()
			if a.fn != nil {
				a.fn()
			}
		})
	}
	dropdown.SetDoneFunc(s.closeMenu)
	dropdown.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch {
		case e.Key() == tcell.KeyLeft:
			s.openMenu(i - 1)
			return nil
		case e.Key() == tcell.KeyRight:
			s.openMenu(i + 1)
			return nil
		case e.Rune() == 'j':
			return tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone)
		case e.Rune() == 'k':
			return tcell.NewEventKey(tcell.KeyUp, 0, tcell.ModNone)
		}
		return e
	})

	// The flexes without a background keep the main screen visible around the dropdown
	row := tview.NewFlex().
		AddItem(nil, menuBarStarts(menus)[i]-1, 0, false).
		AddItem(dropdown, width+4, 0, true).
		AddItem(nil, 0, 1, false)
	overlay := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(nil, 1, 0, false).
		AddItem(row, len(items)+2, 0, true).
		AddItem(nil, 0, 1, false)
	// A click outside the dropdown closes it, and a click on another menu opens that menu
	overlay.SetMouseCapture(func(a tview.MouseAction, e *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		x, y := e.Position()
		if a != tview.MouseLeftClick || dropdown.InRect(x, y) {
			return a, e
		}
		bx, by, _, _ := s.menuBar.GetRect()
		if j := menuAt(menus, x-bx); y == by && j >= 0 && j != i {
			s.openMenu(j)
		} else {
			s.closeMenu()
		}
		return tview.MouseConsumed, nil
	})

	pages := tview.NewPages().
		AddPage("main", s.flex, true, true).
		AddPage("menu", overlay, true, true)
	s.app.SetRoot(pages, true).SetFocus(dropdown)
}

// closeMenu closes the open menu and goes back to the main screen.
func (s *State) closeMenu() {
	s.openedMenu = -1
	s.menuBar.SetText(fmtMenuBar(s.menus(), -1))
	s.backToList()
}

// menuKey returns the index of the menu opened by Alt and the first letter of its name, or -1.
func menuKey(menus []menu, e *tcell.EventKey) int {
	if e.Key() != tcell.KeyRune || e.Modifiers()&tcell.ModAlt == 0 {
		return -1
	}
	for i, m := range menus {
		if strings.EqualFold(m.name[:1], string(e.Rune())) {
			return i
		}
	}
	return -1
}

// showOpenTemplate asks for the path of a template file, and creates an instance from it.
func (s *State) showOpenTemplate() {
	form := newFormStyled().
		AddInputField("Template file", "", 60, nil, nil)
	field := form.GetFormItem(0).(*tview.InputField)
	open := func() {
		path, err := expandHome(strings.TrimSpace(field.GetText()))
		if err != nil {
			s.showTextView(" Error ", tview.Escape(err.Error()))
			return
		}
		if path == "" {
			s.showTextView(" Error ", "The template file must be specified")
			return
		}
		s.backToList()
		s.runSuspended("", func() error {
			return s.runLimactl(nil, "create", path)
		})
	}
	form.AddButton("Open", open).
		AddButton("Cancel", s.backToList).
		SetCancelFunc(s.backToList)
	form.SetBorder(true).
		SetBorderColor(cBord).
		SetTitle(" Open a template file ").
		SetTitleAlign(tview.AlignLeft).
		SetTitleColor(cPrim).
		SetBorderPadding(1, 1, 1, 1)
	s.app.SetRoot(form, true).SetFocus(form)
}

// expandHome expands the leading "~/" of the path to the home directory.
func expandHome(path string) (string, error) {
	rest, ok := strings.CutPrefix(path, "~/")
	if !ok {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, rest), nil
}

func (s *State) showAbout() {
	text := "limactl menu (EXPERIMENTAL)\n\nVersion: " + strings.TrimPrefix(version.Version, "v")
	if out, err := exec.CommandContext(s.ctx, s.limactl, "--version").Output(); err == nil {
		text += "\n" + strings.TrimSpace(string(out))
	}
	text += "\n\nhttps://lima-vm.io/"
	s.showTextView(" About ", tview.Escape(text))
}
