# RecoveryOS

RecoveryOS investigates discrepancies between PayPal payouts and the internal operational record of those payouts.

## Language

**Payout**:
A transfer initiated through PayPal to a recipient, identified by its provider reference.
_Avoid_: Payment, transaction

**Payout record**:
The internal operational record of a payout and its current known state. Its changes are preserved in an immutable event history; it is not an accounting ledger.
_Avoid_: Ledger entry, transaction

**Discrepancy**:
A mismatch between a payout's verified PayPal state and its payout record.
_Avoid_: Failure

**Record correction**:
A change to a payout record that brings it into agreement with a verified PayPal payout state. It does not initiate another payout.
_Avoid_: Payout retry, repayment

**Incident**:
An investigated discrepancy between a payout's verified provider state and its payout record.
_Avoid_: Payout failure

**Injected fault**:
A deliberate interruption of the internal update that would normally follow a payout, used to reproduce a discrepancy in the demo.
_Avoid_: PayPal failure, missing webhook

**Demo session**:
An isolated visitor run containing its own incident and payout record. Approval in one demo session affects only that session's record.
_Avoid_: Shared demo state
