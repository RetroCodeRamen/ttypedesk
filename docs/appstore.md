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
| Enter / Space | Install selected entry (or retry after an error) — see Trust below |
| Esc | Close (or cancel a pending trust confirmation) |

## Trust / warning

Install scripts run unsandboxed and may invoke `sudo` — there's no
sandboxing between the script and your account. A source's entries show
`⚠ unverified source` until you trust it:

- The **first** Enter/Space/click on an entry from an untrusted source arms
  a warning in the status bar instead of installing.
- A **second** Enter/Space/click on the same entry trusts that source —
  persisted to `app_sources[].trusted` in config — and proceeds with the
  install. Every other entry from that source is trusted too from then on.
- Esc while the warning is armed cancels it without installing.

The default catalog (`RetroCodeRamen/ttypedesk-apps`) ships trusted by
default; any source you add by hand gets the warning once.

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
  matched against existing `cfg.Programs` by ID, so repeat installs (or a
  re-detect on a later App Store open) update the existing entry in place
  instead of duplicating it. A brand new entry never takes over some other
  existing program just because they share a display Name — if `name` is
  already in use by an unrelated program, it's suffixed `-store` (then
  `-store-2`, …) so both coexist in the Start menu.

## Naming vs. upstream

`name` is whatever should show in TTYPE Desk's Start menu — it doesn't have
to match the upstream project's name. E.g. an entry for a system monitor
whose upstream project is called `btop` can register as `"name": "Task
Manager"` with `"menu": "system"`; the App Store still detects/installs the
real `btop` binary under the hood.

## Config (`app_sources`)

```json
"app_sources": [
  { "repo": "RetroCodeRamen/ttypedesk-apps", "branch": "main", "trusted": true }
]
```

`branch` defaults to `main` if omitted. `trusted` defaults to `false` (see
Trust above) — set it by hand to skip the in-app confirmation, or just
confirm once in the App Store and let it persist itself. Add more objects
to pull from additional catalogs — later sources' entries are simply
appended to the list.

## Notes

- No sandboxing: install scripts run with the current user's shell
  permissions. Trusting a source is a one-time gate, not a sandbox — only
  trust sources you'd run a script from directly.
- Network failures on a source are shown inline ("Couldn't reach a
  source: …") rather than blocking the other configured sources.
