package config

import "testing"

func TestExpandLaunch(t *testing.T) {
	cfg := Default()
	cfg.Roles.Browser = "pty:lynx"

	cases := []struct {
		in, want string
	}{
		{"notes", "notes"},
		{"role:editor", "pty:nano"},
		{"editor", "pty:nano"},
		{"browser", "pty:lynx"},
		{"role:browser", "pty:lynx"},
		{"open {editor}", "open pty:nano"},
		{"{browser}", "pty:lynx"},
		{"role:filemgr", "files"},
	}
	for _, tc := range cases {
		got := cfg.ExpandLaunch(tc.in)
		if got != tc.want {
			t.Errorf("ExpandLaunch(%q)=%q want %q", tc.in, got, tc.want)
		}
	}

	cfg.Roles.Browser = ""
	if got := cfg.ExpandLaunch("browser"); got != "" {
		t.Errorf("unset browser should expand empty, got %q", got)
	}
}
