package recovery

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/KushnerykPavel/recoveryos/api/internal/domain/recovery"
)

var ErrSessionNotFound = errors.New("session not found")
var ErrDailyLimit = errors.New("daily payout creation limit reached")
var ErrInvalidScenario = errors.New("invalid payout scenario")
var ErrScenarioConflict = errors.New("payout scenario cannot be changed")

// Store persists the operational record and its evidence. ObserveSuccess must
// record provider evidence and the selected internal update atomically.
type Store interface {
	CreateSession(ctx context.Context, tokenHash []byte, expiresAt time.Time) (recovery.Session, error)
	Snapshot(ctx context.Context, tokenHash []byte) (recovery.Snapshot, error)
	ReservePayout(ctx context.Context, sessionID, scenario string) (recovery.Payout, bool, error)
	MarkCreateUncertain(ctx context.Context, payoutID string) error
	SetBatch(ctx context.Context, payoutID, batchID string) error
	ObserveItem(ctx context.Context, payoutID, itemID, status string) error
	ObserveSuccess(ctx context.Context, payoutID, itemID string) error
}

type Batch struct{ ID string }

type Item struct {
	ID           string
	BatchID      string
	SenderItemID string
	Status       string
}

// Provider is the outbound PayPal boundary. It does not expose any correction
// or payout retry method to the reconciliation flow.
type Provider interface {
	CreatePayout(ctx context.Context, creationKey, senderItemID string) (Batch, error)
	GetBatchItem(ctx context.Context, batchID string) (Item, error)
}

type Service struct {
	store        Store
	provider     Provider
	clock        func() time.Time
	pollInterval time.Duration
	waitTimeout  time.Duration
}

func NewService(store Store, provider Provider, pollInterval, waitTimeout time.Duration) *Service {
	return &Service{store: store, provider: provider, clock: time.Now, pollInterval: pollInterval, waitTimeout: waitTimeout}
}

func TokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func (s *Service) CreateSession(ctx context.Context) (recovery.Snapshot, string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return recovery.Snapshot{}, "", fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(secret)
	session, err := s.store.CreateSession(ctx, TokenHash(token), s.clock().Add(24*time.Hour))
	if err != nil {
		return recovery.Snapshot{}, "", err
	}
	return recovery.Snapshot{Session: session, Events: []recovery.Event{}}, token, nil
}

func (s *Service) Snapshot(ctx context.Context, token string) (recovery.Snapshot, error) {
	if token == "" {
		return recovery.Snapshot{}, ErrSessionNotFound
	}
	return s.store.Snapshot(ctx, TokenHash(token))
}

func (s *Service) CreatePayout(ctx context.Context, token, requestedScenario string) (recovery.Snapshot, error) {
	if requestedScenario != "" && requestedScenario != recovery.ScenarioStandard && requestedScenario != recovery.ScenarioMissedUpdate {
		return recovery.Snapshot{}, ErrInvalidScenario
	}
	snapshot, err := s.Snapshot(ctx, token)
	if err != nil {
		return recovery.Snapshot{}, err
	}
	scenario := requestedScenario
	if scenario == "" {
		scenario = recovery.ScenarioStandard
	}
	payout, created, err := s.store.ReservePayout(ctx, snapshot.Session.ID, scenario)
	if err != nil {
		return recovery.Snapshot{}, fmt.Errorf("reserve payout: %w", err)
	}
	if !created && requestedScenario != "" && requestedScenario != payout.Scenario {
		return recovery.Snapshot{}, ErrScenarioConflict
	}
	if created {
		batch, err := s.provider.CreatePayout(ctx, payout.CreationKey, payout.SenderItemID)
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err != nil {
			// The outcome may be ambiguous. The reserved key is retained and no
			// later request creates a new payout for this session.
			if markErr := s.store.MarkCreateUncertain(persistCtx, payout.ID); markErr != nil {
				return recovery.Snapshot{}, fmt.Errorf("record uncertain payout creation: %w", markErr)
			}
			return s.store.Snapshot(persistCtx, TokenHash(token))
		}
		if batch.ID == "" {
			if err := s.store.MarkCreateUncertain(persistCtx, payout.ID); err != nil {
				return recovery.Snapshot{}, err
			}
			return s.store.Snapshot(persistCtx, TokenHash(token))
		}
		if err := s.store.SetBatch(persistCtx, payout.ID, batch.ID); err != nil {
			return recovery.Snapshot{}, fmt.Errorf("save payout batch: %w", err)
		}
		payout.BatchID = batch.ID
	}
	if payout.BatchID == "" {
		return s.store.Snapshot(ctx, TokenHash(token))
	}

	deadline := s.clock().Add(s.waitTimeout)
	for {
		snapshot, err = s.reconcile(ctx, token, payout)
		if err != nil {
			return recovery.Snapshot{}, err
		}
		if snapshot.Payout != nil {
			payout = *snapshot.Payout
		}
		if snapshot.Incident != nil ||
			(payout.ProviderStatus != "" && payout.ProviderStatus != "PENDING") ||
			s.pollInterval <= 0 || !s.clock().Before(deadline) {
			return snapshot, nil
		}
		timer := time.NewTimer(s.pollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return recovery.Snapshot{}, ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Service) Reconcile(ctx context.Context, token string) (recovery.Snapshot, error) {
	snapshot, err := s.Snapshot(ctx, token)
	if err != nil || snapshot.Payout == nil || snapshot.Payout.BatchID == "" || snapshot.Incident != nil {
		return snapshot, err
	}
	return s.reconcile(ctx, token, *snapshot.Payout)
}

func (s *Service) reconcile(ctx context.Context, token string, payout recovery.Payout) (recovery.Snapshot, error) {
	item, err := s.provider.GetBatchItem(ctx, payout.BatchID)
	if err != nil {
		return recovery.Snapshot{}, fmt.Errorf("read payout item: %w", err)
	}
	if item.ID == "" && item.Status == "PENDING" {
		return s.store.Snapshot(ctx, TokenHash(token))
	}
	if item.BatchID != payout.BatchID || item.SenderItemID != payout.SenderItemID || item.ID == "" {
		return recovery.Snapshot{}, errors.New("provider item identity mismatch")
	}
	if item.Status == "SUCCESS" {
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		err = s.store.ObserveSuccess(persistCtx, payout.ID, item.ID)
	} else if payout.ItemID == item.ID && payout.ProviderStatus == item.Status {
		return s.store.Snapshot(ctx, TokenHash(token))
	} else {
		err = s.store.ObserveItem(ctx, payout.ID, item.ID, item.Status)
	}
	if err != nil {
		return recovery.Snapshot{}, fmt.Errorf("record payout observation: %w", err)
	}
	return s.store.Snapshot(ctx, TokenHash(token))
}
