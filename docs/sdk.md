# App SDK (`pkg/uiapp`)

Native TTYPE Desk apps are **not** PTY wrappers. They implement `uiapp.App` and get a rich `Context` with shell **Host** services and optional **Hooks**.

User-registered programs (Start → System → **Add Program…**) run as `shell -c <command>` in a PTY window. Use the SDK when you need real UI, notifications, launches, or lifecycle beyond a terminal.

## App interface

```go
type App interface {
    Init(ctx *Context) error
    Handle(Event) error
    Draw(*Canvas) error
    Close() error
}
```

Events: `key`, `mouse`, `resize`, `focus` / `blur`, `timer`.

## Host services (`ctx`)

| Method | Purpose |
|--------|---------|
| `Notify(title, body, icon)` | System notification banner + tray |
| `Launch(action)` | Start-menu style actions (`terminal`, `prog:id`, `pty:htop`, …) |
| `OpenPath(path)` | Open file via associations / directory in Files |
| `SetTitle(title)` | Window title bar |
| `RequestClose()` | Close this window |
| `MarkDirty()` | Request redraw |
| `StartTimer(d)` | Periodic `EventTimer` |

## Hooks (`HookProvider`)

```go
func (a *App) Hooks() uiapp.Hooks {
    return uiapp.Hooks{
        OnInit:   func(ctx *uiapp.Context) { ctx.SetTitle("My App") },
        OnClose:  func(ctx *uiapp.Context) { /* persist */ },
        OnFocus:  func(ctx *uiapp.Context, focused bool) {},
        OnResize: func(ctx *uiapp.Context, cols, rows int) {},
        OnTimer:  func(ctx *uiapp.Context) {},
        OnDraw:   func(ctx *uiapp.Context, cv *uiapp.Canvas) {},
    }
}
```

## Canvas helpers

`FillRect`, `DrawText`, `DrawIcon`, `DrawBox`, `DrawButton` — width-aware via `pkg/uwidth`.

### Scrollbar (reusable)

```go
var s uiapp.ScrollState
s.Content, s.Viewport = len(items), visibleRows
s.EnsureVisible(sel)
cv.DrawScrollbar(cols-1, top, h, s, uiapp.DefaultScrollbarStyle())

hit := uiapp.HitScrollbar(mx, my, cols-1, top, h, s, true)
s.ApplyScrollHit(hit)
```

Mouse `Action: "wheel"` delivers `Button` as scroll delta (+up / −down).

### OpenPath

`ctx.OpenPath(path)` uses desk **associations** + **roles** (default editor `pty:nano`, images → Image Viewer). Unknown types notify: `No default app selected for file type: .<ext>`.

## Custom programs (no SDK)

Stored in config `programs[]`:

```json
{
  "id": "p123",
  "name": "htop",
  "command": "htop",
  "icon": "📊",
  "desktop": true
}
```

Launched as `prog:<id>` → `$SHELL -c <command>`. Icons come from an unused-emoji palette (Add Program UI).
