package vterm

/*
#cgo CFLAGS: -I${SRCDIR}/../../third_party/libvterm-0.3.3/include -std=c99
#cgo LDFLAGS: ${SRCDIR}/../../third_party/libvterm-0.3.3/build/libvterm.a

#include <vterm.h>
#include <stdlib.h>
#include <string.h>
#include <stdint.h>
#include <stdbool.h>

// user is a runtime/cgo.Handle threaded through C as a plain integer, never
// as a pointer. cgo.Handle values are opaque tokens, not addresses of Go
// memory; converting one straight to unsafe.Pointer on the Go side violates
// the unsafe.Pointer rules and can fatal with "checkptr: pointer arithmetic
// computed bad pointer value" under load (observed via rapid PTY creation).
// Casting between uintptr_t and void* only ever happens here in C, where
// Go's pointer checker doesn't apply.
extern int goDamage(int start_row, int end_row, int start_col, int end_col, uintptr_t user);
extern int goMovecursor(int row, int col, int oldrow, int oldcol, int visible, uintptr_t user);
extern int goSettermprop(int prop, int bool_val, int number_val, const char *str, int str_len, uintptr_t user);
extern int goBell(uintptr_t user);
extern int goSBPushCells(int cols, uint32_t *chars, uint8_t *fg, uint8_t *bg, uint8_t *attrs, uintptr_t user);
extern int goSBPop(int cols, uintptr_t user);
extern int goSBClear(uintptr_t user);
extern void goOutput(const char *s, size_t len, uintptr_t user);

static void indexed_to_rgb(uint8_t idx, uint8_t *r, uint8_t *g, uint8_t *b) {
  if (idx < 16) {
    static const uint8_t ansi[16][3] = {
      {0,0,0},{170,0,0},{0,170,0},{170,85,0},{0,0,170},{170,0,170},{0,170,170},{170,170,170},
      {85,85,85},{255,85,85},{85,255,85},{255,255,85},{85,85,255},{255,85,255},{85,255,255},{255,255,255}
    };
    *r = ansi[idx][0]; *g = ansi[idx][1]; *b = ansi[idx][2];
    return;
  }
  if (idx < 232) {
    idx -= 16;
    uint8_t cr = idx / 36, cg = (idx / 6) % 6, cb = idx % 6;
    *r = cr ? cr * 40 + 55 : 0;
    *g = cg ? cg * 40 + 55 : 0;
    *b = cb ? cb * 40 + 55 : 0;
    return;
  }
  uint8_t v = 8 + (idx - 232) * 10;
  *r = *g = *b = v;
}

static void color_to_rgb(VTermColor col, uint8_t *r, uint8_t *g, uint8_t *b) {
  if (VTERM_COLOR_IS_RGB(&col)) {
    *r = col.rgb.red; *g = col.rgb.green; *b = col.rgb.blue;
  } else {
    indexed_to_rgb(col.indexed.idx, r, g, b);
  }
}

static int c_damage(VTermRect rect, void *user) {
  return goDamage(rect.start_row, rect.end_row, rect.start_col, rect.end_col, (uintptr_t)user);
}
static int c_movecursor(VTermPos pos, VTermPos oldpos, int visible, void *user) {
  return goMovecursor(pos.row, pos.col, oldpos.row, oldpos.col, visible, (uintptr_t)user);
}
static int c_settermprop(VTermProp prop, VTermValue *val, void *user) {
  int b = 0, n = 0;
  const char *s = NULL;
  int sl = 0;
  if (val) {
    switch (prop) {
      case VTERM_PROP_CURSORVISIBLE:
      case VTERM_PROP_ALTSCREEN:
      case VTERM_PROP_REVERSE:
      case VTERM_PROP_CURSORBLINK:
      case VTERM_PROP_FOCUSREPORT:
        b = val->boolean;
        break;
      case VTERM_PROP_MOUSE:
      case VTERM_PROP_CURSORSHAPE:
        n = val->number;
        break;
      case VTERM_PROP_TITLE:
      case VTERM_PROP_ICONNAME:
        s = val->string.str;
        sl = (int)val->string.len;
        break;
      default:
        break;
    }
  }
  return goSettermprop((int)prop, b, n, s, sl, (uintptr_t)user);
}
static int c_bell(void *user) { return goBell((uintptr_t)user); }

static int c_sb_push(int cols, const VTermScreenCell *cells, void *user) {
  if (cols <= 0 || cells == NULL) return 1;
  uint32_t *chars = malloc(sizeof(uint32_t) * (size_t)cols);
  uint8_t *fg = malloc((size_t)cols * 3);
  uint8_t *bg = malloc((size_t)cols * 3);
  uint8_t *attrs = malloc((size_t)cols);
  if (!chars || !fg || !bg || !attrs) {
    free(chars); free(fg); free(bg); free(attrs);
    return 0;
  }
  for (int i = 0; i < cols; i++) {
    chars[i] = cells[i].chars[0] ? cells[i].chars[0] : (uint32_t)' ';
    color_to_rgb(cells[i].fg, &fg[i*3], &fg[i*3+1], &fg[i*3+2]);
    color_to_rgb(cells[i].bg, &bg[i*3], &bg[i*3+1], &bg[i*3+2]);
    attrs[i] = 0;
    if (cells[i].attrs.bold) attrs[i] |= 1;
    if (cells[i].attrs.underline) attrs[i] |= 2;
    if (cells[i].attrs.italic) attrs[i] |= 4;
    if (cells[i].attrs.reverse) attrs[i] |= 16;
  }
  int r = goSBPushCells(cols, chars, fg, bg, attrs, (uintptr_t)user);
  free(chars); free(fg); free(bg); free(attrs);
  return r;
}
static int c_sb_pop(int cols, VTermScreenCell *cells, void *user) {
  (void)cells;
  return goSBPop(cols, (uintptr_t)user);
}
static int c_sb_clear(void *user) { return goSBClear((uintptr_t)user); }
static void c_output(const char *s, size_t len, void *user) { goOutput(s, len, (uintptr_t)user); }

static VTermScreenCallbacks screen_cbs = {
  .damage = c_damage,
  .movecursor = c_movecursor,
  .settermprop = c_settermprop,
  .bell = c_bell,
  .sb_pushline = c_sb_push,
  .sb_popline = c_sb_pop,
  .sb_clear = c_sb_clear,
};

static void setup_screen(VTerm *vt, uintptr_t user) {
  void *voiduser = (void *)user;
  VTermScreen *screen = vterm_obtain_screen(vt);
  vterm_screen_set_callbacks(screen, &screen_cbs, voiduser);
  vterm_screen_set_damage_merge(screen, VTERM_DAMAGE_ROW);
  vterm_screen_enable_altscreen(screen, 1);
  vterm_screen_reset(screen, 1);
  vterm_output_set_callback(vt, c_output, voiduser);
  vterm_set_utf8(vt, 1);
}

typedef struct {
  uint32_t ch;
  uint8_t fr, fg, fb;
  uint8_t br, bg, bb;
  uint8_t attrs;
} GoCell;

static void fill_go_cell(VTermScreen *screen, int row, int col, GoCell *out) {
  VTermPos pos = { .row = row, .col = col };
  VTermScreenCell sc;
  memset(&sc, 0, sizeof(sc));
  vterm_screen_get_cell(screen, pos, &sc);
  VTermColor fgc = sc.fg, bgc = sc.bg;
  vterm_screen_convert_color_to_rgb(screen, &fgc);
  vterm_screen_convert_color_to_rgb(screen, &bgc);
  out->ch = sc.chars[0] ? sc.chars[0] : (uint32_t)' ';
  out->fr = fgc.rgb.red; out->fg = fgc.rgb.green; out->fb = fgc.rgb.blue;
  out->br = bgc.rgb.red; out->bg = bgc.rgb.green; out->bb = bgc.rgb.blue;
  out->attrs = 0;
  if (sc.attrs.bold) out->attrs |= 1;
  if (sc.attrs.underline) out->attrs |= 2;
  if (sc.attrs.italic) out->attrs |= 4;
  if (sc.attrs.blink) out->attrs |= 8;
  if (sc.attrs.reverse) out->attrs |= 16;
  if (sc.attrs.strike) out->attrs |= 32;
}

static void set_defaults(VTerm *vt, uint8_t fr, uint8_t fg, uint8_t fb, uint8_t br, uint8_t bg, uint8_t bb) {
  VTermColor fgc, bgc;
  vterm_color_rgb(&fgc, fr, fg, fb);
  vterm_color_rgb(&bgc, br, bg, bb);
  vterm_state_set_default_colors(vterm_obtain_state(vt), &fgc, &bgc);
}
*/
import "C"

import (
	"runtime/cgo"
	"strings"
	"sync"
	"unsafe"

	"github.com/ttypedesk/ttypedesk/pkg/cell"
)

var (
	registryMu sync.RWMutex
	registry   = map[cgo.Handle]*Term{}
)

// Term wraps a libvterm instance with dirty tracking and scrollback.
type Term struct {
	vt     *C.VTerm
	screen *C.VTermScreen
	// vtMu serializes every call into libvterm against Close's vterm_free.
	// Write/Resize/Close/collectOutput used to capture vt/screen under mu,
	// unlock, then pass the pointer to C — so a concurrent Close could free
	// it in that window and hand the next libvterm call a dangling pointer
	// (observed as a SIGSEGV inside vterm_set_size under rapid resize).
	// Callbacks (goDamage etc.) only ever take mu, never vtMu, so holding
	// vtMu across a libvterm call — which synchronously re-enters Go via
	// those callbacks — can't deadlock.
	vtMu       sync.Mutex
	handle     cgo.Handle
	rows       int
	cols       int
	mu         sync.Mutex
	dirty      map[int]struct{}
	full       bool
	title      string
	cursorX    int
	cursorY    int
	cursorOn   bool
	mouse      int
	altScreen  bool
	scrollback [][]cell.Cell
	scrollMax  int
	scrollOff  int // lines above live bottom
	onOutput   func([]byte)
	defFG      cell.Color
	defBG      cell.Color
	gen        uint64 // bumps on content change
	bellCount  uint64 // incremented on BEL; consumed by TakeBell
}

// New creates a new terminal emulator.
func New(rows, cols, scrollback int) *Term {
	if rows < 1 {
		rows = 24
	}
	if cols < 1 {
		cols = 80
	}
	if scrollback < 0 {
		scrollback = 0
	}
	t := &Term{
		rows:      rows,
		cols:      cols,
		dirty:     make(map[int]struct{}),
		full:      true,
		cursorOn:  true,
		scrollMax: scrollback,
		defFG:     cell.RGB(0xCC, 0xCC, 0xCC),
		defBG:     cell.RGB(0x00, 0x00, 0x00),
	}
	t.vt = C.vterm_new(C.int(rows), C.int(cols))
	t.screen = C.vterm_obtain_screen(t.vt)
	t.handle = cgo.NewHandle(t)
	registryMu.Lock()
	registry[t.handle] = t
	registryMu.Unlock()
	C.setup_screen(t.vt, C.uintptr_t(t.handle))
	C.set_defaults(t.vt,
		C.uint8_t(t.defFG.R), C.uint8_t(t.defFG.G), C.uint8_t(t.defFG.B),
		C.uint8_t(t.defBG.R), C.uint8_t(t.defBG.G), C.uint8_t(t.defBG.B),
	)
	return t
}

func lookup(user C.uintptr_t) *Term {
	h := cgo.Handle(uintptr(user))
	registryMu.RLock()
	t := registry[h]
	registryMu.RUnlock()
	return t
}

func (t *Term) Close() {
	t.vtMu.Lock()
	vt := t.vt
	t.vt = nil
	t.screen = nil
	if vt != nil {
		C.vterm_free(vt)
	}
	t.vtMu.Unlock()

	t.mu.Lock()
	h := t.handle
	t.mu.Unlock()
	registryMu.Lock()
	delete(registry, h)
	registryMu.Unlock()
	// Delete handle after unregister so late callbacks become no-ops.
	func() {
		defer func() { _ = recover() }()
		h.Delete()
	}()
}

func (t *Term) SetOutputCallback(fn func([]byte)) {
	t.mu.Lock()
	t.onOutput = fn
	t.mu.Unlock()
}

func (t *Term) Write(p []byte) {
	if len(p) == 0 {
		return
	}
	t.vtMu.Lock()
	defer t.vtMu.Unlock()
	vt, screen := t.vt, t.screen
	if vt == nil {
		return
	}
	t.mu.Lock()
	if t.scrollOff != 0 {
		t.scrollOff = 0 // new output jumps to bottom
		t.full = true
	}
	t.mu.Unlock()
	C.vterm_input_write(vt, (*C.char)(unsafe.Pointer(&p[0])), C.size_t(len(p)))
	C.vterm_screen_flush_damage(screen)
	t.mu.Lock()
	t.gen++
	t.mu.Unlock()
}

func (t *Term) Resize(rows, cols int) {
	if rows < 1 || cols < 1 {
		return
	}
	t.vtMu.Lock()
	defer t.vtMu.Unlock()
	vt := t.vt
	if vt == nil {
		return
	}
	t.mu.Lock()
	t.rows, t.cols = rows, cols
	t.full = true
	t.scrollOff = 0
	t.mu.Unlock()
	C.vterm_set_size(vt, C.int(rows), C.int(cols))
}

func (t *Term) Size() (cols, rows int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.cols, t.rows
}

func (t *Term) Title() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.title
}

// TakeBell returns true if one or more BEL sequences arrived since the last call.
func (t *Term) TakeBell() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.bellCount == 0 {
		return false
	}
	t.bellCount = 0
	return true
}

func (t *Term) Cursor() (x, y int, visible bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.scrollOff != 0 {
		return t.cursorX, t.cursorY, false
	}
	return t.cursorX, t.cursorY, t.cursorOn
}

func (t *Term) MouseMode() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.mouse
}

func (t *Term) AltScreen() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.altScreen
}

func (t *Term) Generation() uint64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.gen
}

func (t *Term) ScrollOffset() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.scrollOff
}

// ScrollUIState returns scrollbar metrics: offset 0 = top of history, max = live bottom.
func (t *Term) ScrollUIState() (offset, content, viewport int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.altScreen || t.scrollMax == 0 {
		return 0, t.rows, t.rows
	}
	hist := len(t.scrollback)
	content = hist + t.rows
	viewport = t.rows
	offset = hist - t.scrollOff
	if offset < 0 {
		offset = 0
	}
	return offset, content, viewport
}

// SetScrollUIOffset sets scroll position from a UI scrollbar (0 = top of history).
func (t *Term) SetScrollUIOffset(offset int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.altScreen || t.scrollMax == 0 {
		return
	}
	hist := len(t.scrollback)
	if offset < 0 {
		offset = 0
	}
	if offset > hist {
		offset = hist
	}
	t.scrollOff = hist - offset
	t.full = true
	t.gen++
}

func (t *Term) HasScrollback() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return !t.altScreen && t.scrollMax > 0 && len(t.scrollback) > 0
}

// lineTextLocked returns plain text for composed-buffer line abs (0 = oldest).
func (t *Term) lineTextLocked(abs int, screen []cell.Cell) string {
	hist := len(t.scrollback)
	var line []cell.Cell
	if abs < hist {
		line = t.scrollback[abs]
	} else {
		sr := abs - hist
		if sr >= 0 && sr < t.rows && len(screen) >= (sr+1)*t.cols {
			line = screen[sr*t.cols : (sr+1)*t.cols]
		}
	}
	if line == nil {
		return ""
	}
	b := make([]rune, 0, len(line))
	for _, ch := range line {
		r := ch.Rune
		if r == 0 {
			r = ' '
		}
		b = append(b, r)
	}
	return strings.TrimRight(string(b), " ")
}

// SearchScrollback finds query in scrollback+screen. towardOlder=true searches into history.
// On success, scrolls so the match line is visible near the top and returns true.
func (t *Term) SearchScrollback(query string, towardOlder bool) (found bool, matches int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	q := strings.TrimSpace(query)
	if q == "" || t.altScreen {
		return false, 0
	}
	qLower := strings.ToLower(q)
	screen := t.readRect(0, 0, t.cols, t.rows)
	hist := len(t.scrollback)
	total := hist + t.rows
	if total < 1 {
		return false, 0
	}

	// Count all matches + collect abs indices
	var hits []int
	for abs := 0; abs < total; abs++ {
		line := strings.ToLower(t.lineTextLocked(abs, screen))
		if strings.Contains(line, qLower) {
			hits = append(hits, abs)
		}
	}
	matches = len(hits)
	if matches == 0 {
		return false, 0
	}

	// First visible abs line
	start := total - t.rows - t.scrollOff
	if start < 0 {
		start = 0
	}
	cur := start
	var pick int
	if towardOlder {
		// next older than current top: largest h that is < cur, else wrap to newest match
		foundPick := false
		for i := len(hits) - 1; i >= 0; i-- {
			if hits[i] < cur {
				pick = hits[i]
				foundPick = true
				break
			}
		}
		if !foundPick {
			pick = hits[len(hits)-1]
		}
	} else {
		foundPick := false
		for _, h := range hits {
			if h > cur {
				pick = h
				foundPick = true
				break
			}
		}
		if !foundPick {
			pick = hits[0] // wrap to oldest
		}
	}

	// Place pick near top of viewport
	newOff := hist - pick
	if newOff < 0 {
		newOff = 0
	}
	if newOff > hist {
		newOff = hist
	}
	t.scrollOff = newOff
	t.full = true
	t.gen++
	return true, matches
}

// ScrollBy moves the viewport into scrollback. Positive = up into history.
func (t *Term) ScrollBy(delta int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.altScreen || t.scrollMax == 0 {
		return
	}
	maxOff := len(t.scrollback)
	t.scrollOff += delta
	if t.scrollOff < 0 {
		t.scrollOff = 0
	}
	if t.scrollOff > maxOff {
		t.scrollOff = maxOff
	}
	t.full = true
	t.gen++
}

func (t *Term) KeyboardUnichar(r rune, mod int) []byte {
	return t.collectOutput(func(vt *C.VTerm) {
		C.vterm_keyboard_unichar(vt, C.uint32_t(r), C.VTermModifier(mod))
	})
}

func (t *Term) KeyboardKey(key int, mod int) []byte {
	return t.collectOutput(func(vt *C.VTerm) {
		C.vterm_keyboard_key(vt, C.VTermKey(key), C.VTermModifier(mod))
	})
}

// MouseButton sends a mouse button via libvterm (row/col 0-based).
func (t *Term) MouseButton(button int, pressed bool, row, col, mod int) []byte {
	return t.collectOutput(func(vt *C.VTerm) {
		C.vterm_mouse_move(vt, C.int(row), C.int(col), C.VTermModifier(mod))
		C.vterm_mouse_button(vt, C.int(button), C.bool(pressed), C.VTermModifier(mod))
	})
}

func (t *Term) MouseMove(row, col, mod int) []byte {
	return t.collectOutput(func(vt *C.VTerm) {
		C.vterm_mouse_move(vt, C.int(row), C.int(col), C.VTermModifier(mod))
	})
}

func (t *Term) collectOutput(fn func(vt *C.VTerm)) []byte {
	t.vtMu.Lock()
	defer t.vtMu.Unlock()
	vt := t.vt
	if vt == nil {
		return nil
	}
	t.mu.Lock()
	var buf []byte
	prev := t.onOutput
	t.onOutput = func(b []byte) { buf = append(buf, b...) }
	t.mu.Unlock()
	fn(vt)
	t.mu.Lock()
	t.onOutput = prev
	t.mu.Unlock()
	return buf
}

func (t *Term) Snapshot() []cell.Cell {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.viewLocked()
}

func (t *Term) ProduceDiff() cell.Diff {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.scrollOff != 0 || t.full {
		t.full = false
		t.dirty = make(map[int]struct{})
		cells := t.viewLocked()
		return cell.FullGridDiff(t.cols, t.rows, cells)
	}
	if len(t.dirty) == 0 {
		return cell.Diff{}
	}
	minR, maxR := t.rows, -1
	for r := range t.dirty {
		if r < minR {
			minR = r
		}
		if r > maxR {
			maxR = r
		}
	}
	t.dirty = make(map[int]struct{})
	if maxR < minR {
		return cell.Diff{}
	}
	h := maxR - minR + 1
	cells := t.readRect(0, minR, t.cols, h)
	return cell.Diff{Rect: cell.Rect{X: 0, Y: minR, W: t.cols, H: h}, Cells: cells}
}

func (t *Term) viewLocked() []cell.Cell {
	screen := t.readRect(0, 0, t.cols, t.rows)
	if t.scrollOff == 0 || len(t.scrollback) == 0 {
		return screen
	}
	// Compose: scrollback + screen, window ending scrollOff above bottom.
	total := len(t.scrollback) + t.rows
	start := total - t.rows - t.scrollOff
	if start < 0 {
		start = 0
	}
	out := make([]cell.Cell, t.cols*t.rows)
	for row := 0; row < t.rows; row++ {
		src := start + row
		var line []cell.Cell
		if src < len(t.scrollback) {
			line = t.scrollback[src]
		} else {
			sr := src - len(t.scrollback)
			if sr >= 0 && sr < t.rows {
				line = screen[sr*t.cols : (sr+1)*t.cols]
			}
		}
		for col := 0; col < t.cols; col++ {
			ch := cell.Cell{Rune: ' ', FG: t.defFG, BG: t.defBG}
			if line != nil && col < len(line) {
				ch = line[col]
			}
			out[row*t.cols+col] = ch
		}
	}
	return out
}

func (t *Term) readRect(x, y, w, h int) []cell.Cell {
	out := make([]cell.Cell, w*h)
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			var gc C.GoCell
			C.fill_go_cell(t.screen, C.int(y+row), C.int(x+col), &gc)
			out[row*w+col] = goCellToCell(gc)
		}
	}
	return out
}

func goCellToCell(gc C.GoCell) cell.Cell {
	var attr cell.Attr
	a := uint8(gc.attrs)
	if a&1 != 0 {
		attr |= cell.AttrBold
	}
	if a&2 != 0 {
		attr |= cell.AttrUnderline
	}
	if a&4 != 0 {
		attr |= cell.AttrItalic
	}
	if a&8 != 0 {
		attr |= cell.AttrBlink
	}
	if a&16 != 0 {
		attr |= cell.AttrReverse
	}
	if a&32 != 0 {
		attr |= cell.AttrStrike
	}
	return cell.Cell{
		Rune: rune(gc.ch),
		FG:   cell.RGB(uint8(gc.fr), uint8(gc.fg), uint8(gc.fb)),
		BG:   cell.RGB(uint8(gc.br), uint8(gc.bg), uint8(gc.bb)),
		Attr: attr,
	}
}

const (
	ModNone  = 0
	ModShift = 1
	ModAlt   = 2
	ModCtrl  = 4
)

const (
	KeyEnter     = 1
	KeyTab       = 2
	KeyBackspace = 3
	KeyEscape    = 4
	KeyUp        = 5
	KeyDown      = 6
	KeyLeft      = 7
	KeyRight     = 8
	KeyIns       = 9
	KeyDel       = 10
	KeyHome      = 11
	KeyEnd       = 12
	KeyPageUp    = 13
	KeyPageDown  = 14
)

func KeyFunction(n int) int { return 256 + n }
