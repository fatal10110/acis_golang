package fragments

import "fmt"

func pages(npcID int) []string {
	return []string{
		fmt.Sprint(npcID) + "-01.htm",
		fmt.Sprint(npcID) + "-99.htm",
		fmt.Sprint(npcID) + "-pc.htm",
	}
}
