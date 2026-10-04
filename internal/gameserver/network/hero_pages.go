package network

import (
	"strings"
	"unicode/utf16"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/hero"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// The hero pages' datapack files.
const (
	heroDiaryFile   = "data/html/olympiad/herodiary.htm"
	heroHistoryFile = "data/html/olympiad/herohistory.htm"
)

// maxHeroWordsLength is the longest hero message, in UTF-16 units.
const maxHeroWordsLength = 300

// bypassHeroDiary answers _diary?class=<class>&page=<page> with that page
// of the diary of the running era's hero of that class.
func (l *GameClientLink) bypassHeroDiary(live *livePlayer, command string) {
	l.sendHeroPage(live, command, heroDiaryFile, (*hero.Manager).DiaryPage)
}

// bypassHeroFights answers _match?class=<class>&page=<page> with that page
// of the Olympiad fights of the running era's hero of that class.
func (l *GameClientLink) bypassHeroFights(live *livePlayer, command string) {
	l.sendHeroPage(live, command, heroHistoryFile, (*hero.Manager).FightsPage)
}

// sendHeroPage sends live the hero page file filled by fill, without
// taking its links as the ones live may send back. Nothing is sent when
// command's two parameters do not parse, no hero of the running era has
// the class, the hero's character is gone, or fill sends no page.
func (l *GameClientLink) sendHeroPage(live *livePlayer, command, file string,
	fill func(m *hero.Manager, template, heroName string, heroClass, objectID, page int32) (string, bool),
) {
	if l.heroes == nil {
		return
	}
	heroClass, page, ok := heroPageParams(command)
	if !ok {
		return
	}
	id, ok := l.heroes.HeroByClass(int(heroClass))
	if !ok {
		return
	}
	name, ok := l.characterNames([]int32{id})[id]
	if !ok {
		return
	}
	html, ok := fill(l.heroes, l.setPage(file), name, heroClass, id, page)
	if !ok {
		return
	}
	live.SendFrame(serverpackets.FrameNpcHtmlMessage(0, html, 0))
}

// heroPageParams reads a hero page command's class and page: the values of
// its first two '&'-separated parameters after the first '?' (the whole
// command without one), whatever their names. A value is the text after a
// parameter's first '='; the trailing empty pieces of the parameter split
// on '=' count for nothing.
func heroPageParams(command string) (heroClass, page int32, ok bool) {
	params := command[strings.IndexByte(command, '?')+1:]
	tokens := strings.FieldsFunc(params, func(r rune) bool { return r == '&' })
	if len(tokens) < 2 {
		return 0, 0, false
	}
	heroClass, ok = heroPageParam(tokens[0])
	if !ok {
		return 0, 0, false
	}
	page, ok = heroPageParam(tokens[1])
	return heroClass, page, ok
}

func heroPageParam(token string) (int32, bool) {
	parts := strings.Split(token, "=")
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) < 2 {
		return 0, false
	}
	v, err := commons.ParseInt(parts[1], 32)
	if err != nil {
		return 0, false
	}
	return int32(v), true
}

// writeHeroWords makes req's message live's hero message. Only a hero may
// write one, of at most 300 characters; anything else is dropped without
// an answer, as the request expects none.
func (l *GameClientLink) writeHeroWords(live *livePlayer, req clientpackets.RequestWriteHeroWords) {
	if l.heroes == nil || !live.IsHero() || utf16Len(req.Message) > maxHeroWordsLength {
		return
	}
	l.heroes.SetMessage(live.ObjectID(), req.Message)
}

// utf16Len is s's length in UTF-16 units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}
