# CONCERNS TO REMEMBER WHILE DEVELOPING

The terminal builds the request. The host decides and answers. A response the terminal cannot match to its request is the same as no response: the terminal times out and reverses.

## Who fills which field

The terminal owns DE 2 (PAN), DE 3 (processing code), DE 4 (amount), DE 7 (transmission date and time, UTC), DE 11 (STAN), DE 12 and DE 13 (terminal local time and date), DE 22 (entry mode), DE 41 (terminal id), DE 42 (merchant id), DE 49 (currency). On a reversal it also owns DE 90.

The host owns DE 39 (response code) and the response MTI (`0200` → `0210`, `0400` → `0410`). The host owns DE 38 (approval code) on an approval. The host owns DE 37 only when the request did not bring one.

The host copies the terminal's fields back unchanged: DE 3, 4, 7, 11, 12, 13, 41, 42, 49, and DE 37 when the request already had it. The terminal matches the answer on STAN. Changing DE 11, or answering with a different amount, makes the terminal throw the answer away.

## Handling RRN [DE 37]

On a purchase, if the 0200 has no DE 37 because ATMs are lightweight, might miss it. The host assigns the RRN on the 0210, and that value is the sale's RRN from then on. The terminal stores it and sends it back on the void.

On a void, the 0200 already carries the sale's RRN, and the 0210 should return that same value.

The RRN is how a later void finds the sale. It has to stay unique for as long as that sale can still be voided. The value in this PoC is `Y` + day-of-year + hour + a 6-digit counter. The counter lives in memory, so a host restart can issue an RRN that was already given out.

## Handling STAN [DE 11]

The terminal generates the STAN. The host never invents one and never replaces it. STAN is only 6 digits, so it wraps. It is not unique by itself.

The host's identity for one request is [terminal id + STAN + DE 7]. Two purchases from the same terminal with the same STAN are different sales when DE 7 differs. A repeat of all three is the same request, not a new sale.

#### ASK: Do I block duplicate request if terminal id + STAN + DE 7 is identical? Persist it in redis for idempotency check?

## Duplicate request

If the same request arrives again, the host returns the first answer: same RC, same RRN, same approval code. It does not approve a second time and it does not mint a new RRN.

This happens when the terminal did not see the response and sends the request again. A new approval is a double charge. The record of the first answer has to survive a host restart. An in-memory map forgets it, and the retry looks like a new sale.

## Handling approval code [DE 38]

The host generates DE 38 only when it approves. A decline has no approval code. A duplicate of that approval returns the same code, not a fresh one.

The code is what the receipt and a later void or refund show as the issuer's authorisation. It is not the RRN and it is not the STAN.

## Handling a reversal [0400 / DE 90]

A reversal is not a void. The terminal sends it when it did not get a usable 0210 (timeout, or a response it could not parse). The customer may already have been approved. The 0400 asks the host to undo that.

The 0400 has its own new STAN and its own DE 7. Those identify the reversal message. They do not identify the sale. The sale is named in DE 90: original MTI, original STAN, original DE 7. The host looks up the sale with that, not with the reversal's STAN and not with the RRN.

The 0410 echoes the reversal's own STAN and DE 7. Its RRN is the original sale's RRN when the sale exists. RC 00 on a 0410 means "reversal accepted", not "the sale was approved".

If the host never saw the original 0200, it still answers 0410 RC 00. There is nothing to undo, and a decline makes the terminal keep retrying. If the sale was already reversed, the answer is still 0410 RC 00.

A reversal retry is the same 0400 again: same STAN, same DE 7, same DE 90. It is not a new reversal. The host replays the first 0410.

The amount on the 0400 has to equal the original sale. A smaller amount is a partial reversal, which this host does not do. Do not treat it as a full undo.

## Timeout and response code 96

The terminal reverses when the host stays silent, and it reverses on RC 96 (system malfunction). It does not reverse a business decline.

RC 96 is only for "I could not decide". Using it for insufficient funds or a bad card makes the terminal undo a decline, and then retry. Insufficient funds is RC 51 in ISO 8583. This PoC uses 56 because the scope says so. ISO 8583 uses 56 for "no card record".

If the host approved the sale and then failed to write the 0210, the sale is still live. The reversal that follows has to find it. Deciding "approved" and then withholding the bytes is still an approval.

## Handling a void

A void cancels a sale the terminal already completed, in the same batch, before settlement. It is a new 0200, processing code `020000`, with a new STAN, carrying the sale's RRN. A refund is processing code `20` and is a different transaction, used after the batch has closed. A reversal is an 0400 and is only for a sale whose 0210 the terminal never accepted.

The host finds the sale by RRN, then checks it is the same terminal, the same amount, and an approval that has not already been voided or reversed. A void that does not match is a decline. Approving it anyway creates a credit with no sale under it.

A duplicate void (same terminal, STAN, and DE 7) replays the first 0210. The 0210 keeps the sale's RRN.

After settlement, the void window is closed. The terminal must refund, and the host must reject `020000` for a settled sale.

## Amount and currency [DE 4, DE 49]

DE 4 is 12 digits with no decimal point. `000000100000` with currency `050` is 1000.00 BDT, not 100000 and not 10.00. The host echoes both fields. It does not rescale them.

The currency has to be one that terminal is allowed to send. An amount in the wrong currency is not the same sale.

## Processing code [DE 3]

DE 3 is six digits: transaction type, from-account, to-account. `000000` is a purchase from the default account. `001000` is still a purchase, from savings. Matching only the exact string `000000` rejects a valid purchase.

`02` is an adjustment. This host treats `020000` as a void of a purchase. `20` is a refund. `01` is a cash advance. Anything else needs its own rule. Unknown is a format or "invalid transaction" decline, not RC 96.

## Terminal and merchant [DE 41, DE 42]

Both must exist, be active, and belong to each other. A live terminal id with someone else's merchant id is a decline. This PoC uses RC 58 when the terminal is missing or inactive, and RC 03 when the merchant is.

DE 41 is 8 characters and DE 42 is 15, space-padded on the wire. The host compares the ids, not the raw padding.

## What this PoC does not speak yet

A real terminal will also send these. The host cannot ignore them once they show up.

DE 52 is the PIN block. The host never sees the PIN. It translates the block from the terminal's key to the issuer's key. Wrong keys mean every PIN transaction declines or, worse, verifies against the wrong PIN.

DE 55 is the chip data for a tap (`DE 22` = `071`). The host passes it to the issuer and returns the issuer's result in the 0210. An RC 00 with no valid chip result can still be declined by the card.

DE 64 or DE 128 is the MAC. The host checks it on the request and builds it on the response. Without it, anything on the path can forge an approval.

DE 35 (track 2), DE 52, and DE 55 are card secrets. Logs take a masked PAN only. A full PAN, track, PIN block, or chip data in a log is a cardholder data leak.

Sign-on is an 0800 / 0810 (echo test, key change, cutover). Many terminals will not send a purchase until the host has answered sign-on.
