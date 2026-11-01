package serverpackets

import "testing"

// TestSystemMessageIDs pins the system message ids the server sends by name.
// The message table is data, so the byte probe (packets_oracle_test.go) cannot
// name them; these are the table's ids for the messages below.
func TestSystemMessageIDs(t *testing.T) {
	for _, tc := range []struct{ got, want int }{
		{SystemMessagePetRefusingOrder, 1864},
		{SystemMessageSweeperFailedTargetNotSpoiled, 343},
		{SystemMessageHarvestFailedSeedNotSown, 893},
		{SystemMessageCorpseTooOldSkillNotUsed, 1247},
		{SystemMessageOverHit, 361},
		{SystemMessageS1PerformingCounterattack, 1997},
		{SystemMessageCounteredS1Attack, 1998},
		{SystemMessageResistedS1Magic, 159},
		{SystemMessageForceIncreasedToS1, 323},
		{SystemMessageForceMaxLevelReached, 324},
		{SystemMessageYouEarnedS1ExpAndS2SP, 95},
		{SystemMessageYouPickedUpS2S1, 29},
		{SystemMessageNightSkillEffectApplies, 1131},
		{SystemMessageS1ResistedYourS2, 139},
		{SystemMessageS1HPRestored, 1066},
		{SystemMessageS2HPRestoredByS1, 1067},
		{SystemMessageS1MPRestored, 1068},
		{SystemMessageS2MPRestoredByS1, 1069},
		{SystemMessageS1CPWillBeRestored, 1405},
		{SystemMessageS2CPWillBeRestoredByS1, 1406},
		{SystemMessageS2S3SCreatedForS1ForS4Adena, 1147},
		{SystemMessageS2WasDroppedInTheS1Region, 1815},
		{SystemMessageServerDownInS1Seconds, 1},
	} {
		if tc.got != tc.want {
			t.Errorf("system message id = %d, want %d", tc.got, tc.want)
		}
	}
}
