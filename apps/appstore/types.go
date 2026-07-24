package appstore

// RemoteEntry is one installable app, as described by a source repo's
// index.json. See https://github.com/RetroCodeRamen/ttypedesk-apps for the
// format and an example.
type RemoteEntry struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Icon        string          `json:"icon"`
	Detect      DetectSpec      `json:"detect"`
	Install     InstallSpec     `json:"install"`
	Register    []RegisterEntry `json:"register"`
}

// DetectSpec describes how to check whether an entry is already installed.
// Which and Paths are independent checks — either matching counts as
// installed.
type DetectSpec struct {
	Which string   `json:"which"` // exec.LookPath check, optional
	Paths []string `json:"paths"` // literal file paths ("~" expanded), optional
}

// InstallSpec points at the install script within the source repo.
type InstallSpec struct {
	Script string `json:"script"` // path relative to the repo root
}

// RegisterEntry is a Start-menu launcher to add once an app is detected as
// installed. Shape mirrors config.Program.
type RegisterEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Icon    string `json:"icon"`
	Menu    string `json:"menu"`
}

// catalogIndex is the root shape of a source repo's index.json.
type catalogIndex struct {
	Apps []RemoteEntry `json:"apps"`
}
