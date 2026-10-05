package macro

import (
	"reflect"
	"testing"
)

// TestDecodeCommandsReadsUnicodeDigits pins the stored type, d1 and d2 to
// the reference's MacroList.restoreMe, which reads them with
// Integer.parseInt: any Basic Multilingual Plane decimal digit reads as its
// value (a Java probe on OpenJDK 21.0.11, recorded in #3091, prints
// Integer.parseInt("１２") and Integer.parseInt("١٢") as 12). A fullwidth
// letter is no number and fails the decode.
func TestDecodeCommandsReadsUnicodeDigits(t *testing.T) {
	t.Parallel()
	got, err := DecodeCommands("３,١٢,-٥,text;")
	if err != nil {
		t.Fatalf("DecodeCommands: %v", err)
	}
	want := []Command{{Type: 3, D1: 12, D2: -5, Text: "text"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DecodeCommands = %+v, want %+v", got, want)
	}
	if _, err := DecodeCommands("1,ａ,0;"); err == nil {
		t.Fatal("DecodeCommands accepted a fullwidth letter")
	}
}
