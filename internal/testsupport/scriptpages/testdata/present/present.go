package present

import "fmt"

const first = "30048-01.htm"

func pages(npcID int) []string {
	return []string{
		first,
		"30006-03.htm",
		"data/html/script/quest/Q001_LettersOfLove/30033-02.htm",
		"./data/html/script/feature/Alliance/9001-01.htm",
		"data/html/default/30001.htm",
		fmt.Sprint(npcID) + "-01.htm",
		fmt.Sprintf("%d-02.htm", npcID),
		"not a page",
	}
}
