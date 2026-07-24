# App Store

Native app (`apps/appstore`) — Start ▸ App Store. A generic engine that fetches
a catalog from one or more configured GitHub repos, installs the entries you
pick, and registers their launchers into Start ▸ Programs once installed. No
app-specific logic lives in TTYPE Desk itself — the catalog and install
scripts live in the source repo.

Default (and reference) catalog:
[RetroCodeRamen/ttypedesk-apps](https://github.com/RetroCodeRamen/ttypedesk-apps)
— see that repo for the catalog format and the first entry (carbonyl
"Internet").

## How it works

1. On open, fetches `index.json` from each configured source
   (`https://raw.githubusercontent.com/<repo>/<branch>/index.json`).
2. For each entry, runs its `detect` check to see if it's already installed;
   detected entries are marked **Installed** and their launchers registered
   immediately (no re-download needed).
3. Enter/click an entry to install: downloads its install script, writes it
   to a temp file, and runs it via `pty:bash <path>` in a new terminal
   window — interactive and unsandboxed, same as running any command in a
   Terminal (it may prompt for a sudo password right there).
4. Every ~2s, re-runs `detect` on installing entries. Once it passes, the
   entry flips to **Installed**, its launchers are registered into
   `cfg.Programs`, and a notification fires.

## Controls

| Input | Action |
|-------|--------|
| ↑↓ / click | Select entry |
| Enter / Space | Install selected entry (or retry after an error) |
| Esc | Close |

## Catalog entry shape (`index.json`)

```json
{
  "apps": [
    {
      "id": "carbonyl",
      "name": "Internet",
      "description": "Carbonyl browser",
      "icon": "🌐",
      "detect": { "which": "carbonyl", "paths": ["~/.local/bin/carbonyl"] },
      "install": { "script": "install/carbonyl.sh" },
      "register": [
        { "id": "carbonyl", "name": "Internet", "command": "pty:carbonyl", "icon": "🌐", "menu": "programs" }
      ]
    }
  ]
}
```

- `detect.which` / `detect.paths` — either an `exec.LookPath` hit or a
  literal file match (`~` expanded) counts as installed; both are optional
  but at least one should be set or the entry never leaves "Not installed".
- `install.script` — path to a shell script relative to the repo root,
  fetched over `raw.githubusercontent.com` and run as-is.
- `register` — one or more Start-menu launchers to add once detected;
  matched against existing `cfg.Programs` by ID first, then by name, so
  repeat installs (or an app that was already present) don't duplicate
  entries.

## Config (`app_sources`)

```json
"app_sources": [
  { "repo": "RetroCodeRamen/ttypedesk-apps", "branch": "main" }
]
```

`branch` defaults to `main` if omitted. Add more objects to pull from
additional catalogs — later sources' entries are simply appended to the
list.

## Notes

- No sandboxing: install scripts run with the current user's shell
  permissions. Only add sources you trust.
- Network failures on a source are shown inline ("Couldn't reach a
  source: …") rather than blocking the other configured sources.
