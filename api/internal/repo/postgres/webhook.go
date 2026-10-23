package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// SavePayPalWebhook records a verified item event for the original live payout.
// A repeated PayPal event ID is harmless. Backup sessions are never matched.
func (s *Store) SavePayPalWebhook(ctx context.Context, eventID, itemID string) (bool, error) {
	var payoutID, sessionID string
	err := s.db.QueryRow(ctx, `
		SELECT id::text, session_id::text FROM payout_records
		WHERE item_id = $1 AND origin = 'live'`, itemID,
	).Scan(&payoutID, &sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = s.db.Exec(ctx, `
		INSERT INTO payout_events (session_id, payout_id, source, kind, details, paypal_event_id)
		VALUES ($1, $2, 'paypal_webhook', 'payout_item_succeeded', $3, $4)
		ON CONFLICT (paypal_event_id) DO NOTHING`,
		sessionID, payoutID, fmt.Sprintf("PayPal event %s confirmed payout item %s succeeded", eventID, itemID), eventID)
	return true, err
}
