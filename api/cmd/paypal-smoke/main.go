// Command paypal-smoke creates one PayPal Sandbox payout and records item-level evidence.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/KushnerykPavel/recoveryos/api/internal/service/paypal"
)

const sandboxURL = "https://api-m.sandbox.paypal.com"

type evidence struct {
	CreationKey  string    `json:"creation_key"`
	SenderItemID string    `json:"sender_item_id"`
	BatchID      string    `json:"batch_id,omitempty"`
	ItemID       string    `json:"item_id,omitempty"`
	ItemStatus   string    `json:"item_status,omitempty"`
	StartedAt    time.Time `json:"started_at"`
	SucceededAt  time.Time `json:"succeeded_at,omitzero"`
	ElapsedMS    int64     `json:"elapsed_ms,omitempty"`
	Within30s    bool      `json:"within_30_seconds"`
}

func main() {
	path := flag.String("evidence", ".local/paypal-smoke.json", "private evidence file")
	timeout := flag.Duration("timeout", 2*time.Minute, "maximum time to poll for item success")
	interval := flag.Duration("interval", 2*time.Second, "item polling interval")
	flag.Parse()
	if err := run(*path, *timeout, *interval); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path string, timeout, interval time.Duration) error {
	if timeout <= 0 || interval <= 0 {
		return errors.New("timeout and interval must be positive")
	}
	config := paypal.Config{
		BaseURL:      sandboxURL,
		ClientID:     os.Getenv("PAYPAL_CLIENT_ID"),
		ClientSecret: os.Getenv("PAYPAL_CLIENT_SECRET"),
		Receiver:     os.Getenv("PAYPAL_SANDBOX_RECEIVER"),
		Amount:       valueOrDefault("PAYOUT_AMOUNT", "0.01"),
		Currency:     valueOrDefault("PAYOUT_CURRENCY", "USD"),
	}
	if config.ClientID == "" || config.ClientSecret == "" || config.Receiver == "" {
		return errors.New("PAYPAL_CLIENT_ID, PAYPAL_CLIENT_SECRET and PAYPAL_SANDBOX_RECEIVER are required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	lockPath := path + ".lock"
	lock, err := os.OpenFile(lockPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("smoke test is already running or was interrupted; inspect %s before removing its lock", path)
	}
	if err != nil {
		return err
	}
	defer os.Remove(lockPath)
	if err := lock.Close(); err != nil {
		return err
	}
	state, err := loadOrCreate(path)
	if err != nil {
		return err
	}
	client := paypal.New(config, nil)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if state.BatchID == "" {
		// A prior attempt without a batch ID may have reached PayPal. Never retry it.
		if state.ItemStatus == "creation_started" {
			return fmt.Errorf("creation outcome is unknown; inspect %s and PayPal before another payout", path)
		}
		state.ItemStatus = "creation_started"
		if err := save(path, state); err != nil {
			return err
		}
		batch, err := client.CreatePayout(ctx, state.CreationKey, state.SenderItemID)
		if err != nil {
			return fmt.Errorf("payout creation outcome may be unknown: %w", err)
		}
		state.BatchID = batch.ID
		state.ItemStatus = "PENDING"
		if err := save(path, state); err != nil {
			return fmt.Errorf("batch %s was created but evidence could not be saved: %w", batch.ID, err)
		}
		fmt.Printf("Created Sandbox batch %s; polling its payout item.\n", batch.ID)
	}
	if state.ItemStatus == "SUCCESS" {
		fmt.Printf("Existing successful item %s in batch %s.\n", state.ItemID, state.BatchID)
		return nil
	}
	for {
		item, err := client.GetBatchItem(ctx, state.BatchID)
		if err != nil {
			return fmt.Errorf("read payout item (resume with the same evidence file): %w", err)
		}
		if item.ID != "" && item.SenderItemID != state.SenderItemID {
			return errors.New("PayPal payout item identity mismatch")
		}
		state.ItemID = item.ID
		state.ItemStatus = item.Status
		if item.Status == "SUCCESS" && item.ID != "" {
			state.SucceededAt = time.Now().UTC()
			state.ElapsedMS = state.SucceededAt.Sub(state.StartedAt).Milliseconds()
			state.Within30s = state.ElapsedMS <= 30_000
		}
		if err := save(path, state); err != nil {
			return err
		}
		if !state.SucceededAt.IsZero() {
			fmt.Printf("Item %s SUCCESS after %s; within 30 seconds: %t. Evidence: %s\n", state.ItemID, time.Duration(state.ElapsedMS)*time.Millisecond, state.Within30s, path)
			return nil
		}
		if item.Status != "PENDING" && item.Status != "" {
			return fmt.Errorf("item %s ended with status %s; evidence: %s", item.ID, item.Status, path)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("item did not reach SUCCESS within %s; resume using %s", timeout, path)
		case <-time.After(interval):
		}
	}
}

func loadOrCreate(path string) (evidence, error) {
	data, err := os.ReadFile(path)
	if err == nil {
		var state evidence
		if err := json.Unmarshal(data, &state); err != nil || state.CreationKey == "" || state.SenderItemID == "" {
			return evidence{}, fmt.Errorf("invalid evidence file %s; inspect it before creating another payout", path)
		}
		return state, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return evidence{}, err
	}
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return evidence{}, err
	}
	id := hex.EncodeToString(key)
	state := evidence{CreationKey: "recoveryos-smoke-" + id, SenderItemID: "smoke-" + id, StartedAt: time.Now().UTC()}
	data, err = json.MarshalIndent(state, "", "  ")
	if err != nil {
		return evidence{}, err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return evidence{}, err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return evidence{}, err
	}
	if err := f.Close(); err != nil {
		return evidence{}, err
	}
	return state, nil
}

func save(path string, state evidence) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".paypal-smoke-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), path)
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
