package clan

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"syscall"
	"testing"
	"time"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPledgeCrestWriteHoldsNoQueue holds the disk write of an uploaded
// pledge crest: while the image is written, the uploader's own queue still
// runs work posted to it; once the write goes through, the uploader gets
// the answer an upload whose write is not held gets, in the same order, and
// the clan points at the image.
func TestPledgeCrestWriteHoldsNoQueue(t *testing.T) {
	w := bootCrestWorld(t, clanSeed{level: 3})
	image := bytes.Repeat([]byte{0x5a}, pledgeCrestSize)
	id := int64(w.srv.NewObjectID()) + 1
	hold := holdCrestWrite(t, crestFile(w, "Crest_", id))

	w.leader.Send(encodeSetPledgeCrest(image))
	hold.awaitWriter(t)
	probeQueue(t, w.srv, w.leaderID)
	if got := hold.release(t, len(image)); !bytes.Equal(got, image) {
		t.Fatalf("written image = %x..., want the uploaded one", got[:min(len(got), 8)])
	}
	held := ownAnswer(drainFrames(t, w.leader))
	if !slices.Equal(held, []byte{serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("held upload answer = %x, want the uploader's UserInfo, then its message", held)
	}
	if got := storedCrest(t, w, "crest_id"); got != id {
		t.Fatalf("stored crest_id = %d, want %d", got, id)
	}

	unheld := ownAnswer(drainSend(t, w, encodeSetPledgeCrest(bytes.Repeat([]byte{0x33}, pledgeCrestSize))))
	if !slices.Equal(held, unheld) {
		t.Fatalf("held upload answer = %x, want %x as an upload not held", held, unheld)
	}
}

// TestAllyCrestWriteHoldsNoQueue holds the disk write of an uploaded
// alliance crest: while the image is written, the uploader's queue still
// runs work posted to it. Once the write goes through, the uploader gets the
// answer an upload not held gets, the other clan's leader is refreshed and
// both clans point at the image.
func TestAllyCrestWriteHoldsNoQueue(t *testing.T) {
	dir := t.TempDir()
	w := bootAllianceWorld(t, gameservertest.WithCrests(datacache.NewCrestsIn(dir)))
	w.formAlliance(t)

	image := bytes.Repeat([]byte{0x5a}, clientpackets.AllyCrestMaxLength)
	id := int64(w.srv.NewObjectID()) + 1
	hold := holdCrestWrite(t, filepath.Join(dir, "AllyCrest_"+strconv.FormatInt(id, 10)+".dds"))
	w.leader.Send(encodeSetAllyCrest(image))
	hold.awaitWriter(t)
	probeQueue(t, w.srv, w.leaderID)

	if got := hold.release(t, len(image)); !bytes.Equal(got, image) {
		t.Fatalf("written image = %x..., want the uploaded one", got[:min(len(got), 8)])
	}
	held := ownAnswer(drainFrames(t, w.leader))
	if !slices.Equal(held, []byte{serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage}) {
		t.Fatalf("held upload answer = %x, want the uploader's UserInfo, then its message", held)
	}
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeUserInfo); !ok {
		t.Fatal("member clan's leader was not refreshed with the new crest")
	}
	if a, b := storedAlly(t, w, knightsClanID).allyCrestID, storedAlly(t, w, rivalsClanID).allyCrestID; a != id || b != id {
		t.Fatalf("alliance crest columns = %d / %d, want %d", a, b, id)
	}

	w.leader.Send(encodeSetAllyCrest(bytes.Repeat([]byte{0x33}, clientpackets.AllyCrestMaxLength)))
	if unheld := ownAnswer(drainFrames(t, w.leader)); !slices.Equal(held, unheld) {
		t.Fatalf("held upload answer = %x, want %x as an upload not held", held, unheld)
	}
}

// ownAnswer is what the uploader's own refresh and answer send it among
// frames, in order: its UserInfo and the system messages. The CharInfo and
// relation of the other members come from their own queues, in no fixed
// order with these.
func ownAnswer(frames [][]byte) []byte {
	return only(frames, serverpackets.OpcodeUserInfo, serverpackets.OpcodeSystemMessage)
}

// crestWrite holds the write of a crest image to path: path is a named
// pipe this end keeps open and full, so the server's open of it returns and
// its write waits until release drains the pipe.
type crestWrite struct {
	path   string
	fd     int
	filled int
}

func holdCrestWrite(t *testing.T, path string) *crestWrite {
	t.Helper()
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Fatal(err)
	}
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Closing the only reader fails a write still held, so a failing test
	// does not leave the server stuck on it.
	t.Cleanup(func() { syscall.Close(fd) })
	// The server's descriptor is matched by the path it resolves to.
	if path, err = filepath.EvalSymlinks(path); err != nil {
		t.Fatal(err)
	}
	h := &crestWrite{path: path, fd: fd}
	page := make([]byte, 4096)
	for {
		n, err := syscall.Write(fd, page)
		if errors.Is(err, syscall.EAGAIN) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		h.filled += n
	}
	for {
		n, err := syscall.Write(fd, page[:1])
		if errors.Is(err, syscall.EAGAIN) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		h.filled += n
	}
	return h
}

// awaitWriter returns once the server has opened the pipe to write the
// image, failing the test if it does not.
func (h *crestWrite) awaitWriter(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("no crest write opened %s", h.path)
		}
		entries, err := os.ReadDir("/proc/self/fd")
		if err != nil {
			t.Fatal(err)
		}
		open := 0
		for _, e := range entries {
			if target, err := os.Readlink("/proc/self/fd/" + e.Name()); err == nil && target == h.path {
				open++
			}
		}
		if open > 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
}

// release drains the pipe, letting the held write through, and returns the
// n bytes it wrote.
func (h *crestWrite) release(t *testing.T, n int) []byte {
	t.Helper()
	if err := syscall.SetNonblock(h.fd, false); err != nil {
		t.Fatal(err)
	}
	readFull := func(buf []byte) {
		for read := 0; read < len(buf); {
			n, err := syscall.Read(h.fd, buf[read:])
			if err != nil && !errors.Is(err, syscall.EINTR) {
				t.Fatal(err)
			}
			read += max(n, 0)
		}
	}
	readFull(make([]byte, h.filled))
	got := make([]byte, n)
	readFull(got)
	return got
}

// probeQueue fails the test unless the queue of the player objectID runs
// work posted to it.
func probeQueue(t *testing.T, srv *gameservertest.Server, objectID int32) {
	t.Helper()
	ran := make(chan struct{})
	if !srv.PlayerQueue(t, objectID).Post(func() { close(ran) }) {
		t.Fatal("the player's queue refused the probe")
	}
	select {
	case <-ran:
	case <-time.After(30 * time.Second):
		t.Fatal("the player's queue is held by the crest write")
	}
}
