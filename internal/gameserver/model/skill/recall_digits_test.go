package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// TestParseTeleCoordsReadsUnicodeDigits pins teleCoords to the reference's
// StatSet.getLocation, which reads each part with Integer.parseInt: any
// Basic Multilingual Plane decimal digit reads as its value (a Java probe on
// OpenJDK 21.0.11, recorded in #3091, prints Integer.parseInt("１２") and
// Integer.parseInt("١٢") as 12). A fullwidth letter is no number.
func TestParseTeleCoordsReadsUnicodeDigits(t *testing.T) {
	t.Parallel()
	got, ok := ParseTeleCoords("１２;-١٢; ３ ")
	if want := (location.Location{X: 12, Y: -12, Z: 3}); !ok || got != want {
		t.Fatalf("ParseTeleCoords = (%+v, %t), want %+v", got, ok, want)
	}
	if got, ok := ParseTeleCoords("1;2;ａ"); ok {
		t.Fatalf("ParseTeleCoords with a fullwidth letter = %+v, want none", got)
	}
}
