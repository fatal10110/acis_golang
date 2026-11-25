package network

import "testing"

// TestPetNameClaims: a claimed name, in any ASCII case, is refused until it
// is released; other names stay free.
func TestPetNameClaims(t *testing.T) {
	var c petNameClaims
	if !c.claim("Fenrir") {
		t.Fatal("claim(Fenrir) on no claims = false")
	}
	if c.claim("Fenrir") || c.claim("fENRIR") {
		t.Fatal("claim of a claimed name = true")
	}
	if !c.claim("Rex") {
		t.Fatal("claim(Rex) = false while only Fenrir is claimed")
	}
	c.release("FENRIR")
	if !c.claim("fenrir") {
		t.Fatal("claim after release = false")
	}
}
