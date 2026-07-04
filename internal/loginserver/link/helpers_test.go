package link

import (
	"encoding/binary"
	"unicode/utf16"
)

// appendString appends s UTF-16LE-encoded with its 0x0000 terminator, for
// building test payloads.
func appendString(buf []byte, s string) []byte {
	for _, u := range utf16.Encode([]rune(s)) {
		buf = binary.LittleEndian.AppendUint16(buf, u)
	}
	return binary.LittleEndian.AppendUint16(buf, 0)
}
