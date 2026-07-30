package vterm

/*
#include <stdint.h>
*/
import "C"

import (
	"unsafe"

	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
)

//export goDamage
func goDamage(startRow, endRow, startCol, endCol C.int, user C.uintptr_t) C.int {
	_ = startCol
	_ = endCol
	t := lookup(user)
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	for r := int(startRow); r < int(endRow); r++ {
		t.dirty[r] = struct{}{}
	}
	return 1
}

//export goMovecursor
func goMovecursor(row, col, oldrow, oldcol, visible C.int, user C.uintptr_t) C.int {
	_ = oldrow
	_ = oldcol
	t := lookup(user)
	if t == nil {
		return 0
	}
	t.mu.Lock()
	t.cursorX = int(col)
	t.cursorY = int(row)
	t.cursorOn = visible != 0
	t.mu.Unlock()
	return 1
}

//export goSettermprop
func goSettermprop(prop, boolVal, numberVal C.int, str *C.char, strLen C.int, user C.uintptr_t) C.int {
	t := lookup(user)
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch prop {
	case 4: // TITLE
		if str != nil && strLen > 0 {
			t.title = C.GoStringN(str, strLen)
		}
	case 1: // CURSORVISIBLE
		t.cursorOn = boolVal != 0
	case 3: // ALTSCREEN
		t.altScreen = boolVal != 0
		if t.altScreen {
			t.scrollOff = 0
		}
	case 8: // MOUSE
		t.mouse = int(numberVal)
	}
	return 1
}

//export goBell
func goBell(user C.uintptr_t) C.int {
	t := lookup(user)
	if t != nil {
		t.mu.Lock()
		t.bellCount++
		t.mu.Unlock()
	}
	return 1
}

//export goSBPushCells
func goSBPushCells(cols C.int, chars *C.uint32_t, fg, bg, attrs *C.uint8_t, user C.uintptr_t) C.int {
	t := lookup(user)
	if t == nil || t.scrollMax == 0 {
		return 1
	}
	n := int(cols)
	line := make([]cell.Cell, n)
	charSlice := unsafe.Slice(chars, n)
	fgSlice := unsafe.Slice(fg, n*3)
	bgSlice := unsafe.Slice(bg, n*3)
	attrSlice := unsafe.Slice(attrs, n)
	for i := 0; i < n; i++ {
		var attr cell.Attr
		a := uint8(attrSlice[i])
		if a&1 != 0 {
			attr |= cell.AttrBold
		}
		if a&2 != 0 {
			attr |= cell.AttrUnderline
		}
		if a&4 != 0 {
			attr |= cell.AttrItalic
		}
		if a&16 != 0 {
			attr |= cell.AttrReverse
		}
		line[i] = cell.Cell{
			Rune: rune(charSlice[i]),
			FG:   cell.RGB(uint8(fgSlice[i*3]), uint8(fgSlice[i*3+1]), uint8(fgSlice[i*3+2])),
			BG:   cell.RGB(uint8(bgSlice[i*3]), uint8(bgSlice[i*3+1]), uint8(bgSlice[i*3+2])),
			Attr: attr,
		}
	}
	t.mu.Lock()
	t.scrollback = append(t.scrollback, line)
	if len(t.scrollback) > t.scrollMax {
		t.scrollback = t.scrollback[len(t.scrollback)-t.scrollMax:]
	}
	t.mu.Unlock()
	return 1
}

//export goSBPop
func goSBPop(cols C.int, user C.uintptr_t) C.int {
	_ = cols
	t := lookup(user)
	if t == nil {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.scrollback) == 0 {
		return 0
	}
	t.scrollback = t.scrollback[:len(t.scrollback)-1]
	return 1
}

//export goSBClear
func goSBClear(user C.uintptr_t) C.int {
	t := lookup(user)
	if t == nil {
		return 1
	}
	t.mu.Lock()
	t.scrollback = nil
	t.scrollOff = 0
	t.mu.Unlock()
	return 1
}

//export goOutput
func goOutput(s *C.char, length C.size_t, user C.uintptr_t) {
	t := lookup(user)
	if t == nil || s == nil || length == 0 {
		return
	}
	b := C.GoBytes(unsafe.Pointer(s), C.int(length))
	t.mu.Lock()
	fn := t.onOutput
	t.mu.Unlock()
	if fn != nil {
		fn(b)
	}
}
