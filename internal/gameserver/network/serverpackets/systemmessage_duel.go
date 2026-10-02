package serverpackets

// Duel system message ids. Each S1 or S2 is a character name.
const (
	SystemMessageNoOpponentForDuel           = 1926
	SystemMessageS1ChallengedToDuel          = 1927
	SystemMessageS1PartyChallengedToDuel     = 1928
	SystemMessageS1AcceptedYourDuel          = 1929
	SystemMessageYouAcceptedS1Duel           = 1930
	SystemMessageS1DeclinedYourDuel          = 1931
	SystemMessageYouAcceptedS1PartyDuel      = 1933
	SystemMessageS1AcceptedYourPartyDuel     = 1934
	SystemMessageOpposingPartyDeclinedDuel   = 1936
	SystemMessageChallengedNotInParty        = 1937
	SystemMessageS1ChallengedYouToDuel       = 1938
	SystemMessageS1PartyChallengedYourParty  = 1939
	SystemMessageUnableToRequestDuel         = 1940
	SystemMessageOpposingPartyUnableToDuel   = 1942
	SystemMessageTransportedToDuelSite       = 1944
	SystemMessageDuelBeginsInS1Seconds       = 1945 // S1 is a number
	SystemMessageLetTheDuelBegin             = 1949
	SystemMessageS1WonTheDuel                = 1950
	SystemMessageS1PartyWonTheDuel           = 1951
	SystemMessageDuelEndedInTie              = 1952
	SystemMessageS1WithdrewS2Won             = 1955
	SystemMessageS1PartyWithdrewS2PartyWon   = 1956
	SystemMessageS1CannotDuelPrivateStore    = 2017
	SystemMessageS1CannotDuelFishing         = 2018
	SystemMessageS1CannotDuelHPOrMPBelowHalf = 2019
	SystemMessageS1CannotDuelProhibitedArea  = 2020
	SystemMessageS1CannotDuelInBattle        = 2021
	SystemMessageS1CannotDuelAlreadyDuelling = 2022
	SystemMessageS1CannotDuelChaotic         = 2023
	SystemMessageS1CannotDuelOlympiad        = 2024
	SystemMessageS1CannotDuelRiding          = 2027
	SystemMessageS1CannotReceiveDuelTooFar   = 2028
	SystemMessageOtherPartyIsFrozen          = 692 // no parameter
)
