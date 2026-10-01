package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

func TestLoadServerNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "serverNames.xml")
	mustWriteFile(t, path, `<?xml version='1.0' encoding='utf-8'?>
<list>
	<!-- comment -->
	<server id="1" name="Bartz" />
	<server id="5" name="Erica" />
</list>`)

	names, err := LoadServerNames(path)
	if err != nil {
		t.Fatalf("LoadServerNames: %v", err)
	}

	if got, ok := names.Name(1); !ok || got != "Bartz" {
		t.Fatalf("Name(1) = %q, %v", got, ok)
	}
	if got, ok := names.Name(5); !ok || got != "Erica" {
		t.Fatalf("Name(5) = %q, %v", got, ok)
	}
	if _, ok := names.Name(2); ok {
		t.Fatal("Name(2) = true, want false")
	}
	if want := []int{1, 5}; !reflect.DeepEqual(names.IDs(), want) {
		t.Fatalf("IDs() = %v, want %v", names.IDs(), want)
	}
}

// TestLoadServerNamesIDIsStrictDecimal pins the accepted input set of the
// server id to the reference's StatSet.getInteger (Integer.parseInt, Java
// probe OpenJDK 21.0.11): a signed base-10 int32 with nothing around it. An
// absent, empty, padded, non-numeric, prefixed or out-of-range id fails the
// load naming the file; the decoder's own int conversion would read the
// absent and empty forms as 0 and trim the padding.
func TestLoadServerNamesIDIsStrictDecimal(t *testing.T) {
	cases := []struct {
		name    string
		attr    string
		want    int
		wantErr bool
	}{
		{name: "plain", attr: `id="12"`, want: 12},
		{name: "signed", attr: `id="+12"`, want: 12},
		{name: "leading zero is decimal", attr: `id="010"`, want: 10},
		{name: "absent", attr: ``, wantErr: true},
		{name: "empty", attr: `id=""`, wantErr: true},
		{name: "padded", attr: `id=" 12 "`, wantErr: true},
		{name: "non-numeric", attr: `id="abc"`, wantErr: true},
		{name: "hex", attr: `id="0x10"`, wantErr: true},
		{name: "int32 overflow", attr: `id="2147483648"`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "serverNames.xml")
			mustWriteFile(t, path, fmt.Sprintf(`<list><server %s name="Bartz"/></list>`, tc.attr))

			names, err := LoadServerNames(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("LoadServerNames(%s) = %v, want a rejection", tc.attr, names.IDs())
				}
				if !strings.Contains(err.Error(), path) {
					t.Fatalf("LoadServerNames(%s) error %q does not name %q", tc.attr, err, path)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadServerNames(%s): %v", tc.attr, err)
			}
			if got := names.IDs(); len(got) != 1 || got[0] != tc.want {
				t.Fatalf("LoadServerNames(%s) ids = %v, want [%d]", tc.attr, got, tc.want)
			}
		})
	}
}

// TestLoadServerNamesRepeatedIDKeepsLastName pins a repeated id to one
// entry under its last name, as an id-keyed map put would.
func TestLoadServerNamesRepeatedIDKeepsLastName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "serverNames.xml")
	mustWriteFile(t, path, `<list><server id="1" name="Bartz"/><server id="1" name="Erica"/></list>`)
	names, err := LoadServerNames(path)
	if err != nil {
		t.Fatalf("LoadServerNames: %v", err)
	}
	if got := names.IDs(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("IDs() = %v, want [1]", got)
	}
	if got, _ := names.Name(1); got != "Erica" {
		t.Fatalf("Name(1) = %q, want Erica", got)
	}
}

// TestLoadServerNamesShippedFile loads the shipped serverNames.xml: all 127
// ids are bare integers, so the strict reading keeps every entry.
func TestLoadServerNamesShippedFile(t *testing.T) {
	path := filepath.Join(datapack.Require(t), "data", "serverNames.xml")
	names, err := LoadServerNames(path)
	if err != nil {
		t.Fatalf("LoadServerNames(%q): %v", path, err)
	}
	ids := names.IDs()
	if len(ids) != 127 || ids[0] != 1 || ids[len(ids)-1] != 127 {
		t.Fatalf("shipped ids = %d entries from %d to %d, want 127 from 1 to 127", len(ids), ids[0], ids[len(ids)-1])
	}
	if got, ok := names.Name(1); !ok || got != "Bartz" {
		t.Fatalf("Name(1) = %q, %v, want Bartz", got, ok)
	}
}

func TestLoadServerNamesMissingFile(t *testing.T) {
	if _, err := LoadServerNames(filepath.Join(t.TempDir(), "missing.xml")); err == nil {
		t.Fatal("LoadServerNames() error = nil, want error for missing file")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
