package network

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

func TestRequestLinkHTMLSendsCachedNpcHtmlMessage(t *testing.T) {
	html := testHTMLCache(t, map[string]string{"help/tutorial.htm": "<html><body>tutorial</body></html>"})
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, item.NewTable(nil), nil)
	gcl := &GameClientLink{html: html}

	gcl.requestLinkHTML(live, clientpackets.RequestLinkHTML{Link: "data/html/help/tutorial.htm"})

	assertOpcodeSequence(t, capture.frames, serverpackets.OpcodeNpcHtmlMessage)
	assertNpcHtmlMessageFrame(t, capture.frames[0], 0, "<html><body>tutorial</body></html>", 0)
}

func TestRequestLinkHTMLSendsMissingNoticeForSafeMissingFile(t *testing.T) {
	html := testHTMLCache(t, map[string]string{"help/tutorial.htm": "<html/>"})
	capture := &frameCapture{}
	live := newEquipTestLivePlayer(t, 1, capture, item.NewTable(nil), nil)
	gcl := &GameClientLink{html: html}

	gcl.requestLinkHTML(live, clientpackets.RequestLinkHTML{Link: "help/missing.htm"})

	assertOpcodeSequence(t, capture.frames, serverpackets.OpcodeNpcHtmlMessage)
	assertNpcHtmlMessageFrame(t, capture.frames[0], 0, "<html><body>My html is missing:<br>help/missing.htm</body></html>", 0)
}

func TestRequestLinkHTMLRejectsUnsafeLinks(t *testing.T) {
	html := testHTMLCache(t, map[string]string{"help/tutorial.htm": "<html/>"})
	tests := []string{
		"../help/tutorial.htm",
		"help/tutorial.txt",
	}
	for _, link := range tests {
		t.Run(link, func(t *testing.T) {
			capture := &frameCapture{}
			live := newEquipTestLivePlayer(t, 1, capture, item.NewTable(nil), nil)
			gcl := &GameClientLink{html: html}

			gcl.requestLinkHTML(live, clientpackets.RequestLinkHTML{Link: link})

			if len(capture.frames) != 0 {
				t.Fatalf("frames = %x, want none", capture.frames)
			}
		})
	}
}

func TestGameClientLinkRequestLinkHTMLDispatch(t *testing.T) {
	c, chars, _, _ := newLinkedGameClient(t)

	c.send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.read() // CharCreateOk
	c.read() // CharSelectInfo
	chars.soleObjectID(t)

	c.send(encodeRequestGameStart(0))
	c.read() // SSQInfo
	c.read() // CharSelected
	c.send(encodeEnterWorld())
	readEnterWorldBurst(t, c, false)

	c.send(encodeRequestLinkHTML("help/tutorial.htm"))
	reply := c.read()
	if reply[0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("reply opcode = %#x, want NpcHtmlMessage (%#x)", reply[0], serverpackets.OpcodeNpcHtmlMessage)
	}
	assertNpcHtmlMessageFrame(t, reply, 0, "<html><body>tutorial</body></html>", 0)
}

func testHTMLCache(t *testing.T, pages map[string]string) *datacache.HTML {
	t.Helper()
	dir := t.TempDir()
	for name, content := range pages {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	html, err := datacache.LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	return html
}

func assertNpcHtmlMessageFrame(t *testing.T, frame []byte, objectID int32, html string, itemID int32) {
	t.Helper()
	if frame[0] != serverpackets.OpcodeNpcHtmlMessage {
		t.Fatalf("NpcHtmlMessage opcode = %#x, want %#x", frame[0], serverpackets.OpcodeNpcHtmlMessage)
	}
	r := wire.NewReader(frame[1:])
	if got := r.ReadInt32(); got != objectID {
		t.Fatalf("NpcHtmlMessage object id = %d, want %d", got, objectID)
	}
	if got := r.ReadString(); got != html {
		t.Fatalf("NpcHtmlMessage html = %q, want %q", got, html)
	}
	if got := r.ReadInt32(); got != itemID {
		t.Fatalf("NpcHtmlMessage item id = %d, want %d", got, itemID)
	}
	if err := r.Err(); err != nil {
		t.Fatalf("read NpcHtmlMessage: %v", err)
	}
}

func encodeRequestLinkHTML(link string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestLinkHtml)
	w.WriteString(link)
	return w.Bytes()
}
