package crypt

import "testing"

func TestUnscrambleModulusRoundTrip(t *testing.T) {
	pair, err := NewLoginKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	pub, err := UnscrambleModulus(pair.ScrambledModulus)
	if err != nil {
		t.Fatal(err)
	}
	if pub.N.Cmp(pair.Private.N) != 0 || pub.E != pair.Private.E {
		t.Fatalf("unscrambled key (N=%x, E=%d) differs from the generated public key", pub.N, pub.E)
	}
	if _, err := UnscrambleModulus(pair.ScrambledModulus[1:]); err == nil {
		t.Fatal("UnscrambleModulus accepted a 127-byte modulus")
	}
}
