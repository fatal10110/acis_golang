package network

// adminTest answers //test, whatever follows it, with its usage.
func (l *GameClientLink) adminTest(gm *livePlayer, _ string) {
	sendText(gm, "Usage : //test ...")
}
