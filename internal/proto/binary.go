package proto

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/RetroCodeRamen/ttypedesk/pkg/cell"
)

// Frame types for the length-prefixed wire framing every attach
// connection uses (replaces the original newline-delimited JSON-only
// framing). Input (key/mouse/attach/detach) stays JSON — small, and
// changing it wasn't the point; only the snapshot direction, which used
// to re-encode every window's full cell grid as JSON on every tick
// regardless of whether anything had changed, needed to go. FrameAudio
// carries raw PCM captured on the host (see internal/audiocap) to an
// attached client's speakers, opt-in and independent of the diff stream.
const (
	FrameJSON  byte = 1
	FrameDiff  byte = 2
	FrameAudio byte = 3
)

// maxFrameLen bounds a single frame so a corrupt/malicious length prefix
// can't make ReadFrame try to allocate an absurd buffer.
const maxFrameLen = 64 * 1024 * 1024

// WriteFrame writes one length-prefixed frame: a 4-byte big-endian length
// (of the type byte + payload), the type byte, then payload.
func WriteFrame(w io.Writer, typ byte, payload []byte) error {
	buf := make([]byte, 5+len(payload))
	binary.BigEndian.PutUint32(buf[0:4], uint32(len(payload)+1))
	buf[4] = typ
	copy(buf[5:], payload)
	_, err := w.Write(buf)
	return err
}

// ReadFrame reads one length-prefixed frame written by WriteFrame.
func ReadFrame(r *bufio.Reader) (typ byte, payload []byte, err error) {
	var lenBuf [4]byte
	if _, err = io.ReadFull(r, lenBuf[:]); err != nil {
		return 0, nil, err
	}
	n := binary.BigEndian.Uint32(lenBuf[:])
	if n == 0 || n > maxFrameLen {
		return 0, nil, fmt.Errorf("proto: invalid frame length %d", n)
	}
	body := make([]byte, n)
	if _, err = io.ReadFull(r, body); err != nil {
		return 0, nil, err
	}
	return body[0], body[1:], nil
}

// DiffWindow is one window's metadata, always present, plus its cell
// grid, present only when it changed since the last DiffFrame sent to
// this specific connection — Cells is nil for a window whose content the
// receiver should keep reusing from its own cache. Metadata (position,
// size, title, focus, …) is cheap enough to resend every frame; the cell
// grid is the part worth skipping.
type DiffWindow struct {
	ID         string
	Title      string
	X, Y       int
	W, H, Z    int
	Focused    bool
	Maximized  bool
	Kind       string
	Cols, Rows int
	Cells      []cell.Cell // nil if unchanged this frame
}

// DiffFrame is the binary replacement for a JSON Snapshot.
type DiffFrame struct {
	Cols, Rows int
	Windows    []DiffWindow
}

// EncodeDiffFrame serializes f to the FrameDiff binary payload.
func EncodeDiffFrame(f DiffFrame) []byte {
	var buf bytes.Buffer
	putUint16(&buf, uint16(clampUint16(f.Cols)))
	putUint16(&buf, uint16(clampUint16(f.Rows)))
	putUint16(&buf, uint16(len(f.Windows)))
	for _, w := range f.Windows {
		putString8(&buf, w.ID)
		putString8(&buf, w.Title)
		putInt16(&buf, int16(clampInt16(w.X)))
		putInt16(&buf, int16(clampInt16(w.Y)))
		putUint16(&buf, uint16(clampUint16(w.W)))
		putUint16(&buf, uint16(clampUint16(w.H)))
		putUint16(&buf, uint16(clampUint16(w.Z)))
		var flags byte
		if w.Focused {
			flags |= 1
		}
		if w.Maximized {
			flags |= 2
		}
		buf.WriteByte(flags)
		putString8(&buf, w.Kind)
		putUint16(&buf, uint16(clampUint16(w.Cols)))
		putUint16(&buf, uint16(clampUint16(w.Rows)))
		if w.Cells == nil {
			buf.WriteByte(0)
			continue
		}
		buf.WriteByte(1)
		putUint32(&buf, uint32(len(w.Cells)))
		for _, c := range w.Cells {
			putUint32(&buf, uint32(c.Rune))
			buf.WriteByte(c.FG.R)
			buf.WriteByte(c.FG.G)
			buf.WriteByte(c.FG.B)
			buf.WriteByte(c.BG.R)
			buf.WriteByte(c.BG.G)
			buf.WriteByte(c.BG.B)
			putUint16(&buf, uint16(c.Attr))
		}
	}
	return buf.Bytes()
}

// DecodeDiffFrame parses a FrameDiff payload produced by EncodeDiffFrame.
func DecodeDiffFrame(data []byte) (DiffFrame, error) {
	r := &byteReader{buf: data}
	var f DiffFrame
	f.Cols = int(r.uint16())
	f.Rows = int(r.uint16())
	n := r.uint16()
	if r.err != nil {
		return f, r.err
	}
	f.Windows = make([]DiffWindow, 0, n)
	for i := uint16(0); i < n; i++ {
		var w DiffWindow
		w.ID = r.string8()
		w.Title = r.string8()
		w.X = int(r.int16())
		w.Y = int(r.int16())
		w.W = int(r.uint16())
		w.H = int(r.uint16())
		w.Z = int(r.uint16())
		flags := r.byte_()
		w.Focused = flags&1 != 0
		w.Maximized = flags&2 != 0
		w.Kind = r.string8()
		w.Cols = int(r.uint16())
		w.Rows = int(r.uint16())
		hasCells := r.byte_()
		if r.err != nil {
			return f, r.err
		}
		if hasCells == 1 {
			count := r.uint32()
			cells := make([]cell.Cell, count)
			for j := uint32(0); j < count; j++ {
				var c cell.Cell
				c.Rune = rune(r.uint32())
				c.FG.R = r.byte_()
				c.FG.G = r.byte_()
				c.FG.B = r.byte_()
				c.BG.R = r.byte_()
				c.BG.G = r.byte_()
				c.BG.B = r.byte_()
				c.Attr = cell.Attr(r.uint16())
				cells[j] = c
			}
			if r.err != nil {
				return f, r.err
			}
			w.Cells = cells
		}
		f.Windows = append(f.Windows, w)
	}
	if r.err != nil {
		return f, r.err
	}
	return f, nil
}

// EncodeAudioChunk serializes interleaved int16 PCM samples (as captured
// by internal/audiocap, at its fixed SampleRate/Channels) into a FrameAudio
// payload — raw little-endian bytes, no header, since the frame envelope
// already carries type and length.
func EncodeAudioChunk(samples []int16) []byte {
	b := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(b[i*2:], uint16(s))
	}
	return b
}

// DecodeAudioChunk reverses EncodeAudioChunk.
func DecodeAudioChunk(data []byte) []int16 {
	out := make([]int16, len(data)/2)
	for i := range out {
		out[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return out
}

// --- small encode helpers ---

func putUint16(buf *bytes.Buffer, v uint16) {
	var b [2]byte
	binary.BigEndian.PutUint16(b[:], v)
	buf.Write(b[:])
}

func putInt16(buf *bytes.Buffer, v int16) {
	putUint16(buf, uint16(v))
}

func putUint32(buf *bytes.Buffer, v uint32) {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], v)
	buf.Write(b[:])
}

// putString8 writes a length-prefixed (1 byte, so 0-255 bytes) string.
// Window IDs/titles/kinds are all short by construction elsewhere in the
// codebase; a longer title is simply truncated rather than growing the
// length field for a case that shouldn't happen in practice.
func putString8(buf *bytes.Buffer, s string) {
	b := []byte(s)
	if len(b) > 255 {
		b = b[:255]
	}
	buf.WriteByte(byte(len(b)))
	buf.Write(b)
}

func clampUint16(v int) int {
	if v < 0 {
		return 0
	}
	if v > 0xFFFF {
		return 0xFFFF
	}
	return v
}

func clampInt16(v int) int {
	if v < -0x8000 {
		return -0x8000
	}
	if v > 0x7FFF {
		return 0x7FFF
	}
	return v
}

// --- small decode helpers ---

// byteReader is a minimal cursor over a []byte for DecodeDiffFrame — a
// bytes.Reader plus encoding/binary.Read would work too, but a plain
// cursor with a sticky error avoids repetitive err-checking at every
// field for a format this small and fully under this package's control.
type byteReader struct {
	buf []byte
	pos int
	err error
}

func (r *byteReader) need(n int) bool {
	if r.err != nil {
		return false
	}
	if r.pos+n > len(r.buf) {
		r.err = fmt.Errorf("proto: unexpected end of diff frame")
		return false
	}
	return true
}

func (r *byteReader) byte_() byte {
	if !r.need(1) {
		return 0
	}
	b := r.buf[r.pos]
	r.pos++
	return b
}

func (r *byteReader) uint16() uint16 {
	if !r.need(2) {
		return 0
	}
	v := binary.BigEndian.Uint16(r.buf[r.pos:])
	r.pos += 2
	return v
}

func (r *byteReader) int16() int16 {
	return int16(r.uint16())
}

func (r *byteReader) uint32() uint32 {
	if !r.need(4) {
		return 0
	}
	v := binary.BigEndian.Uint32(r.buf[r.pos:])
	r.pos += 4
	return v
}

func (r *byteReader) string8() string {
	n := int(r.byte_())
	if !r.need(n) {
		return ""
	}
	s := string(r.buf[r.pos : r.pos+n])
	r.pos += n
	return s
}
