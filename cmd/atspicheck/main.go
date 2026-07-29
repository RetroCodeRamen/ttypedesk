// atspicheck is the Phase G1 spike: walks a target app's AT-SPI accessible
// tree and prints whatever text + bounding-box data comes back, to answer
// one question before any integration work — does this actually retrieve
// real, correctly-positioned text? See docs/gui-bridge.md and the plan
// this was built against.
//
// Self-contained (doesn't import internal/bridge) since this predates and
// is independent of the eventual BridgeSurface integration — it's meant to
// validate the AT-SPI mechanism on its own.
//
// Usage: DBUS_SESSION_BUS_ADDRESS must already point at a session bus with
// at-spi2-registryd running and the target app launched against it (see
// the accompanying shell setup used during the spike).
package main

import (
	"fmt"
	"os"

	"github.com/godbus/dbus/v5"
)

const (
	ifaceA11yBus    = "org.a11y.Bus"
	ifaceA11yStatus = "org.a11y.Status"
	ifaceAccessible = "org.a11y.atspi.Accessible"
	ifaceComponent  = "org.a11y.atspi.Component"
	ifaceText       = "org.a11y.atspi.Text"
)

type nodeRef struct {
	Bus  string
	Path dbus.ObjectPath
}

func main() {
	sessAddr := os.Getenv("DBUS_SESSION_BUS_ADDRESS")
	if sessAddr == "" {
		fatal("DBUS_SESSION_BUS_ADDRESS not set")
	}

	sessConn, err := dbus.Connect(sessAddr)
	if err != nil {
		fatal(fmt.Sprintf("connect session bus: %v", err))
	}
	defer sessConn.Close()

	var a11yAddr string
	busObj := sessConn.Object(ifaceA11yBus, dbus.ObjectPath("/org/a11y/bus"))
	if err := busObj.Call(ifaceA11yBus+".GetAddress", 0).Store(&a11yAddr); err != nil {
		fatal(fmt.Sprintf("%s.GetAddress: %v", ifaceA11yBus, err))
	}
	fmt.Println("ok: accessibility bus address =", a11yAddr)

	for _, prop := range []string{"IsEnabled", "ScreenReaderEnabled"} {
		call := busObj.Call("org.freedesktop.DBus.Properties.Set", 0, ifaceA11yStatus, prop, dbus.MakeVariant(true))
		if call.Err != nil {
			fmt.Printf("WARN: set %s.%s: %v\n", ifaceA11yStatus, prop, call.Err)
		} else {
			fmt.Printf("ok: %s.%s = true\n", ifaceA11yStatus, prop)
		}
	}

	a11yConn, err := dbus.Connect(a11yAddr)
	if err != nil {
		fatal(fmt.Sprintf("connect accessibility bus: %v", err))
	}
	defer a11yConn.Close()

	// The registry daemon owns the well-known name org.a11y.atspi.Registry
	// and exposes every registered app as a child of its root object.
	root := nodeRef{Bus: "org.a11y.atspi.Registry", Path: "/org/a11y/atspi/accessible/root"}
	fmt.Printf("-- walking from registry root (%s %s) --\n", root.Bus, root.Path)

	apps, err := children(a11yConn, root)
	if err != nil {
		fatal(fmt.Sprintf("GetChildren on registry root: %v", err))
	}
	fmt.Printf("ok: %d registered app(s)\n", len(apps))
	if len(apps) == 0 {
		fmt.Println("FAIL: no apps registered with AT-SPI — nothing to walk")
		os.Exit(1)
	}

	totalText := 0
	for _, app := range apps {
		fmt.Printf("== app: bus=%s path=%s ==\n", app.Bus, app.Path)
		totalText += walk(a11yConn, app, 0, 12)
	}
	fmt.Printf("-- done: %d text node(s) found across %d app(s) --\n", totalText, len(apps))
	if totalText == 0 {
		fmt.Println("FAIL: zero text nodes found")
		os.Exit(1)
	}
	fmt.Println("PASS")
}

func walk(conn *dbus.Conn, n nodeRef, depth, maxDepth int) int {
	if depth > maxDepth {
		return 0
	}
	found := 0
	indent := ""
	for i := 0; i < depth; i++ {
		indent += "  "
	}

	debug := os.Getenv("ATSPI_DEBUG") != ""
	role, _ := roleName(conn, n)
	s, hasText := text(conn, n)
	if debug {
		fmt.Printf("%s[node role=%q hasText=%v len=%d]\n", indent, role, hasText, len(s))
	}
	if hasText && s != "" {
		x0, y0, w0, h0, ok0 := extentsCT(conn, n, 0)
		x1, y1, w1, h1, ok1 := extentsCT(conn, n, 1)
		fmt.Printf("%stext %q  screen(ok=%v)=(%d,%d %dx%d)  window(ok=%v)=(%d,%d %dx%d)\n",
			indent, truncate(s, 40), ok0, x0, y0, w0, h0, ok1, x1, y1, w1, h1)
		found++
	}

	kids, err := children(conn, n)
	if err != nil {
		return found
	}
	for _, k := range kids {
		found += walk(conn, k, depth+1, maxDepth)
	}
	return found
}

func children(conn *dbus.Conn, n nodeRef) ([]nodeRef, error) {
	var raw [][]interface{}
	call := conn.Object(n.Bus, n.Path).Call(ifaceAccessible+".GetChildren", 0)
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
// separate out-params, discovered empirically (a naive 4-out-param Store
// failed with "length mismatch" against the real reply body).
type atspiRect struct {
	X, Y, W, H int32
}

func extentsCT(conn *dbus.Conn, n nodeRef, coordType uint32) (x, y, w, h int32, ok bool) {
	call := conn.Object(n.Bus, n.Path).Call(ifaceComponent+".GetExtents", 0, coordType)
	if call.Err != nil {
		if os.Getenv("ATSPI_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG extents error: %v\n", call.Err)
		}
		return 0, 0, 0, 0, false
	}
	var rect atspiRect
	if err := call.Store(&rect); err != nil {
		if os.Getenv("ATSPI_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "DEBUG extents decode error: %v (body=%v)\n", err, call.Body)
		}
		return 0, 0, 0, 0, false
	}
	return rect.X, rect.Y, rect.W, rect.H, true
}

func roleName(conn *dbus.Conn, n nodeRef) (string, error) {
	var s string
	call := conn.Object(n.Bus, n.Path).Call(ifaceAccessible+".GetRoleName", 0)
	if call.Err != nil {
		return "", call.Err
	}
	err := call.Store(&s)
	return s, err
}

func text(conn *dbus.Conn, n nodeRef) (string, bool) {
	var s string
	call := conn.Object(n.Bus, n.Path).Call(ifaceText+".GetText", 0, int32(0), int32(-1))
	if call.Err != nil {
		return "", false
	}
	if err := call.Store(&s); err != nil {
		return "", false
	}
	return s, true
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func fatal(msg string) {
	fmt.Fprintln(os.Stderr, "FAIL:", msg)
	os.Exit(1)
}
