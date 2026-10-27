package cipher

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"
)

// TestCipherMatchesReferenceProbe replays testdata/cipher.golden, the output
// of testdata/oracle/CipherProbe.java driving the unmodified reference cipher:
// each "key" line starts a fresh cipher, and every "enc"/"dec" line must
// transform its input exactly as the reference did at that point of the
// sequence (arming, no-op decrypt before arming, key roll).
func TestCipherMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/cipher.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	var c *Cipher
	ops := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<16)
	for line := 1; sc.Scan(); line++ {
		fields := strings.Fields(sc.Text())
		switch {
		case len(fields) == 2 && fields[0] == "key":
			if c, err = NewCipher(mustHex(fields[1])); err != nil {
				t.Fatal(err)
			}
		case len(fields) == 3 && c != nil && (fields[0] == "enc" || fields[0] == "dec"):
			buf, want := mustHex(fields[1]), mustHex(fields[2])
			if fields[0] == "enc" {
				c.Encrypt(buf)
			} else {
				c.Decrypt(buf)
			}
			if !bytes.Equal(buf, want) {
				t.Fatalf("line %d %s: got %x, want %x", line, fields[0], buf, want)
			}
			ops++
		default:
			t.Fatalf("malformed golden line %d %q", line, sc.Text())
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if ops == 0 {
		t.Fatal("golden holds no operations")
	}
}
