package cli

import "testing"

func TestNormalizeVersion(t *testing.T) {
	cases := map[string]string{
		"v0.1.0":  "0.1.0",
		"0.1.0":   "0.1.0",
		"  v1.2 ": "1.2",
		"":        "",
	}
	for in, want := range cases {
		if got := normalizeVersion(in); got != want {
			t.Errorf("normalizeVersion(%q)=%q want %q", in, got, want)
		}
	}
}

func TestResolveVersionPrefersLdflags(t *testing.T) {
	if got := resolveVersion("0.1.0"); got != "0.1.0" {
		t.Fatalf("got %q", got)
	}
	if got := resolveVersion("v0.1.0"); got != "0.1.0" {
		t.Fatalf("got %q", got)
	}
}
