# Correct the payout record without reissuing a payout

For the MVP's completed PayPal payout with a stale internal payout record, RecoveryOS corrects only that record after verifying `SUCCESS` for the specific payout item. At approval time, the backend fetches that item again and rejects the correction if verification fails or the item is no longer `SUCCESS`. Reissuing a payout would create an unnecessary risk of paying the recipient twice. The MVP provides no manual override of this policy.
