package serverpackets

// System messages the other members of a party hear when one of them takes
// loot under the party's loot rule.
const (
	// SystemMessageS1ObtainedS3S2 names the member, the item and its count.
	SystemMessageS1ObtainedS3S2 = 299
	// SystemMessageS1ObtainedS2 names the member and the item.
	SystemMessageS1ObtainedS2 = 300
	// SystemMessageS1ObtainedS2S3 names the member, the enchant level and
	// the item.
	SystemMessageS1ObtainedS2S3 = 376
	// SystemMessageS1SweptUpS3S2 names the sweeping member, the item and
	// its count.
	SystemMessageS1SweptUpS3S2 = 608
	// SystemMessageS1SweptUpS2 names the sweeping member and the item.
	SystemMessageS1SweptUpS2 = 609
)
