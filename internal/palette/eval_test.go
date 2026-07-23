package palette

import "testing"

func TestEval(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"0xff * 16", "4080  (0xFF0)"},
		{"2**10", "1024  (0x400)"},
		{"(1+2)*3", "9  (0x9)"},
		{"0x10 | 0x01", "17  (0x11)"},
		{"8 >> 1", "4  (0x4)"},
	}
	for _, c := range cases {
		got, err := Eval(c.in)
		if err != nil {
			t.Fatalf("%s: %v", c.in, err)
		}
		if got != c.want {
			t.Fatalf("%s: got %q want %q", c.in, got, c.want)
		}
	}
}
