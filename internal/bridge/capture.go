package bridge

import (
	"fmt"
	"image"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

// captureFrame reads the current contents of root via GetImage and decodes
// it into an image.Image. Xvfb's default TrueColor visual is 24/32bpp
// ZPixmap; byteOrder (from the connection setup, LSBFirst=0/MSBFirst=1)
// says how the four bytes per pixel are laid out.
func captureFrame(conn *xgb.Conn, root xproto.Window, w, h int, byteOrder byte) (image.Image, error) {
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid capture size %dx%d", w, h)
	}
	reply, err := xproto.GetImage(conn, xproto.ImageFormatZPixmap, xproto.Drawable(root),
		0, 0, uint16(w), uint16(h), 0xffffffff).Reply()
	if err != nil {
		return nil, fmt.Errorf("GetImage: %w", err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	data := reply.Data
	need := w * h * 4
	if len(data) < need {
		return nil, fmt.Errorf("GetImage returned %d bytes, want at least %d", len(data), need)
	}
	for i := 0; i < w*h; i++ {
		px := data[i*4 : i*4+4]
		var r, g, b byte
		if byteOrder == 1 { // MSBFirst: bytes are X,R,G,B
			r, g, b = px[1], px[2], px[3]
		} else { // LSBFirst (the common case): bytes are B,G,R,X
			r, g, b = px[2], px[1], px[0]
		}
		o := i * 4
		img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = r, g, b, 0xff
	}
	return img, nil
}
