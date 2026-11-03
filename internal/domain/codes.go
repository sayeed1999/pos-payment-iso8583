package domain

// DE 3 processing code. ISO 8583 transaction type 00 is a purchase.
// Type 02 is an adjustment. Card-present hosts use 020000 as a void of a
// purchase (same batch, sale cancelled). A refund is type 20 and is a
// different transaction.
const (
	ProcPurchase = "000000"
	ProcVoid     = "020000"
)

const (
	RCApproved = "00"
	RCMerchant = "03" // invalid merchant
	RCFormat   = "30"
	RCTerminal = "58" // transaction not permitted to terminal

	// RCInsufficientFunds is 56 because the PoC scope says so.
	// ISO 8583 itself uses 51 for insufficient funds and 56 for "no card record".
	RCInsufficientFunds = "56"
	RCSystemFailure     = "96"
)
