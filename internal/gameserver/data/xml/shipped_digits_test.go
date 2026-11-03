package xml

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// TestShippedDataHasNoNonASCIIDigits proves the shipped XML reads the same
// whether integers are parsed over ASCII digits only or over every decimal
// digit: no file holds a non-ASCII decimal digit, nor a fullwidth Latin
// letter, which a hexadecimal parse over every digit would also read.
func TestShippedDataHasNoNonASCIIDigits(t *testing.T) {
	t.Parallel()
	root := datapackPath(t, "data")
	files := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".xml") {
			return err
		}
		files++
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, r := range string(data) {
			fullwidthLetter := ('Ａ' <= r && r <= 'Ｚ') || ('ａ' <= r && r <= 'ｚ')
			if r > unicode.MaxASCII && (unicode.IsDigit(r) || fullwidthLetter) {
				t.Errorf("%s: byte %d: %U reads as a digit", path, i, r)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if files == 0 {
		t.Fatal("no shipped XML file scanned")
	}
}
