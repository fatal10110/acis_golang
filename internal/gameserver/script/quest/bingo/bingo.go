// Package bingo holds the quests that trade their quest item for a game of
// picking six cells of a scrambled board of the numbers 1 to 9, and pay by
// the rows, columns and diagonals the picks complete. The quests are one
// family in one package, whose literals and calls are checked as the union of
// its quests (#4712); each quest keeps its own dialogue and rewards in its
// own file.
package bingo

import (
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// lines returns the cells, numbered 1 to 9 by rows, of every row, column
// and diagonal of the board.
func lines() [8][3]int {
	return [8][3]int{
		{1, 2, 3},
		{4, 5, 6},
		{7, 8, 9},
		{1, 4, 7},
		{2, 5, 8},
		{3, 6, 9},
		{1, 5, 9},
		{3, 5, 7},
	}
}

// variable returns the state's variable key; a variable that was never set
// aborts the invocation, as the board does of a pick made before the bet.
func variable(st *script.QuestState, key string) string {
	v, ok := st.Get(key)
	if !ok {
		panic("bingo: no variable " + key)
	}
	return v
}

// fillBoard fills the cells of the page with the picked numbers of the
// board and a question mark for the cells not picked yet.
func fillBoard(st *script.QuestState, htmltext string) string {
	playerArray := variable(st, "playerArray")
	board := strings.Split(variable(st, "board"), "")

	for i := 1; i < 10; i++ {
		cell := "?"
		if strings.Contains(playerArray, board[i-1]) {
			cell = board[i-1]
		}
		htmltext = strings.ReplaceAll(htmltext, "<?Cell"+strconv.Itoa(i)+"?>", cell)
	}
	return htmltext
}

// winningLines counts the lines of board whose three numbers are all picked.
func winningLines(board []string, picks string) int {
	n := 0
	for _, line := range lines() {
		won := true
		for _, cell := range line {
			won = won && strings.Contains(picks, board[cell-1])
		}
		if won {
			n++
		}
	}
	return n
}

// reveal fills the cells of the page with every number of the board, the
// picked ones in red.
func reveal(htmltext string, board []string, picks string) string {
	for i := 1; i < 10; i++ {
		htmltext = strings.ReplaceAll(htmltext, "<?Cell"+strconv.Itoa(i)+"?>", board[i-1])
		color := "ffffff"
		if strings.Contains(picks, board[i-1]) {
			color = "ff0000"
		}
		htmltext = strings.ReplaceAll(htmltext, "<?FontColor"+strconv.Itoa(i)+"?>", color)
	}
	return htmltext
}
