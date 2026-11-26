package netutil

import "testing"

func TestListenAddress(t *testing.T) {
	for _, tc := range []struct {
		host string
		want string
	}{
		{"*", ":2106"},
		{"", "127.0.0.1:2106"},
		{"10.0.0.5", "10.0.0.5:2106"},
		{"login.example", "login.example:2106"},
		{"::1", "[::1]:2106"},
	} {
		if got := ListenAddress(tc.host, 2106); got != tc.want {
			t.Errorf("ListenAddress(%q, 2106) = %q, want %q", tc.host, got, tc.want)
		}
	}
}
