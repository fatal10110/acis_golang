package model

import (
	"fmt"
	"math/big"
)

// GameServer is a registered game server row.
type GameServer struct {
	ID    int
	HexID []byte
	Host  string
}

// NewGameServer returns a GameServer with its auth key copied.
func NewGameServer(id int, hexID []byte, host string) GameServer {
	key := append([]byte(nil), hexID...)
	return GameServer{ID: id, HexID: key, Host: host}
}

// HexKeyText renders an auth key in the signed big-integer hex form used by
// the gameservers table's hexid column and by hexid files. Keys whose top
// bit is set render as a negative hex string; ParseHexKey reverses this to
// the minimal two's-complement byte form, not necessarily the original
// length.
func HexKeyText(key []byte) string {
	if key == nil {
		return "null"
	}
	return signedBytesToInt(key).Text(16)
}

// ParseHexKey parses text produced by HexKeyText back into key bytes.
func ParseHexKey(text string) ([]byte, error) {
	n, ok := new(big.Int).SetString(text, 16)
	if !ok {
		return nil, fmt.Errorf("invalid hex key %q", text)
	}
	return signedIntToBytes(n), nil
}

func signedBytesToInt(b []byte) *big.Int {
	n := new(big.Int).SetBytes(b)
	if len(b) == 0 || b[0]&0x80 == 0 {
		return n
	}

	mod := new(big.Int).Lsh(big.NewInt(1), uint(len(b)*8))
	return n.Sub(n, mod)
}

func signedIntToBytes(n *big.Int) []byte {
	if n.Sign() == 0 {
		return []byte{0}
	}
	if n.Sign() > 0 {
		b := n.Bytes()
		if b[0]&0x80 == 0 {
			return b
		}
		return append([]byte{0}, b...)
	}

	length := 1
	for {
		min := new(big.Int).Lsh(big.NewInt(1), uint(8*length-1))
		min.Neg(min)
		if n.Cmp(min) >= 0 {
			break
		}
		length++
	}

	mod := new(big.Int).Lsh(big.NewInt(1), uint(8*length))
	b := new(big.Int).Add(mod, n).Bytes()
	if len(b) >= length {
		return b
	}
	out := make([]byte, length)
	for i := 0; i < length-len(b); i++ {
		out[i] = 0xff
	}
	copy(out[length-len(b):], b)
	return out
}
