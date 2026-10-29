package main

import "testing"

func TestDecideSandbox(t *testing.T) {
	cases := []struct {
		goos   string
		bwrap  bool
		strict bool
		want   sandboxAction
	}{
		{"linux", true, false, sandboxKeep},
		{"linux", true, true, sandboxKeep},
		{"linux", false, false, sandboxDisable},
		{"linux", false, true, sandboxRefuse},
		{"darwin", false, false, sandboxKeep},
		{"darwin", false, true, sandboxKeep},
		{"windows", false, true, sandboxKeep},
	}
	for _, c := range cases {
		if got := decideSandbox(c.goos, c.bwrap, c.strict); got != c.want {
			t.Fatalf("decideSandbox(%q, %v, %v) = %v，想要 %v",
				c.goos, c.bwrap, c.strict, got, c.want)
		}
	}
}
