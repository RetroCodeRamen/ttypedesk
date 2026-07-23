# Image wallpaper (design)

Convert a user image into a **block-based color wallpaper** for the desktop field (behind windows/icons).

## Approach

Reuse `internal/gfx` half-block encoding (`▄`, bg=top / fg=bottom):

1. Load PNG/JPEG/GIF (already supported in imageview).
2. Scale to desktop size: `cols × (rows-taskbar) * 2` vertical samples.
3. Encode once into a cell grid; cache until resize or wallpaper path changes.
4. On full layout redraw, blit wallpaper cells instead of solid/pattern fill.
5. Desktop icons draw **on top** of wallpaper.

## Config

```json
{
  "wallpaper": {
    "mode": "solid" | "pattern" | "image",
    "path": "builtin:bliss" | "/path/to/image.png",
    "fit": "cover" | "contain" | "stretch"
  }
}
```

Builtin tokens (theme packs): `builtin:bliss`, `builtin:scarlet`, `builtin:bumble`, `builtin:bubble`, `builtin:sprout`. Applying a theme pack from Settings → Appearance or the command palette sets both chrome colors and the matching wallpaper.

SSH: optional `wallpaper.ssh_mode: "solid"` to avoid pushing a full-color frame often (or keep cached image wallpaper — it’s only redrawn on layout dirty).

## Settings UI

Appearance → Wallpaper → mode / typed path / **Browse wallpaper in Files…** / fit / **Theme: …** rows (XP, Scarlet, Bumble, Bubble, Sprout).

In **Files**: select a PNG/JPEG/GIF → press **W** or toolbar **Wall**. Applies immediately (saved to config); Bliss remains the default until you pick something else.

## Pipeline

Not redrawn every frame: decode image once, encode to half-block cells when path/fit/desktop size changes, blit from cache on layout redraw.