package script

// EquipItem puts the item rightHand in the NPC's right hand and leftHand in
// its left, 0 for an empty hand, and shows the players around it its new
// look. The right-hand item decides its attack style and reuse from then
// on; its death puts its template's items back.
func (n *NPC) EquipItem(rightHand, leftHand int32) {
	n.hostile("equip an item").EquipItem(rightHand, leftHand)
}

// SetEnchantEffect sets the enchant glow of the NPC's weapon and shows the
// players around it its new look; its death clears the glow.
func (n *NPC) SetEnchantEffect(enchant int32) {
	n.hostile("set the enchant effect").SetEnchantEffect(enchant)
}

// LastAttacker returns the last creature that damaged the NPC in this
// life, nil before any did.
func (n *NPC) LastAttacker() Creature {
	return creatureOf(n.hostile("read the last attacker").LastAttacker())
}
