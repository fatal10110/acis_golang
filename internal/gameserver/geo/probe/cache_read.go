//go:build !(linux || darwin || freebsd)

package probe

import (
	"io"
	"os"
)

// mapFile reads the whole file onto the heap where mapping is not supported.
func mapFile(f *os.File, size int) ([]byte, error) {
	b := make([]byte, size)
	if _, err := io.ReadFull(f, b); err != nil {
		return nil, err
	}
	return b, nil
}

func adviseRandom([]byte) {}

func unmapFile([]byte) {}
