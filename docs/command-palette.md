# Universal command palette (design)

A single **type-to-run** overlay for TTYPE Desk. Distinctive vs “another Start menu”: one chord, fuzzy text, verbs that reach across apps, files, shell, and later system/media providers.

## Feel

```text
┌──────────────────────────────────────────┐
│ > play coheed█                           │
│   ▶ play “Coheed and Cambria – …”  Amp   │
│   ▶ open files: Music/Coheed             │
│   ▶ find “coheed” in scrollback          │
└──────────────────────────────────────────┘
```

DOS / Win9x-adjacent: solid bar, highlight row, no glassy cards. Recent queries at the top when the query is empty.

## Hotkey

| Binding | Role |
|---------|------|
| **Alt+Space** (legacy) | Start menu — often stolen by Guake/host; remappable |
| **F10** / **Ctrl+Esc** | Start menu (preferred defaults) |
| **Ctrl+Space** or **Ctrl+P** | Command palette |
| Remappable | Settings → Input (`hotkeys.palette`, `hotkeys.start_menu`, …) |

Optional setting: “Start opens palette instead of menu” for users who want one chord.

## Query shape

Loose English is fine; parse as `verb rest` when possible:

| Example | Intent |
|---------|--------|
| `open notes` | `LaunchAction("notes")` / FocusOrCreate |
| `settings` | Focus Settings |
| `find readme` | Files search or scrollback find with query |
| `calculate 0xff * 16` | Inline eval; show result; Enter copies |
| `= 2**10` | Same as calculate |
| `ssh server` | PTY: `ssh server` (from programs / known hosts later) |
| `run htop` / `pty:htop` | CreatePtyCmd |
| `wifi connect` | Provider stub → recipe or notify “no wifi provider” |
| `install doom` | Provider stub / user recipe (`apt install …` with confirm) |
| `play coheed` | Amp provider when Amp exists; else Files fuzzy under Music |

Unknown verbs: fuzzy-match against app titles, desktop icons, `programs`, open windows (“focus Terminal”).

## Architecture

```text
Palette
  ├─ UI (client overlay; not a Window unless we want persistence)
  ├─ Registry of Providers
  │    ├─ AppsProvider      (LaunchAction / FocusOrCreate)
  │    ├─ FilesProvider     (OpenPath, fuzzy under home)
  │    ├─ WindowsProvider   (focus / close / minimize)
  │    ├─ CalcProvider      (expr)
  │    ├─ ShellProvider     (ssh / run — confirm policy)
  │    └─ RecipeProvider    (~/.config/ttypedesk/recipes.json)
  └─ Ranker (prefix > fuzzy > recency)
```

Providers return `[]Hit{ Title, Subtitle, Icon, Run func() }`. Keep the WM free of apt/NetworkManager — those live in optional providers or user recipes.

## Config (sketch)

```json
{
  "hotkeys": { "palette": "ctrl+space" },
  "palette": {
    "start_opens_palette": false,
    "max_results": 12,
    "history": 20
  },
  "recipes": [
    { "match": "wifi connect", "action": "pty:nmtui" },
    { "match": "install doom", "action": "pty:bash", "args": ["-lc", "sudo apt install chocolate-doom"], "confirm": true }
  ]
}
```

## Phasing

1. ~~Overlay UI + Apps/Windows/Calc providers + hotkey~~  
2. ~~Files fuzzy + `find` / `open path`~~  
3. ~~Shell one-shots~~ (`run` / `ssh`)  
4. ~~Recipes + history~~ (in-session history; recipes in config)  
5. Amp / network providers as those features land  

## Non-goals (v1)

- Full NLP / LLM in-process  
- Replacing Start menu for everyone by default  
- Bundling distro network/package stacks inside the WM binary  
