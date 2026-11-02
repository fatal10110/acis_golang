package serverpackets

import "bytes"

// Byte-building helpers shared by the hand-written packet tests.

// utf16z is name as a null-terminated UTF-16LE string (writeS).
func utf16z(name string) []byte {
	var out []byte
	for _, r := range name {
		out = append(out, byte(r), byte(r>>8))
	}
	return append(out, 0, 0)
}

func d(v int32) []byte { return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)} }

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }
