package model

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

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
