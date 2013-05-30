// Copyright 2010 The Walk Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package walk

import (
	"fmt"
	"syscall"
	"unsafe"
)

import . "github.com/lxn/go-winapi"

type ToolBar struct {
	WidgetBase
	imageList          *ImageList
	actions            *ActionList
	defaultButtonWidth int
	maxTextRows        int
}

func newToolBar(parent Container, style uint32) (*ToolBar, error) {
	tb := new(ToolBar)
	tb.actions = newActionList(tb)

	if err := InitChildWidget(
		tb,
		parent,
		"ToolbarWindow32",
		CCS_NODIVIDER|TBSTYLE_FLAT|TBSTYLE_TOOLTIPS|style,
		0); err != nil {
		return nil, err
	}

	exStyle := tb.SendMessage(TB_GETEXTENDEDSTYLE, 0, 0)
	exStyle |= TBSTYLE_EX_DRAWDDARROWS | TBSTYLE_EX_MIXEDBUTTONS
	tb.SendMessage(TB_SETEXTENDEDSTYLE, 0, exStyle)

	return tb, nil
}

func NewToolBar(parent Container) (*ToolBar, error) {
	return newToolBar(parent, TBSTYLE_LIST|TBSTYLE_WRAPABLE)
}

func NewVerticalToolBar(parent Container) (*ToolBar, error) {
	tb, err := newToolBar(parent, CCS_VERT|CCS_NORESIZE)
	if err != nil {
		return nil, err
	}

	tb.defaultButtonWidth = 100

	return tb, nil
}

func (tb *ToolBar) LayoutFlags() LayoutFlags {
	style := GetWindowLong(tb.hWnd, GWL_STYLE)

	if style&CCS_VERT > 0 {
		return ShrinkableVert | GrowableVert | GreedyVert
	}

	// FIXME: Since reimplementation of BoxLayout we must return 0 here,
	// otherwise the ToolBar contained in MainWindow will eat half the space.
	return 0 //ShrinkableHorz | GrowableHorz
}

func (tb *ToolBar) MinSizeHint() Size {
	return tb.SizeHint()
}

func (tb *ToolBar) SizeHint() Size {
	if tb.actions.Len() == 0 {
		return Size{}
	}

	size := uint32(tb.SendMessage(TB_GETBUTTONSIZE, 0, 0))

	width := tb.defaultButtonWidth
	if width == 0 {
		width = int(LOWORD(size))
	}

	height := int(HIWORD(size))

	return Size{width, height}
}

func (tb *ToolBar) applyDefaultButtonWidth() error {
	if tb.defaultButtonWidth == 0 {
		return nil
	}

	lParam := uintptr(
		MAKELONG(uint16(tb.defaultButtonWidth), uint16(tb.defaultButtonWidth)))
	if 0 == tb.SendMessage(TB_SETBUTTONWIDTH, 0, lParam) {
		return newError("SendMessage(TB_SETBUTTONWIDTH)")
	}

	size := uint32(tb.SendMessage(TB_GETBUTTONSIZE, 0, 0))
	height := HIWORD(size)

	lParam = uintptr(MAKELONG(uint16(tb.defaultButtonWidth), height))
	if FALSE == tb.SendMessage(TB_SETBUTTONSIZE, 0, lParam) {
		return newError("SendMessage(TB_SETBUTTONSIZE)")
	}

	return nil
}

// DefaultButtonWidth returns the default button width of the ToolBar.
//
// The default value for a horizontal ToolBar is 0, resulting in automatic
// sizing behavior. For a vertical ToolBar, the default is 100 pixels.
func (tb *ToolBar) DefaultButtonWidth() int {
	return tb.defaultButtonWidth
}

// SetDefaultButtonWidth sets the default button width of the ToolBar.
//
// Calling this method affects all buttons in the ToolBar, no matter if they are
// added before or after the call. A width of 0 results in automatic sizing
// behavior. Negative values are not allowed.
func (tb *ToolBar) SetDefaultButtonWidth(width int) error {
	if width == tb.defaultButtonWidth {
		return nil
	}

	if width < 0 {
		return newError("width must be >= 0")
	}

	old := tb.defaultButtonWidth

	tb.defaultButtonWidth = width

	for _, action := range tb.actions.actions {
		if err := tb.onActionChanged(action); err != nil {
			tb.defaultButtonWidth = old

			return err
		}
	}

	return tb.applyDefaultButtonWidth()
}

func (tb *ToolBar) MaxTextRows() int {
	return tb.maxTextRows
}

func (tb *ToolBar) SetMaxTextRows(maxTextRows int) error {
	if 0 == tb.SendMessage(TB_SETMAXTEXTROWS, uintptr(maxTextRows), 0) {
		return newError("SendMessage(TB_SETMAXTEXTROWS)")
	}

	tb.maxTextRows = maxTextRows

	return nil
}

func (tb *ToolBar) Actions() *ActionList {
	return tb.actions
}

func (tb *ToolBar) ImageList() *ImageList {
	return tb.imageList
}

func (tb *ToolBar) SetImageList(value *ImageList) {
	var hIml HIMAGELIST

	if value != nil {
		hIml = value.hIml
	}

	tb.SendMessage(TB_SETIMAGELIST, 0, uintptr(hIml))

	tb.imageList = value
}

func (tb *ToolBar) imageIndex(image *Bitmap) (imageIndex int32, err error) {
	imageIndex = -1
	if image != nil {
		// FIXME: Protect against duplicate insertion
		if imageIndex, err = tb.imageList.AddMasked(image); err != nil {
			return
		}
	}

	return
}

func (tb *ToolBar) WndProc(hwnd HWND, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		switch HIWORD(uint32(wParam)) {
		case BN_CLICKED:
			actionId := uint16(LOWORD(uint32(wParam)))
			if action, ok := actionsById[actionId]; ok {
				action.raiseTriggered()
				return 0
			}
		}

	case WM_NOTIFY:
		nmhdr := (*NMHDR)(unsafe.Pointer(lParam))

		switch int32(nmhdr.Code) {
		case TBN_DROPDOWN:
			nmtb := (*NMTOOLBAR)(unsafe.Pointer(lParam))
			actionId := uint16(nmtb.IItem)
			if action := actionsById[actionId]; action != nil {
				var r RECT
				if 0 == tb.SendMessage(TB_GETRECT, uintptr(actionId), uintptr(unsafe.Pointer(&r))) {
					break
				}

				p := POINT{r.Left, r.Bottom}

				if !ClientToScreen(tb.hWnd, &p) {
					break
				}

				TrackPopupMenuEx(
					action.menu.hMenu,
					TPM_NOANIMATION,
					p.X,
					p.Y,
					tb.hWnd,
					nil)

				return TBDDRET_DEFAULT
			}
		}
	}

	return tb.WidgetBase.WndProc(hwnd, msg, wParam, lParam)
}

func (tb *ToolBar) initButtonForAction(action *Action, state, style *byte, image *int32, text *uintptr) (err error) {
	if tb.hasStyleBits(CCS_VERT) {
		*state |= TBSTATE_WRAP
	} else if tb.defaultButtonWidth == 0 {
		*style |= BTNS_AUTOSIZE
	}

	if action.checked {
		*state |= TBSTATE_CHECKED
	}

	if action.enabled {
		*state |= TBSTATE_ENABLED
	}

	if action.checkable {
		*style |= BTNS_CHECK
	}

	if action.exclusive {
		*style |= BTNS_GROUP
	}

	if action.image == nil {
		*style |= BTNS_SHOWTEXT
	}

	if action.menu != nil {
		if len(action.Triggered().handlers) > 0 {
			*style |= BTNS_DROPDOWN
		} else {
			*style |= BTNS_WHOLEDROPDOWN
		}
	}

	if action.IsSeparator() {
		*style = BTNS_SEP
	}

	if *image, err = tb.imageIndex(action.image); err != nil {
		return
	}

	var actionText string
	if s := action.shortcut; s.Key != 0 {
		actionText = fmt.Sprintf("%s (%s)", action.Text(), s.String())
	} else {
		actionText = action.Text()
	}

	*text = uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(actionText)))

	return
}

func (tb *ToolBar) onActionChanged(action *Action) error {
	tbbi := TBBUTTONINFO{
		DwMask: TBIF_IMAGE | TBIF_STATE | TBIF_STYLE | TBIF_TEXT,
	}

	tbbi.CbSize = uint32(unsafe.Sizeof(tbbi))

	if err := tb.initButtonForAction(
		action,
		&tbbi.FsState,
		&tbbi.FsStyle,
		&tbbi.IImage,
		&tbbi.PszText); err != nil {

		return err
	}

	if 0 == tb.SendMessage(
		TB_SETBUTTONINFO,
		uintptr(action.id),
		uintptr(unsafe.Pointer(&tbbi))) {

		return newError("SendMessage(TB_SETBUTTONINFO) failed")
	}

	return nil
}

func (tb *ToolBar) onActionVisibleChanged(action *Action) error {
	if !action.IsSeparator() {
		defer tb.actions.updateSeparatorVisibility()
	}

	if action.Visible() {
		return tb.insertAction(action, true)
	}

	return tb.removeAction(action, true)
}

func (tb *ToolBar) insertAction(action *Action, visibleChanged bool) (err error) {
	if !visibleChanged {
		action.addChangedHandler(tb)
		defer func() {
			if err != nil {
				action.removeChangedHandler(tb)
			}
		}()
	}

	if !action.Visible() {
		return
	}

	index := tb.actions.indexInObserver(action)

	tbb := TBBUTTON{
		IdCommand: int32(action.id),
	}

	if err = tb.initButtonForAction(
		action,
		&tbb.FsState,
		&tbb.FsStyle,
		&tbb.IBitmap,
		&tbb.IString); err != nil {

		return
	}

	tb.SetVisible(true)

	tb.SendMessage(TB_BUTTONSTRUCTSIZE, uintptr(unsafe.Sizeof(tbb)), 0)

	if FALSE == tb.SendMessage(TB_INSERTBUTTON, uintptr(index), uintptr(unsafe.Pointer(&tbb))) {
		return newError("SendMessage(TB_ADDBUTTONS)")
	}

	if err = tb.applyDefaultButtonWidth(); err != nil {
		return
	}

	tb.SendMessage(TB_AUTOSIZE, 0, 0)

	return
}

func (tb *ToolBar) removeAction(action *Action, visibleChanged bool) error {
	index := tb.actions.indexInObserver(action)

	if !visibleChanged {
		action.removeChangedHandler(tb)
	}

	if 0 == tb.SendMessage(TB_DELETEBUTTON, uintptr(index), 0) {
		return newError("SendMessage(TB_DELETEBUTTON) failed")
	}

	return nil
}

func (tb *ToolBar) onInsertedAction(action *Action) error {
	return tb.insertAction(action, false)
}

func (tb *ToolBar) onRemovingAction(action *Action) error {
	return tb.removeAction(action, false)
}

func (tb *ToolBar) onClearingActions() error {
	for i := tb.actions.Len() - 1; i >= 0; i-- {
		if action := tb.actions.At(i); action.Visible() {
			if err := tb.onRemovingAction(action); err != nil {
				return err
			}
		}
	}

	return nil
}
