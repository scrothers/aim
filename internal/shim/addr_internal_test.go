package shim

import "testing"

func TestDefaultAddrPlatforms(t *testing.T) {
	t.Parallel()
	none := func(string) string { return "" }
	xdg := func(k string) string {
		if k == "XDG_RUNTIME_DIR" {
			return "/x"
		}
		return ""
	}
	tests := []struct {
		name string
		env  func(string) string
		goos string
		home string
		want string
	}{
		{"darwin without xdg", none, "darwin", "/Users/me", "/Users/me/.local/state/aim/aim.sock"},
		{"darwin with xdg", xdg, "darwin", "/Users/me", "/x/aim/aim.sock"},
		{"linux without xdg", none, "linux", "/home/me", "/run/user/1000/aim/aim.sock"},
		{"darwin without home", none, "darwin", "", "/run/user/1000/aim/aim.sock"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := defaultAddr(tc.env, tc.goos, tc.home, 1000); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
