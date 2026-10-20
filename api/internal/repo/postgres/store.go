package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KushnerykPavel/recoveryos/api/internal/domain/demo"
	demouc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/demo"
)

type Store struct {
	db         *pgxpool.Pool
	dailyLimit int
}

func New(db *pgxpool.Pool, dailyLimit int) *Store { return &Store{db: db, dailyLimit: dailyLimit} }

func (s *Store) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, expiresAt time.Time) (demo.Session, error) {
	var session demo.Session
	err := s.db.QueryRow(ctx, `
		INSERT INTO demo_sessions (token_hash, expires_at) VALUES ($1, $2)
		RETURNING id::text, created_at, expires_at`, tokenHash, expiresAt,
	).Scan(&session.ID, &session.CreatedAt, &session.ExpiresAt)
	return session, err
}

func (s *Store) Snapshot(ctx context.Context, tokenHash []byte) (demo.Snapshot, error) {
	var snapshot demo.Snapshot
	snapshot.Events = []demo.Event{}
	err := s.db.QueryRow(ctx, `
		SELECT id::text, created_at, expires_at FROM demo_sessions
		WHERE token_hash = $1 AND expires_at > now()`, tokenHash,
	).Scan(&snapshot.Session.ID, &snapshot.Session.CreatedAt, &snapshot.Session.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return demo.Snapshot{}, demouc.ErrSessionNotFound
	}
	if err != nil {
		return demo.Snapshot{}, err
	}

	payout, err := s.getPayout(ctx, snapshot.Session.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return demo.Snapshot{}, err
	}
	if err == nil {
		snapshot.Payout = &payout
		var incident demo.Incident
		err = s.db.QueryRow(ctx, `
			SELECT id::text, payout_id::text, status, created_at
			FROM incidents WHERE payout_id = $1`, payout.ID,
		).Scan(&incident.ID, &incident.PayoutID, &incident.Status, &incident.CreatedAt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return demo.Snapshot{}, err
		}
		if err == nil {
			snapshot.Incident = &incident
		}
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, source, kind, details, occurred_at FROM payout_events
		WHERE session_id = $1 ORDER BY id`, snapshot.Session.ID)
	if err != nil {
		return demo.Snapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var event demo.Event
		if err := rows.Scan(&event.ID, &event.Source, &event.Kind, &event.Details, &event.OccurredAt); err != nil {
			return demo.Snapshot{}, err
		}
		snapshot.Events = append(snapshot.Events, event)
	}
	return snapshot, rows.Err()
}

func (s *Store) ReservePayout(ctx context.Context, sessionID string) (demo.Payout, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return demo.Payout{}, false, err
	}
	defer tx.Rollback(ctx)
	var payout demo.Payout
	var created bool
	err = tx.QueryRow(ctx, `
		INSERT INTO payout_records (session_id, creation_key, sender_item_id)
		VALUES ($1, 'recoveryos-' || $1::text, 'item-' || $1::text)
		ON CONFLICT (session_id) DO NOTHING
		RETURNING id::text, session_id::text, creation_key, sender_item_id, status, created_at`, sessionID,
	).Scan(&payout.ID, &payout.SessionID, &payout.CreationKey, &payout.SenderItemID, &payout.Status, &payout.CreatedAt)
	if err == nil {
		created = true
	} else if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Rollback(ctx); err != nil {
			return demo.Payout{}, false, err
		}
		payout, err = s.getPayout(ctx, sessionID)
		return payout, false, err
	} else {
		return demo.Payout{}, false, err
	}
	var count int
	err = tx.QueryRow(ctx, `
		INSERT INTO daily_payout_counts (day_utc, count)
		VALUES ((now() AT TIME ZONE 'UTC')::date, 1)
		ON CONFLICT (day_utc) DO UPDATE SET count = daily_payout_counts.count + 1
		WHERE daily_payout_counts.count < $1
		RETURNING count`, s.dailyLimit).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return demo.Payout{}, false, demouc.ErrDailyLimit
	}
	if err != nil {
		return demo.Payout{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return demo.Payout{}, false, err
	}
	return payout, created, nil
}

func (s *Store) getPayout(ctx context.Context, sessionID string) (demo.Payout, error) {
	var payout demo.Payout
	var batchID, itemID, providerStatus pgtype.Text
	err := s.db.QueryRow(ctx, `
		SELECT id::text, session_id::text, creation_key, sender_item_id,
		       batch_id, item_id, provider_status, status, created_at
		FROM payout_records WHERE session_id = $1`, sessionID,
	).Scan(&payout.ID, &payout.SessionID, &payout.CreationKey, &payout.SenderItemID,
		&batchID, &itemID, &providerStatus, &payout.Status, &payout.CreatedAt)
	if batchID.Valid {
		payout.BatchID = batchID.String
	}
	if itemID.Valid {
		payout.ItemID = itemID.String
	}
	if providerStatus.Valid {
		payout.ProviderStatus = providerStatus.String
	}
	return payout, err
}

func (s *Store) SetBatch(ctx context.Context, payoutID, batchID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sessionID string
	err = tx.QueryRow(ctx, `
		UPDATE payout_records SET batch_id = $2, status = 'pending'
		WHERE id = $1 AND batch_id IS NULL RETURNING session_id::text`, payoutID, batchID,
	).Scan(&sessionID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO payout_events (session_id, payout_id, source, kind, details)
		VALUES ($1, $2, 'paypal', 'batch_created', $3)`, sessionID, payoutID, batchID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) MarkCreateUncertain(ctx context.Context, payoutID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sessionID string
	err = tx.QueryRow(ctx, `
		UPDATE payout_records SET status = 'provider_unknown'
		WHERE id = $1 AND status = 'creating' RETURNING session_id::text`, payoutID,
	).Scan(&sessionID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO payout_events (session_id, payout_id, source, kind, details)
		VALUES ($1, $2, 'system', 'creation_outcome_unknown', 'PayPal create result could not be confirmed')`, sessionID, payoutID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ObserveItem(ctx context.Context, payoutID, itemID, status string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sessionID string
	err = tx.QueryRow(ctx, `
		UPDATE payout_records SET item_id = $2, provider_status = $3
		WHERE id = $1 AND status = 'pending' AND (item_id IS NULL OR item_id = $2)
		RETURNING session_id::text`, payoutID, itemID, status,
	).Scan(&sessionID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO payout_events (session_id, payout_id, source, kind, details)
		VALUES ($1, $2, 'paypal', 'item_observed', $3)`, sessionID, payoutID, itemID+":"+status)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) ObserveSuccess(ctx context.Context, payoutID, itemID string) error {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sessionID, status string
	var existingItemID pgtype.Text
	err = tx.QueryRow(ctx, `
		SELECT session_id::text, status, item_id FROM payout_records WHERE id = $1 FOR UPDATE`, payoutID,
	).Scan(&sessionID, &status, &existingItemID)
	if err != nil {
		return err
	}
	if existingItemID.Valid && existingItemID.String != itemID {
		return errors.New("PayPal payout item identity changed")
	}
	if status == "stale" {
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("cannot observe success from payout status %q", status)
	}
	_, err = tx.Exec(ctx, `
		UPDATE payout_records SET item_id = $2, provider_status = 'SUCCESS', status = 'stale'
		WHERE id = $1`, payoutID, itemID)
	if err != nil {
		return err
	}
	for _, event := range []struct{ source, kind, details string }{
		{"paypal", "item_observed", itemID + ":SUCCESS"},
		{"injected", "internal_update_skipped", "successful provider item deliberately left stale"},
	} {
		_, err = tx.Exec(ctx, `
			INSERT INTO payout_events (session_id, payout_id, source, kind, details)
			VALUES ($1, $2, $3, $4, $5)`, sessionID, payoutID, event.source, event.kind, event.details)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO incidents (session_id, payout_id, status)
		VALUES ($1, $2, 'open') ON CONFLICT (payout_id) DO NOTHING`, sessionID, payoutID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
