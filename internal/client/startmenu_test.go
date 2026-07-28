package client

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

// newTestMenuClient builds a Client with a hand-crafted two-root-item menu
// (mirroring buildStartMenu's shape) without needing a screen or server:
// root[0] "Sub" has two leaf children, root[1] "Leaf" is directly
// activatable and records that it ran.
func newTestMenuClient() (c *Client, leafRan *bool) {
	leafRan = new(bool)
	c = &Client{
		subOpen: -1,
		menuRoot: []startMenuItem{
			{Label: "Sub", Sub: []startMenuItem{
				{Label: "Child A"},
				{Label: "Child B"},
			}},
			{Label: "Leaf", Do: func() { *leafRan = true }},
		},
	}
	return c, leafRan
}

func keyEvent(k tcell.Key, mod tcell.ModMask) *tcell.EventKey {
	return tcell.NewEventKey(k, 0, mod)
}

func TestHandleStartMenuKeyNavigatesRoot(t *testing.T) {
	c, _ := newTestMenuClient()
	if c.menuIdx != 0 {
		t.Fatalf("initial menuIdx = %d, want 0", c.menuIdx)
	}
	c.handleStartMenuKey(keyEvent(tcell.KeyDown, tcell.ModNone))
	if c.menuIdx != 1 {
		t.Fatalf("menuIdx after Down = %d, want 1", c.menuIdx)
	}
	c.handleStartMenuKey(keyEvent(tcell.KeyDown, tcell.ModNone))
	if c.menuIdx != 1 {
		t.Fatalf("menuIdx after Down past the end = %d, want clamped 1", c.menuIdx)
	}
	c.handleStartMenuKey(keyEvent(tcell.KeyUp, tcell.ModNone))
	if c.menuIdx != 0 {
		t.Fatalf("menuIdx after Up = %d, want 0", c.menuIdx)
	}
	c.handleStartMenuKey(keyEvent(tcell.KeyUp, tcell.ModNone))
	if c.menuIdx != 0 {
		t.Fatalf("menuIdx after Up past the start = %d, want clamped 0", c.menuIdx)
	}
}

func TestHandleStartMenuKeyOpensAndClosesSubmenu(t *testing.T) {
	c, _ := newTestMenuClient()
	c.handleStartMenuKey(keyEvent(tcell.KeyRight, tcell.ModNone))
	if c.subOpen != 0 {
		t.Fatalf("subOpen after Right on a Sub item = %d, want 0", c.subOpen)
	}
	if c.subIdx != 0 {
		t.Fatalf("subIdx after opening submenu = %d, want 0", c.subIdx)
	}

	c.handleStartMenuKey(keyEvent(tcell.KeyDown, tcell.ModNone))
	if c.subIdx != 1 {
		t.Fatalf("subIdx after Down inside submenu = %d, want 1", c.subIdx)
	}

	c.handleStartMenuKey(keyEvent(tcell.KeyLeft, tcell.ModNone))
	if c.subOpen != -1 {
		t.Fatalf("subOpen after Left = %d, want -1 (closed)", c.subOpen)
	}
}

func TestHandleStartMenuKeyRightOnLeafIsNoop(t *testing.T) {
	c, _ := newTestMenuClient()
	c.menuIdx = 1 // "Leaf" has no Sub
	c.handleStartMenuKey(keyEvent(tcell.KeyRight, tcell.ModNone))
	if c.subOpen != -1 {
		t.Fatalf("subOpen after Right on a leaf item = %d, want -1 (unopened)", c.subOpen)
	}
}

func TestHandleStartMenuKeyEscapeClosesSubmenuFirstThenMenu(t *testing.T) {
	c, _ := newTestMenuClient()
	c.menuOpen = true
	c.openSubmenuAt(0)
	c.handleStartMenuKey(keyEvent(tcell.KeyEscape, tcell.ModNone))
	if c.subOpen != -1 {
		t.Fatalf("first Escape should close the submenu, subOpen = %d", c.subOpen)
	}
	if !c.menuOpen {
		t.Fatal("first Escape should not close the whole menu yet")
	}
	c.handleStartMenuKey(keyEvent(tcell.KeyEscape, tcell.ModNone))
	if c.menuOpen {
		t.Fatal("second Escape should close the whole menu")
	}
}

func TestOpenSubmenuAtBounds(t *testing.T) {
	c, _ := newTestMenuClient()
	c.openSubmenuAt(-1)
	if c.subOpen != -1 {
		t.Fatalf("openSubmenuAt(-1) mutated subOpen to %d", c.subOpen)
	}
	c.openSubmenuAt(99)
	if c.subOpen != -1 {
		t.Fatalf("openSubmenuAt(out of range) mutated subOpen to %d", c.subOpen)
	}
	c.openSubmenuAt(1) // "Leaf" has no Sub
	if c.subOpen != -1 {
		t.Fatalf("openSubmenuAt(leaf without Sub) mutated subOpen to %d", c.subOpen)
	}
	c.openSubmenuAt(0)
	if c.subOpen != 0 {
		t.Fatalf("openSubmenuAt(0) subOpen = %d, want 0", c.subOpen)
	}
}

func TestActivateStartMenuRootLeaf(t *testing.T) {
	c, leafRan := newTestMenuClient()
	c.menuOpen = true
	c.menuIdx = 1
	c.activateStartMenu()
	if !*leafRan {
		t.Fatal("activateStartMenu on a root leaf did not call Do")
	}
	if c.menuOpen {
		t.Fatal("activating a leaf should close the menu")
	}
}

func TestActivateStartMenuOpensSubmenuInsteadOfRunning(t *testing.T) {
	c, _ := newTestMenuClient()
	c.menuOpen = true
	c.menuIdx = 0 // "Sub" has children, no Do
	c.activateStartMenu()
	if c.subOpen != 0 {
		t.Fatalf("activating a Sub item should open its submenu, subOpen = %d", c.subOpen)
	}
	if !c.menuOpen {
		t.Fatal("opening a submenu should not close the whole menu")
	}
}

func TestActivateStartMenuSubmenuLeaf(t *testing.T) {
	var ranChild bool
	c := &Client{
		menuOpen: true,
		subOpen:  -1,
		menuRoot: []startMenuItem{
			{Label: "Sub", Sub: []startMenuItem{
				{Label: "Child A", Do: func() { ranChild = true }},
			}},
		},
	}
	c.openSubmenuAt(0)
	c.subIdx = 0
	c.activateStartMenu()
	if !ranChild {
		t.Fatal("activateStartMenu on a submenu leaf did not call Do")
	}
	if c.menuOpen {
		t.Fatal("activating a submenu leaf should close the whole menu")
	}
}
