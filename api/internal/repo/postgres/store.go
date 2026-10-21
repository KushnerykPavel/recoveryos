package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/KushnerykPavel/recoveryos/api/internal/domain/recovery"
	recoveryuc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/recovery"
)

type Store struct {
	db         *pgxpool.Pool
	dailyLimit int
}

func New(db *pgxpool.Pool, dailyLimit int) *Store { return &Store{db: db, dailyLimit: dailyLimit} }

func (s *Store) Ping(ctx context.Context) error { return s.db.Ping(ctx) }

func (s *Store) CreateSession(ctx context.Context, tokenHash []byte, expiresAt time.Time) (recovery.Session, error) {
	var session recovery.Session
	err := s.db.QueryRow(ctx, `
		INSERT INTO sessions (token_hash, expires_at) VALUES ($1, $2)
		RETURNING id::text, created_at, expires_at`, tokenHash, expiresAt,
	).Scan(&session.ID, &session.CreatedAt, &session.ExpiresAt)
	return session, err
}

func (s *Store) Snapshot(ctx context.Context, tokenHash []byte) (recovery.Snapshot, error) {
	var snapshot recovery.Snapshot
	snapshot.Events = []recovery.Event{}
	err := s.db.QueryRow(ctx, `
		SELECT id::text, created_at, expires_at FROM sessions
		WHERE token_hash = $1 AND expires_at > now()`, tokenHash,
	).Scan(&snapshot.Session.ID, &snapshot.Session.CreatedAt, &snapshot.Session.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return recovery.Snapshot{}, recoveryuc.ErrSessionNotFound
	}
	if err != nil {
		return recovery.Snapshot{}, err
	}

	payout, err := s.getPayout(ctx, snapshot.Session.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return recovery.Snapshot{}, err
	}
	if err == nil {
		snapshot.Payout = &payout
		var incident recovery.Incident
		err = s.db.QueryRow(ctx, `
			SELECT id::text, payout_id::text, status, created_at
			FROM incidents WHERE payout_id = $1`, payout.ID,
		).Scan(&incident.ID, &incident.PayoutID, &incident.Status, &incident.CreatedAt)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return recovery.Snapshot{}, err
		}
		if err == nil {
			snapshot.Incident = &incident
			var raw []byte
			err = s.db.QueryRow(ctx, `SELECT result FROM investigations WHERE session_id = $1 AND incident_id = $2 ORDER BY id DESC LIMIT 1`, snapshot.Session.ID, incident.ID).Scan(&raw)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return recovery.Snapshot{}, err
			}
			if err == nil {
				var result recovery.Investigation
				if err := json.Unmarshal(raw, &result); err != nil {
					return recovery.Snapshot{}, err
				}
				snapshot.Investigation = &result
			}
		}
	}

	rows, err := s.db.Query(ctx, `
		SELECT id, source, kind, details, occurred_at FROM payout_events
		WHERE session_id = $1 ORDER BY id`, snapshot.Session.ID)
	if err != nil {
		return recovery.Snapshot{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var event recovery.Event
		if err := rows.Scan(&event.ID, &event.Source, &event.Kind, &event.Details, &event.OccurredAt); err != nil {
			return recovery.Snapshot{}, err
		}
		snapshot.Events = append(snapshot.Events, event)
	}
	return snapshot, rows.Err()
}

func (s *Store) SaveInvestigation(ctx context.Context, sessionID, incidentID string, result recovery.Investigation) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return err
	}
	command, err := s.db.Exec(ctx, `
		INSERT INTO investigations (session_id, incident_id, result)
		SELECT i.session_id, i.id, $3 FROM incidents i
		WHERE i.id = $2 AND i.session_id = $1 AND i.status = 'open'`, sessionID, incidentID, string(encoded))
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return errors.New("incident is no longer open")
	}
	return nil
}

func (s *Store) ReservePayout(ctx context.Context, sessionID, scenario string) (recovery.Payout, bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return recovery.Payout{}, false, err
	}
	defer tx.Rollback(ctx)
	var payout recovery.Payout
	var created bool
	err = tx.QueryRow(ctx, `
		INSERT INTO payout_records (session_id, creation_key, sender_item_id, scenario)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (session_id) DO NOTHING
		RETURNING id::text, session_id::text, creation_key, sender_item_id, status, scenario, created_at`,
		sessionID, "recoveryos-"+sessionID, "item-"+sessionID, scenario,
	).Scan(&payout.ID, &payout.SessionID, &payout.CreationKey, &payout.SenderItemID, &payout.Status, &payout.Scenario, &payout.CreatedAt)
	if err == nil {
		created = true
	} else if errors.Is(err, pgx.ErrNoRows) {
		if err := tx.Rollback(ctx); err != nil {
			return recovery.Payout{}, false, err
		}
		payout, err = s.getPayout(ctx, sessionID)
		return payout, false, err
	} else {
		return recovery.Payout{}, false, err
	}
	var count int
	err = tx.QueryRow(ctx, `
		INSERT INTO daily_payout_counts (day_utc, count)
		VALUES ((now() AT TIME ZONE 'UTC')::date, 1)
		ON CONFLICT (day_utc) DO UPDATE SET count = daily_payout_counts.count + 1
		WHERE daily_payout_counts.count < $1
		RETURNING count`, s.dailyLimit).Scan(&count)
	if errors.Is(err, pgx.ErrNoRows) {
		return recovery.Payout{}, false, recoveryuc.ErrDailyLimit
	}
	if err != nil {
		return recovery.Payout{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return recovery.Payout{}, false, err
	}
	return payout, created, nil
}

func (s *Store) getPayout(ctx context.Context, sessionID string) (recovery.Payout, error) {
	var payout recovery.Payout
	var batchID, itemID, providerStatus pgtype.Text
	err := s.db.QueryRow(ctx, `
		SELECT id::text, session_id::text, creation_key, sender_item_id,
		       batch_id, item_id, provider_status, status, scenario, created_at
		FROM payout_records WHERE session_id = $1`, sessionID,
	).Scan(&payout.ID, &payout.SessionID, &payout.CreationKey, &payout.SenderItemID,
		&batchID, &itemID, &providerStatus, &payout.Status, &payout.Scenario, &payout.CreatedAt)
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
	var sessionID, status, scenario string
	var existingItemID, providerStatus pgtype.Text
	err = tx.QueryRow(ctx, `
		SELECT session_id::text, status, scenario, item_id, provider_status
		FROM payout_records WHERE id = $1 FOR UPDATE`, payoutID,
	).Scan(&sessionID, &status, &scenario, &existingItemID, &providerStatus)
	if err != nil {
		return err
	}
	if existingItemID.Valid && existingItemID.String != itemID {
		return errors.New("PayPal payout item identity changed")
	}
	if status == "stale" || status == "completed" || (providerStatus.Valid && providerStatus.String == "SUCCESS") {
		return nil
	}
	if status != "pending" {
		return fmt.Errorf("cannot observe success from payout status %q", status)
	}
	nextStatus := "completed"
	if scenario == recovery.ScenarioMissedUpdate {
		nextStatus = "pending"
	}
	_, err = tx.Exec(ctx, `
		UPDATE payout_records SET item_id = $2, provider_status = 'SUCCESS', status = $3
		WHERE id = $1`, payoutID, itemID, nextStatus)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO payout_events (session_id, payout_id, source, kind, details)
		VALUES ($1, $2, 'paypal', 'item_observed', $3)`, sessionID, payoutID, itemID+":SUCCESS")
	if err != nil {
		return err
	}
	if scenario == recovery.ScenarioMissedUpdate {
		_, err = tx.Exec(ctx, `
			INSERT INTO payout_events (session_id, payout_id, source, kind, details)
			VALUES ($1, $2, 'injected', 'internal_update_skipped', $3)`, sessionID, payoutID,
			"successful provider item deliberately left stale")
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO incidents (session_id, payout_id, status)
			VALUES ($1, $2, 'open') ON CONFLICT (payout_id) DO NOTHING`, sessionID, payoutID)
		if err != nil {
			return err
		}
	} else {
		_, err = tx.Exec(ctx, `
			INSERT INTO payout_events (session_id, payout_id, source, kind, details)
			VALUES ($1, $2, 'system', 'internal_record_updated', $3)`, sessionID, payoutID,
			"payout record updated after provider success")
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
