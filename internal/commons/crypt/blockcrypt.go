package crypt

import "encoding/binary"

// PaddedSize rounds size up to the next Blowfish block boundary, always
// adding at least one byte of padding — even when size already sits on a
// boundary — matching the padding both the GS-LS link and the login
// server's client-facing protocol expect.
func PaddedSize(size int) int {
	return size + (BlockSize - size%BlockSize)
}

// EncryptBlocks encrypts buf in place, one Blowfish block at a time, with c.
func EncryptBlocks(c *BlowfishCipher, buf []byte) {
	for i := 0; i+BlockSize <= len(buf); i += BlockSize {
		c.Encrypt(buf[i:i+BlockSize], buf[i:i+BlockSize])
	}
}

// DecryptBlocks decrypts buf in place, one Blowfish block at a time, with c.
func DecryptBlocks(c *BlowfishCipher, buf []byte) {
	for i := 0; i+BlockSize <= len(buf); i += BlockSize {
		c.Decrypt(buf[i:i+BlockSize], buf[i:i+BlockSize])
	}
}

// AppendChecksum XOR-folds every 4-byte little-endian word in buf except the
// last into the last word, so VerifyChecksum on the same buf succeeds.
// Requires len(buf) to be a positive multiple of 4 greater than 4.
func AppendChecksum(buf []byte) {
	var chksum uint32
	for i := 0; i < len(buf)-4; i += 4 {
		chksum ^= binary.LittleEndian.Uint32(buf[i : i+4])
	}
	binary.LittleEndian.PutUint32(buf[len(buf)-4:], chksum)
}

// VerifyChecksum reports whether the last 4-byte little-endian word in buf
// equals the XOR-fold of every word before it. Returns false if len(buf) is
// not a multiple of 4 or is 4 or less.
func VerifyChecksum(buf []byte) bool {
	if len(buf)%4 != 0 || len(buf) <= 4 {
		return false
	}
	var chksum uint32
	for i := 0; i < len(buf)-4; i += 4 {
		chksum ^= binary.LittleEndian.Uint32(buf[i : i+4])
	}
	return binary.LittleEndian.Uint32(buf[len(buf)-4:]) == chksum
}
