package link

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fatal10110/acis_golang/internal/config"
)

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

// WriteHexIDFile stores a game server's id and auth key in the properties
// file the game server reads its registration from at boot, stamped with
// now. The file is replaced whole: the new content goes to a temporary
// file in the same directory first, then takes the old one's name, so a
// failed write never leaves a truncated file behind.
func WriteHexIDFile(path string, serverID int, key []byte, now time.Time) error {
	content := fmt.Sprintf("#the hexID to auth into login\n#%s\nServerID=%d\nHexID=%s\n",
		now.Format("Mon Jan 02 15:04:05 MST 2006"), serverID, HexKeyText(key))

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed
	if _, err := tmp.WriteString(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

// HexIDFromProperties returns the server id and auth key a loaded hexid
// file holds.
func HexIDFromProperties(props *config.Properties) (int, []byte, error) {
	idText, ok := props.Lookup("ServerID")
	if !ok {
		return 0, nil, fmt.Errorf("missing ServerID in hexid file")
	}
	id, err := strconv.ParseInt(idText, 10, 32)
	if err != nil {
		return 0, nil, fmt.Errorf("parse ServerID in hexid file: %w", err)
	}
	hexText, ok := props.Lookup("HexID")
	if !ok {
		return 0, nil, fmt.Errorf("missing HexID in hexid file")
	}
	key, err := ParseHexKey(hexText)
	if err != nil {
		return 0, nil, err
	}
	return int(id), key, nil
}

// ReadHexIDFile returns the server id and auth key the hexid file at path
// holds. A missing file is reported as fs.ErrNotExist.
func ReadHexIDFile(path string) (int, []byte, error) {
	if _, err := os.Stat(path); err != nil {
		return 0, nil, err
	}
	props, err := config.LoadFile(path)
	if err != nil {
		return 0, nil, err
	}
	id, key, err := HexIDFromProperties(props)
	if err != nil {
		return 0, nil, fmt.Errorf("%s: %w", path, err)
	}
	return id, key, nil
}
