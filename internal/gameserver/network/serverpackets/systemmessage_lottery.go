package serverpackets

// Lottery system message ids.
const (
	SystemMessageLotteryTicketSalesTempSuspended               = 783  // no parameter
	SystemMessageNoLotteryTicketsAvailable                     = 784  // no parameter
	SystemMessageNoLotteryTicketsCurrentSold                   = 930  // no parameter
	SystemMessageAmountForWinnerS1IsS2AdenaWeHaveS3PrizeWinner = 1112 // numbers: round, jackpot, first-place winners
	SystemMessageAmountForLotteryS1IsS2AdenaNoWinner           = 1113 // numbers: round, jackpot
)
