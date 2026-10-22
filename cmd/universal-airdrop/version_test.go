package main

import "testing"

func TestPlusRecente(t *testing.T) {
	cas := []struct {
		a, b string
		vrai bool
	}{
		{"1.0.10", "1.0.9", true}, {"1.0.9", "1.0.10", false}, {"1.1", "1.0.9", true},
		{"1.0.7", "1.0.7", false}, {"dev", "1.0.7", false}, {"1.0.7", "dev", false}, {"v2.0.0", "1.9.9", true},
	}
	for _, c := range cas {
		if got := plusRecente(c.a, c.b); got != c.vrai {
			t.Errorf("plusRecente(%q, %q) = %v", c.a, c.b, got)
		}
	}
}
