package palette

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"github.com/ttypedesk/ttypedesk/pkg/uwidth"
)

// Hit is one ranked result in the command palette.
type Hit struct {
	Title    string
	Subtitle string
	Icon     string
	Score    int
	Run      func()
}

// Win is a minimal window descriptor for focus hits.
type Win struct {
	ID, Title string
	Minimized bool
}

// Icon is a desktop shortcut.
type Icon struct {
	Label, Action, Glyph string
}

// Program is a user-registered launcher.
type Program struct {
	ID, Name, Action, Icon string
}

// Recipe is a user-defined phrase → action.
type Recipe struct {
	Match   string
	Action  string
	Confirm bool
}

// Env is everything providers need from the desk (no UI).
type Env struct {
	Query     string
	Max       int
	Launch    func(action string) error
	OpenPath  func(path string) error
	Focus     func(id string)
	OpenFind  func(query string)
	Quit      func()
	Notify    func(title, body string)
	CopyText   func(text string)
	ApplyTheme func(id string) // theme pack id: xp, scarlet, bumble, bubble, sprout
	// Confirm asks the user to confirm a dangerous recipe. Return false to abort.
	// Implementations may keep the palette open for a second Enter.
	Confirm func(prompt, action string) bool
	Windows    []Win
	Icons      []Icon
	Programs   []Program
	Recipes    []Recipe
	History    []string // recent queries, newest first
	AsciiIcons bool      // draw ASCII substitutes instead of emoji
}

// Search returns ranked hits for the current query.
func Search(env Env) []Hit {
	q := strings.TrimSpace(env.Query)
	max := env.Max
	if max <= 0 {
		max = 12
	}
	var hits []Hit
	if q == "" {
		hits = append(hits, historyHits(env)...)
		hits = append(hits, catalogHits(env, "")...)
		return applyIconMode(trim(hits, max), env.AsciiIcons)
	}
	low := strings.ToLower(q)
	verb, rest := splitVerb(low)

	switch verb {
	case "calculate", "calc", "eval":
		hits = append(hits, calcHits(env, rest)...)
	case "open", "launch", "start":
		hits = append(hits, openHits(env, rest)...)
		hits = append(hits, catalogHits(env, rest)...)
	case "find", "search":
		hits = append(hits, findHits(env, rest)...)
	case "run", "exec":
		hits = append(hits, shellHits(env, "run", rest)...)
	case "ssh":
		hits = append(hits, shellHits(env, "ssh", rest)...)
	case "focus", "window":
		hits = append(hits, windowHits(env, rest)...)
	case "quit", "exit":
		hits = append(hits, Hit{
			Title: "Quit TTYPE Desk", Icon: "⏻", Score: 100,
			Run: env.Quit,
		})
	default:
		if strings.HasPrefix(q, "=") {
			hits = append(hits, calcHits(env, strings.TrimSpace(q[1:]))...)
		} else if looksLikeExpr(q) {
			hits = append(hits, calcHits(env, q)...)
		}
		hits = append(hits, themeHits(env, low)...)
		hits = append(hits, recipeHits(env, low)...)
		hits = append(hits, catalogHits(env, low)...)
		hits = append(hits, windowHits(env, low)...)
		hits = append(hits, shellHits(env, "run", q)...)
		hits = append(hits, findHits(env, q)...)
	}

	return applyIconMode(trim(dedupe(hits), max), env.AsciiIcons)
}

// applyIconMode substitutes ASCII stand-ins for emoji icons when ascii is set.
func applyIconMode(hits []Hit, ascii bool) []Hit {
	if !ascii {
		return hits
	}
	for i := range hits {
		hits[i].Icon = uwidth.ASCIIIcon(hits[i].Icon, ascii)
	}
	return hits
}

func splitVerb(q string) (verb, rest string) {
	parts := strings.SplitN(q, " ", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], strings.TrimSpace(parts[1])
}

func themeHits(env Env, low string) []Hit {
	if env.ApplyTheme == nil {
		return nil
	}
	type pack struct {
		id, title, sub, keys string
	}
	packs := []pack{
		{"xp", "Apply XP theme", "classic Bliss hills", "xp theme bliss windows"},
		{"scarlet", "Apply Scarlet theme", "red & black heat", "scarlet red black crimson"},
		{"bumble", "Apply Bumble theme", "yellow black white hive", "bumble yellow honey bee hive"},
		{"bubble", "Apply Bubble theme", "layers of blue", "bubble blue azure aqua"},
		{"sprout", "Apply Sprout theme", "green face, blue & yellow pops", "sprout green face moss"},
	}
	var out []Hit
	for _, p := range packs {
		sc := fuzzyScore(low, p.keys+" theme wallpaper")
		if low == p.id || strings.HasPrefix(p.id+" theme", low) || strings.Contains(low, p.id) {
			if sc < 40 {
				sc = 90
			}
		}
		if strings.Contains(low, "theme") && sc < 30 {
			sc = 50
		}
		if sc < 20 {
			continue
		}
		id := p.id
		out = append(out, Hit{
			Title: p.title, Subtitle: p.sub, Icon: "🪟", Score: sc,
			Run: func() { env.ApplyTheme(id) },
		})
	}
	return out
}

func historyHits(env Env) []Hit {
	var out []Hit
	for i, h := range env.History {
		if i >= 5 {
			break
		}
		h := h
		out = append(out, Hit{
			Title: h, Subtitle: "recent", Icon: "⏱", Score: 90 - i,
			Run: func() {}, // client refills query
		})
	}
	return out
}

func catalogHits(env Env, filter string) []Hit {
	type item struct {
		title, sub, icon, action string
		score                      int
	}
	builtins := []item{
		{"Terminal", "open terminal", "💻", "terminal", 50},
		{"Files", "file manager", "📁", "files", 50},
		{"Settings", "control panel", "⚙️", "settings", 50},
		{"Notes", "notepad", "📝", "notes", 48},
		{"Calendar", "month view", "📅", "calendar", 48},
		{"Clock", "digital clock", "🕐", "clock", 40},
		{"Manual", "user guide", "📖", "manual", 45},
		{"Image Viewer", "graphics", "🖼", "image", 40},
		{"System folder", "Manual on disk", "📂", "files:system", 35},
	}
	var out []Hit
	for _, b := range builtins {
		sc := fuzzyScore(filter, b.title+" "+b.action+" "+b.sub)
		if filter != "" && sc < 10 {
			continue
		}
		if filter == "" {
			sc = b.score
		} else {
			sc += b.score / 5
		}
		action := b.action
		out = append(out, Hit{
			Title: b.title, Subtitle: b.sub, Icon: b.icon, Score: sc,
			Run: func() { _ = env.Launch(action) },
		})
	}
	for _, ic := range env.Icons {
		sc := fuzzyScore(filter, ic.Label+" "+ic.Action)
		if filter != "" && sc < 10 {
			continue
		}
		if filter == "" {
			sc = 30
		}
		label, action, glyph := ic.Label, ic.Action, ic.Glyph
		if glyph == "" {
			glyph = "■"
		}
		out = append(out, Hit{
			Title: label, Subtitle: "desktop · " + action, Icon: glyph, Score: sc + 5,
			Run: func() { _ = env.Launch(action) },
		})
	}
	for _, p := range env.Programs {
		sc := fuzzyScore(filter, p.Name+" "+p.ID+" "+p.Action)
		if filter != "" && sc < 10 {
			continue
		}
		if filter == "" {
			sc = 35
		}
		name, action, icon := p.Name, p.Action, p.Icon
		if action == "" {
			action = "prog:" + p.ID
		}
		if icon == "" {
			icon = "🚀"
		}
		out = append(out, Hit{
			Title: name, Subtitle: "program · " + action, Icon: icon, Score: sc + 8,
			Run: func() { _ = env.Launch(action) },
		})
	}
	return out
}

func openHits(env Env, rest string) []Hit {
	if rest == "" {
		return nil
	}
	// Path-like
	if strings.HasPrefix(rest, "/") || strings.HasPrefix(rest, "~") || strings.Contains(rest, "/") {
		path := expandPath(rest)
		return []Hit{{
			Title: "Open " + path, Subtitle: "path", Icon: "📂", Score: 95,
			Run: func() { _ = env.OpenPath(path) },
		}}
	}
	return nil
}

func findHits(env Env, rest string) []Hit {
	if rest == "" {
		return nil
	}
	q := rest
	var out []Hit
	out = append(out, Hit{
		Title: "Find in scrollback: " + q, Subtitle: "terminal find", Icon: "🔍", Score: 80,
		Run: func() {
			if env.OpenFind != nil {
				env.OpenFind(q)
			}
		},
	})
	home, _ := os.UserHomeDir()
	if home != "" {
		// Shallow name match under home (one level) + common dirs
		roots := []string{home, filepath.Join(home, "Documents"), filepath.Join(home, "Downloads"), filepath.Join(home, "Desktop")}
		needle := strings.ToLower(rest)
		for _, root := range roots {
			ents, err := os.ReadDir(root)
			if err != nil {
				continue
			}
			for _, e := range ents {
				name := e.Name()
				if !strings.Contains(strings.ToLower(name), needle) {
					continue
				}
				p := filepath.Join(root, name)
				out = append(out, Hit{
					Title: name, Subtitle: p, Icon: fileIcon(e.IsDir(), name), Score: 70,
					Run: func() { _ = env.OpenPath(p) },
				})
				if len(out) > 20 {
					return out
				}
			}
		}
	}
	return out
}

func fileIcon(isDir bool, name string) string {
	if isDir {
		return "📁"
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp":
		return "🖼"
	case ".md", ".txt":
		return "📄"
	default:
		return "📄"
	}
}

func windowHits(env Env, filter string) []Hit {
	var out []Hit
	for _, w := range env.Windows {
		sc := fuzzyScore(filter, w.Title+" "+w.ID)
		if filter != "" && sc < 8 {
			continue
		}
		if filter == "" {
			sc = 25
		}
		id, title := w.ID, w.Title
		sub := "focus window"
		if w.Minimized {
			sub = "restore window"
		}
		out = append(out, Hit{
			Title: title, Subtitle: sub, Icon: "🗔", Score: sc + 15,
			Run: func() {
				if env.Focus != nil {
					env.Focus(id)
				}
			},
		})
	}
	return out
}

func shellHits(env Env, kind, rest string) []Hit {
	rest = strings.TrimSpace(rest)
	if rest == "" && kind != "ssh" {
		return nil
	}
	switch kind {
	case "ssh":
		host := rest
		if host == "" {
			return []Hit{{
				Title: "ssh …", Subtitle: "type a host", Icon: "🔌", Score: 40, Run: func() {},
			}}
		}
		action := "pty:ssh " + host
		return []Hit{{
			Title: "ssh " + host, Subtitle: action, Icon: "🔌", Score: 90,
			Run: func() { _ = env.Launch(action) },
		}}
	default:
		// run command
		action := "pty:" + rest
		return []Hit{{
			Title: "Run " + rest, Subtitle: action, Icon: "💻", Score: 75,
			Run: func() { _ = env.Launch(action) },
		}}
	}
}

func recipeHits(env Env, low string) []Hit {
	var out []Hit
	for _, r := range env.Recipes {
		m := strings.ToLower(strings.TrimSpace(r.Match))
		if m == "" {
			continue
		}
		sc := fuzzyScore(low, m)
		if sc < 20 && low != m && !strings.HasPrefix(m, low) && !strings.Contains(m, low) {
			continue
		}
		if low == m || strings.HasPrefix(m, low) {
			sc = 100
		}
		action, confirm := r.Action, r.Confirm
		title := r.Match
		sub := "recipe · " + action
		icon := "⭐"
		if confirm {
			sub = "⚠ confirm · " + action
			icon = "⚠"
		}
		out = append(out, Hit{
			Title: title, Subtitle: sub, Icon: icon, Score: sc,
			Run: func() {
				if confirm && env.Confirm != nil {
					if !env.Confirm("Run "+title+"?", action) {
						return
					}
				}
				_ = env.Launch(action)
			},
		})
	}
	return out
}

func calcHits(env Env, expr string) []Hit {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return []Hit{{
			Title: "calculate …", Subtitle: "e.g. 0xff * 16", Icon: "🔢", Score: 50, Run: func() {},
		}}
	}
	v, err := Eval(expr)
	if err != nil {
		return []Hit{{
			Title: "calculate " + expr, Subtitle: err.Error(), Icon: "🔢", Score: 40, Run: func() {},
		}}
	}
	text := v
	return []Hit{{
		Title: "= " + text, Subtitle: expr, Icon: "🔢", Score: 100,
		Run: func() {
			if env.CopyText != nil {
				env.CopyText(text)
			}
			if env.Notify != nil {
				env.Notify("Calculate", expr+" = "+text)
			}
		},
	}}
}

func looksLikeExpr(q string) bool {
	if q == "" {
		return false
	}
	hasDigit := false
	hasOp := false
	for _, r := range q {
		if unicode.IsDigit(r) {
			hasDigit = true
		}
		switch r {
		case '+', '-', '*', '/', '%', '(', ')', '&', '|', '^', 'x', 'X':
			hasOp = true
		}
	}
	return hasDigit && hasOp
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	if p == "~" {
		home, _ := os.UserHomeDir()
		return home
	}
	return p
}

func fuzzyScore(filter, target string) int {
	filter = strings.ToLower(strings.TrimSpace(filter))
	target = strings.ToLower(target)
	if filter == "" {
		return 1
	}
	if target == filter {
		return 100
	}
	if strings.HasPrefix(target, filter) {
		return 80
	}
	if strings.Contains(target, filter) {
		return 50
	}
	return subseqScore(filter, target)
}

func subseqScore(filter, target string) int {
	fr := []rune(filter)
	tr := []rune(target)
	if len(fr) == 0 {
		return 1
	}
	j := 0
	for i := 0; i < len(tr) && j < len(fr); i++ {
		if unicode.ToLower(tr[i]) == fr[j] {
			j++
		}
	}
	if j < len(fr) {
		return 0
	}
	// denser matches score higher
	return 20 + (len(fr)*40)/max(1, len(tr))
}

func dedupe(hits []Hit) []Hit {
	seen := map[string]bool{}
	out := hits[:0]
	for _, h := range hits {
		key := h.Title + "|" + h.Subtitle
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, h)
	}
	return out
}

func trim(hits []Hit, max int) []Hit {
	// sort by score desc
	for i := 0; i < len(hits); i++ {
		for j := i + 1; j < len(hits); j++ {
			if hits[j].Score > hits[i].Score {
				hits[i], hits[j] = hits[j], hits[i]
			}
		}
	}
	if len(hits) > max {
		hits = hits[:max]
	}
	return hits
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// FormatInt formats calc results.
func FormatInt(n int64) string {
	if n < 0 {
		return strconv.FormatInt(n, 10)
	}
	return fmt.Sprintf("%d  (0x%X)", n, uint64(n))
}
