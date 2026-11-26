package clan

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// readUntilOpcode reads c's frames up to and including the first with opcode
// want, failing the test if none arrives within 100 frames. A frame a later
// scheduled step sends is not queued when the request is answered, so a test
// that needs it reads until it arrives instead of draining what is queued.
func readUntilOpcode(t *testing.T, c *testsupport.ScriptedClient, want byte) [][]byte {
	t.Helper()
	var frames [][]byte
	for range 100 {
		frame := c.Read()
		frames = append(frames, frame)
		if frame[0] == want {
			return frames
		}
	}
	t.Fatalf("opcode %#x not among 100 frames", want)
	return nil
}
