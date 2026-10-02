package network

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
)

// Each period-change notice maps onto its packet: system messages 1210/1211
// (competition begun/ended), 1218/1219 (validation begun/ended), 1212-1217
// (Dawn then Dusk obtained Avarice, Gnosis, Strife), 1241/1240 (Dawn/Dusk
// won), and SSQInfo 256-258 for the regular, Dusk and Dawn skies.
func TestSevenSignsNoticeFrames(t *testing.T) {
	msg := func(id byte, hi byte) []byte { return []byte{0x64, id, hi, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00} }
	for _, tc := range []struct {
		notice sevensigns.Notice
		want   []byte
	}{
		{sevensigns.Notice{Kind: sevensigns.NoticeCompetitionBegun}, msg(0xba, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeCompetitionEnded}, msg(0xbb, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeValidationBegun}, msg(0xc2, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeValidationEnded}, msg(0xc3, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeSealObtained, Cabal: sevensigns.Dawn, Seal: sevensigns.Avarice}, msg(0xbc, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeSealObtained, Cabal: sevensigns.Dawn, Seal: sevensigns.Strife}, msg(0xbe, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeSealObtained, Cabal: sevensigns.Dusk, Seal: sevensigns.Avarice}, msg(0xbf, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeSealObtained, Cabal: sevensigns.Dusk, Seal: sevensigns.Gnosis}, msg(0xc0, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeCabalWon, Cabal: sevensigns.Dawn}, msg(0xd9, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeCabalWon, Cabal: sevensigns.Dusk}, msg(0xd8, 0x04)},
		{sevensigns.Notice{Kind: sevensigns.NoticeSky, Cabal: sevensigns.NoCabal}, []byte{0xf8, 0x00, 0x01}},
		{sevensigns.Notice{Kind: sevensigns.NoticeSky, Cabal: sevensigns.Dusk}, []byte{0xf8, 0x01, 0x01}},
		{sevensigns.Notice{Kind: sevensigns.NoticeSky, Cabal: sevensigns.Dawn}, []byte{0xf8, 0x02, 0x01}},
	} {
		build, ok := sevenSignsNoticeFrame(tc.notice)
		if !ok {
			t.Fatalf("%+v: no packet", tc.notice)
		}
		frame := build()
		got := append([]byte(nil), frame.Bytes()[2:]...)
		frame.Release()
		if !bytes.Equal(got, tc.want) {
			t.Errorf("%+v = % x, want % x", tc.notice, got, tc.want)
		}
	}
	if _, ok := sevenSignsNoticeFrame(sevensigns.Notice{Kind: sevensigns.NoticeSealObtained, Cabal: sevensigns.NoCabal, Seal: sevensigns.Avarice}); ok {
		t.Fatal("an unowned seal was announced as obtained")
	}
}
