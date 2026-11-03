package crypt

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestCryptMatchesReferenceProbe checks every line of testdata/crypt.golden,
// the output of testdata/oracle/CryptProbe.java calling the unmodified
// reference checksum, Blowfish, link-packet and modulus-scramble code.
func TestCryptMatchesReferenceProbe(t *testing.T) {
	f, err := os.Open("testdata/crypt.golden")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	seen := map[string]int{}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<16)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if strings.HasPrefix(text, "#") {
			continue
		}
		fields := strings.Fields(text)
		hexAt := func(i int) []byte {
			if fields[i] == "-" {
				return []byte{}
			}
			b, err := hex.DecodeString(fields[i])
			if err != nil {
				t.Fatalf("line %d: %v", line, err)
			}
			return b
		}
		boolAt := func(i int) bool {
			v, err := strconv.ParseBool(fields[i])
			if err != nil {
				t.Fatalf("line %d: %v", line, err)
			}
			return v
		}
		switch {
		case fields[0] == "checksum" && len(fields) == 5:
			in := hexAt(1)
			if got := VerifyChecksum(bytes.Clone(in)); got != boolAt(2) {
				t.Errorf("line %d: VerifyChecksum(%x) = %v, want %v", line, in, got, !got)
			}
			if fields[3] != "-" {
				buf := bytes.Clone(in)
				AppendChecksum(buf)
				if want := hexAt(3); !bytes.Equal(buf, want) {
					t.Errorf("line %d: AppendChecksum(%x) = %x, want %x", line, in, buf, want)
				}
				if got := VerifyChecksum(buf); got != boolAt(4) {
					t.Errorf("line %d: VerifyChecksum(appended) = %v, want %v", line, got, !got)
				}
			}
		case fields[0] == "blowfish" && len(fields) == 4:
			c, err := NewBlowfishCipher(hexAt(1))
			if err != nil {
				t.Fatalf("line %d: %v", line, err)
			}
			plain := hexAt(2)
			got := make([]byte, len(plain))
			for off := 0; off < len(plain); off += BlockSize {
				c.Encrypt(got[off:off+BlockSize], plain[off:off+BlockSize])
			}
			if want := hexAt(3); !bytes.Equal(got, want) {
				t.Errorf("line %d: Encrypt = %x, want %x", line, got, want)
			}
		case fields[0] == "link" && len(fields) == 5:
			enc, dec := NewLinkCrypt(), NewLinkCrypt()
			if fields[1] != "-" {
				if err := enc.SetKey(hexAt(1)); err != nil {
					t.Fatalf("line %d: %v", line, err)
				}
				if err := dec.SetKey(hexAt(1)); err != nil {
					t.Fatalf("line %d: %v", line, err)
				}
			}
			got := enc.Encrypt(hexAt(2))
			if want := hexAt(4); !bytes.Equal(got, want) {
				t.Errorf("line %d: Encrypt = %x, want %x", line, got, want)
				continue
			}
			if err := dec.Decrypt(got); err != nil {
				t.Errorf("line %d: Decrypt: %v", line, err)
			} else if want := hexAt(3); !bytes.Equal(got, want) {
				t.Errorf("line %d: Decrypt = %x, want %x", line, got, want)
			}
		case fields[0] == "scramble" && len(fields) == 3:
			got, err := scrambleModulus(new(big.Int).SetBytes(hexAt(1)))
			if err != nil {
				t.Fatalf("line %d: %v", line, err)
			}
			if want := hexAt(2); !bytes.Equal(got, want) {
				t.Errorf("line %d: scrambleModulus = %x, want %x", line, got, want)
			}
		default:
			t.Fatalf("malformed golden line %d %q", line, text)
		}
		seen[fields[0]]++
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"checksum", "blowfish", "link", "scramble"} {
		if seen[kind] == 0 {
			t.Errorf("golden holds no %s vectors", kind)
		}
	}
}
