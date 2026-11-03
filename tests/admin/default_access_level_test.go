package admin

import (
	"context"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// defaultAccessLevels is the user and master rows of the shipped
// access-level table, with //admin open to the master.
func defaultAccessLevels(t *testing.T) (*admin.Data, admin.AccessLevel) {
	t.Helper()
	master := admin.AccessLevel{Level: masterLevel, Name: "Master", NameColor: "00CCFF", TitleColor: "00CCFF", ChildLevel: adminLevel, IsGM: true, AllowFixedRes: true, AllowTransaction: true, AllowAltG: true, GiveDamage: true}
	data, err := admin.NewData([]admin.AccessLevel{
		{Level: userLevel, Name: "User", NameColor: "FFFFFF", TitleColor: "FFFF77", AllowTransaction: true, GiveDamage: true},
		master,
	}, []admin.Command{{Name: "admin_admin", AccessLevel: masterLevel}})
	if err != nil {
		t.Fatalf("new admin data: %v", err)
	}
	return data, master
}

// createDefaultAccessCharacter boots with players.properties
// DefaultAccessLevel = level, creates "Newbie" through the client and
// returns the server, the new character's object id and its stored
// accesslevel and title.
func createDefaultAccessCharacter(t *testing.T, level int, opts ...gameservertest.Option) (srv *gameservertest.Server, objID int32, stored int, title string) {
	t.Helper()
	data, _ := defaultAccessLevels(t)
	opts = append([]gameservertest.Option{
		gameservertest.WithAdmin(data),
		gameservertest.WithDefaultAccessLevel(level),
		gameservertest.WithHTMLPages(map[string]string{"admin/main_menu.htm": "<html><title>Main menu</title></html>"}),
	}, opts...)
	srv = gameservertest.Boot(t, opts...)
	c := srv.Client
	c.Send(encodeCharacterCreate("Newbie"))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("opcode = %#x, want CharCreateOk", reply[0])
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelectInfo {
		t.Fatalf("opcode = %#x, want CharSelectInfo", reply[0])
	}
	objID = srv.SoleObjectID(t)
	row := srv.DB.QueryRowContext(context.Background(), "SELECT accesslevel, title FROM characters WHERE obj_Id = ?", objID)
	if err := row.Scan(&stored, &title); err != nil {
		t.Fatalf("read characters row: %v", err)
	}
	return srv, objID, stored, title
}

// enterWorldUserInfo selects slot 0, enters the world, reads until the
// stream is quiet and returns what the first UserInfo shows of the access
// level.
func enterWorldUserInfo(t *testing.T, c *testsupport.ScriptedClient) userInfoAccess {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	c.Send(w.Bytes())
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	var info *userInfoAccess
	for _, frame := range c.ReadQueued() {
		if frame[0] == serverpackets.OpcodeUserInfo && info == nil {
			got := decodeUserInfoAccess(t, frame)
			info = &got
		}
	}
	if info == nil {
		t.Fatal("enter world sent no UserInfo")
	}
	return *info
}

func TestDefaultAccessLevelGrantedOnCreate(t *testing.T) {
	t.Parallel()

	t.Run("master level", func(t *testing.T) {
		t.Parallel()
		srv, _, stored, title := createDefaultAccessCharacter(t, masterLevel)
		// The row keeps the level; the access level's name is shown as
		// title in the world, not stored at creation.
		if stored != masterLevel || title != "" {
			t.Fatalf("row accesslevel, title = %d, %q; want %d, \"\"", stored, title, masterLevel)
		}
		_, master := defaultAccessLevels(t)
		nameColor, titleColor := master.Colors()
		got := enterWorldUserInfo(t, srv.Client)
		want := userInfoAccess{gm: true, title: "Master", nameColor: nameColor, titleColor: titleColor}
		if got != want {
			t.Fatalf("first login UserInfo = %+v, want %+v", got, want)
		}
		assertPage(t, exchange(t, srv.Client, encodeBuildCmd("admin")), "<title>Main menu</title>")
	})

	t.Run("undefined level stores the user level", func(t *testing.T) {
		t.Parallel()
		srv, _, stored, title := createDefaultAccessCharacter(t, 99, gameservertest.WithCapturedLog())
		if stored != userLevel || title != "" {
			t.Fatalf("row accesslevel, title = %d, %q; want %d, \"\"", stored, title, userLevel)
		}
		if logs := srv.LogText(); !strings.Contains(logs, "undefined level granted") || !strings.Contains(logs, `"level":99`) {
			t.Fatalf("log = %s, want the undefined level 99 warned", logs)
		}
		got := enterWorldUserInfo(t, srv.Client)
		if got.gm || got.title != "" {
			t.Fatalf("first login UserInfo = %+v, want a plain user", got)
		}
		assertTexts(t, exchange(t, srv.Client, encodeBuildCmd("admin")), "You don't have the access right to use this command.")
	})

	t.Run("level 0 creates a plain user", func(t *testing.T) {
		t.Parallel()
		srv, _, stored, title := createDefaultAccessCharacter(t, userLevel)
		if stored != userLevel || title != "" {
			t.Fatalf("row accesslevel, title = %d, %q; want %d, \"\"", stored, title, userLevel)
		}
		if got := enterWorldUserInfo(t, srv.Client); got.gm || got.title != "" {
			t.Fatalf("first login UserInfo = %+v, want a plain user", got)
		}
	})
}

func encodeCharacterCreate(name string) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestCharacterCreate)
	w.WriteString(name)
	w.WriteInt32(0) // race
	w.WriteInt32(0) // sex
	w.WriteInt32(0) // class
	for range 6 {
		w.WriteInt32(0)
	}
	w.WriteInt32(1) // hair style
	w.WriteInt32(0) // hair color
	w.WriteInt32(0) // face
	return w.Bytes()
}
