//go:build linux || darwin || freebsd

package probe

import (
	"os"

	"golang.org/x/sys/unix"
)

// mapFile maps the whole file read-only. The mapping is private and is
// never written, so it is shared with the page cache.
func mapFile(f *os.File, size int) ([]byte, error) {
	b, err := unix.Mmap(int(f.Fd()), 0, size, unix.PROT_READ, unix.MAP_PRIVATE)
	if err != nil {
		return nil, err
	}
	// Opening checks the whole file once: read it ahead.
	_ = unix.Madvise(b, unix.MADV_WILLNEED)
	return b, nil
}

// adviseRandom tells the kernel queries touch the mapping at random, so it
// stops reading ahead.
func adviseRandom(b []byte) {
	_ = unix.Madvise(b, unix.MADV_RANDOM)
}

func unmapFile(b []byte) {
	_ = unix.Munmap(b)
}
