// AT-SPI2 client: enough of the D-Bus accessibility protocol to walk an
// app's accessible object tree and pull text + screen-space bounding boxes
// out of it, for overlaying real characters onto the bridge's raster
// capture (see docs/gui-bridge.md). Hand-rolled directly on godbus rather
// than a wrapper library — there's no mature Go AT-SPI client to depend on.
//
// Spec references: GNOME devel-docs "AT-SPI2 D-Bus API", specifically the
// org.a11y.Bus, org.a11y.atspi.Accessible, org.a11y.atspi.Component, and
// org.a11y.atspi.Text interfaces.
package bridge

import (
	"fmt"

	"github.com/godbus/dbus/v5"
)

const (
	ifaceA11yBus    = "org.a11y.Bus"
	ifaceA11yStatus = "org.a11y.Status"
	ifaceAccessible = "org.a11y.atspi.Accessible"
	ifaceComponent  = "org.a11y.atspi.Component"
	ifaceText       = "org.a11y.atspi.Text"
	registryBus     = "org.a11y.atspi.Registry"
	rootPath        = dbus.ObjectPath("/org/a11y/atspi/accessible/root")
)

// registryRoot is the well-known entry point every registered app hangs
// off of as a child.
var registryRoot = nodeRef{Bus: registryBus, Path: rootPath}

// nodeRef identifies one accessible object: which bus connection owns it
// (accessible objects are spread across every AT-SPI-aware app's own bus
// connection, not centralized) plus its object path on that connection.
type nodeRef struct {
	Bus  string
	Path dbus.ObjectPath
}

// atspiClient holds the two D-Bus connections AT-SPI needs: the session
// bus (just used once, to ask where the real accessibility bus is) and the
// accessibility bus itself (used for everything else).
type atspiClient struct {
	sessConn *dbus.Conn
	a11yConn *dbus.Conn
}

// connectATSPI dials sessionBusAddr, asks it where the accessibility bus
// lives, and connects there too.
func connectATSPI(sessionBusAddr string) (*atspiClient, error) {
	sessConn, err := dbus.Connect(sessionBusAddr)
	if err != nil {
		return nil, fmt.Errorf("connect session bus: %w", err)
	}
	var a11yAddr string
	busObj := sessConn.Object(ifaceA11yBus, dbus.ObjectPath("/org/a11y/bus"))
	if err := busObj.Call(ifaceA11yBus+".GetAddress", 0).Store(&a11yAddr); err != nil {
		sessConn.Close()
		return nil, fmt.Errorf("%s.GetAddress: %w", ifaceA11yBus, err)
	}
	a11yConn, err := dbus.Connect(a11yAddr)
	if err != nil {
		sessConn.Close()
		return nil, fmt.Errorf("connect accessibility bus (%s): %w", a11yAddr, err)
	}
	return &atspiClient{sessConn: sessConn, a11yConn: a11yConn}, nil
}

// enable flips org.a11y.Status.IsEnabled (and ScreenReaderEnabled) on,
// which is what makes toolkits that gate their accessibility tree behind
// "is an AT client actually listening" (notably Chromium/Electron) start
// building one. Must be called before the guest app launches.
func (c *atspiClient) enable() error {
	statusObj := c.sessConn.Object(ifaceA11yBus, dbus.ObjectPath("/org/a11y/bus"))
	for _, prop := range []string{"IsEnabled", "ScreenReaderEnabled"} {
		call := statusObj.Call("org.freedesktop.DBus.Properties.Set", 0, ifaceA11yStatus, prop, dbus.MakeVariant(true))
		if call.Err != nil {
			return fmt.Errorf("set %s.%s: %w", ifaceA11yStatus, prop, call.Err)
		}
	}
	return nil
}

func (c *atspiClient) object(n nodeRef) dbus.BusObject {
	return c.a11yConn.Object(n.Bus, n.Path)
}

// children returns n's direct accessible children.
func (c *atspiClient) children(n nodeRef) ([]nodeRef, error) {
	var raw [][]interface{}
	call := c.object(n).Call(ifaceAccessible+".GetChildren", 0)
	if call.Err != nil {
		return nil, call.Err
	}
	if err := call.Store(&raw); err != nil {
		return nil, err
	}
	out := make([]nodeRef, 0, len(raw))
	for _, r := range raw {
		if len(r) != 2 {
			continue
		}
		bus, _ := r[0].(string)
		path, _ := r[1].(dbus.ObjectPath)
		if bus == "" || path == "" {
			continue
		}
		out = append(out, nodeRef{Bus: bus, Path: path})
	}
	return out, nil
}

// atspiRect is GetExtents' single (iiii) struct return value — not 4
// separate out-params (confirmed empirically against a real AT-SPI reply).
type atspiRect struct {
	X, Y, W, H int32
}

// atspiCoordWindow is AT-SPI2's CoordType enum value 1 ("relative to the
// object's top-level window"). CoordType 0 ("screen") was tried first and
// empirically returns (0,0) for every node in our headless, no-window-
// manager Xvfb — screen-position computation there depends on a WM
// providing placement, which we deliberately don't run. Window-relative
// coordinates work out equivalently for us anyway: each BridgeSurface gets
// its own dedicated Xvfb, so the guest's top-level window sits at the
// captured root window's origin.
const atspiCoordWindow = 1

// extents returns n's bounding box (window-relative — see atspiCoordWindow),
// or ok=false if n has no Component interface.
func (c *atspiClient) extents(n nodeRef) (x, y, w, h int32, ok bool) {
	call := c.object(n).Call(ifaceComponent+".GetExtents", 0, uint32(atspiCoordWindow))
	if call.Err != nil {
		return 0, 0, 0, 0, false
	}
	var rect atspiRect
	if err := call.Store(&rect); err != nil {
		return 0, 0, 0, 0, false
	}
	return rect.X, rect.Y, rect.W, rect.H, true
}

// text returns n's full text content, or ok=false if n has no Text
// interface (most nodes don't — only text-bearing widgets/labels do).
func (c *atspiClient) text(n nodeRef) (s string, ok bool) {
	call := c.object(n).Call(ifaceText+".GetText", 0, int32(0), int32(-1))
	if call.Err != nil {
		return "", false
	}
	if err := call.Store(&s); err != nil {
		return "", false
	}
	return s, true
}

// textNode is one accessible text-bearing widget: its content plus a
// window-relative pixel bounding box.
type textNode struct {
	Text       string
	X, Y, W, H int32
}

// atspiInvalidCoord is what GetExtents comes back with for accessible
// objects with no real on-screen position (observed empirically against
// gtk3-demo — hidden/zero-size internal widgets report INT32_MIN).
const atspiInvalidCoord = -2147483648

// walkText walks every registered app's accessible tree (in our isolated
// per-window bus there's normally exactly one) and collects every node
// that has both real text and a plausible bounding box. Depth-bounded
// against runaway/cyclic trees; maxNodes bounds total D-Bus round-trips so
// one pathologically large app tree can't stall the capture loop.
func (c *atspiClient) walkText(maxDepth, maxNodes int) []textNode {
	apps, err := c.children(registryRoot)
	if err != nil {
		return nil
	}
	var out []textNode
	visited := 0
	for _, app := range apps {
		c.walkTextInto(app, 0, maxDepth, maxNodes, &visited, &out)
		if visited >= maxNodes {
			break
		}
	}
	return out
}

func (c *atspiClient) walkTextInto(n nodeRef, depth, maxDepth, maxNodes int, visited *int, out *[]textNode) {
	if depth > maxDepth || *visited >= maxNodes {
		return
	}
	*visited++

	if s, ok := c.text(n); ok && s != "" {
		x, y, w, h, hasExt := c.extents(n)
		if hasExt && w > 0 && h > 0 && x > atspiInvalidCoord && y > atspiInvalidCoord {
			*out = append(*out, textNode{Text: s, X: x, Y: y, W: w, H: h})
		}
	}

	kids, err := c.children(n)
	if err != nil {
		return
	}
	for _, k := range kids {
		c.walkTextInto(k, depth+1, maxDepth, maxNodes, visited, out)
		if *visited >= maxNodes {
			return
		}
	}
}

func (c *atspiClient) close() {
	if c == nil {
		return
	}
	if c.a11yConn != nil {
		c.a11yConn.Close()
	}
	if c.sessConn != nil {
		c.sessConn.Close()
	}
}
