package paypal

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	demouc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/demo"
)

type Config struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	Receiver     string
	Amount       string
	Currency     string
}

type Client struct {
	config      Config
	http        *http.Client
	tokenMu     sync.Mutex
	token       string
	tokenExpiry time.Time
}

type payoutAmount struct {
	Value    string `json:"value"`
	Currency string `json:"currency"`
}

type payoutRequest struct {
	SenderBatchHeader struct {
		SenderBatchID string `json:"sender_batch_id"`
		RecipientType string `json:"recipient_type"`
		EmailSubject  string `json:"email_subject"`
	} `json:"sender_batch_header"`
	Items []struct {
		Receiver     string       `json:"receiver"`
		SenderItemID string       `json:"sender_item_id"`
		Amount       payoutAmount `json:"amount"`
	} `json:"items"`
}

type batchResponse struct {
	BatchHeader struct {
		ID string `json:"payout_batch_id"`
	} `json:"batch_header"`
	Items []struct {
		ID         string `json:"payout_item_id"`
		Status     string `json:"transaction_status"`
		PayoutItem struct {
			SenderItemID string `json:"sender_item_id"`
		} `json:"payout_item"`
	} `json:"items"`
}

func New(config Config, client *http.Client) *Client {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{config: config, http: client}
}

func (c *Client) CreatePayout(ctx context.Context, creationKey, senderItemID string) (demouc.Batch, error) {
	if len(senderItemID) > 63 {
		return demouc.Batch{}, errors.New("sender item ID exceeds PayPal limit")
	}
	requestBody := payoutRequest{}
	requestBody.SenderBatchHeader.SenderBatchID = creationKey
	requestBody.SenderBatchHeader.RecipientType = "EMAIL"
	requestBody.SenderBatchHeader.EmailSubject = "RecoveryOS Sandbox demo payout"
	requestBody.Items = make([]struct {
		Receiver     string       `json:"receiver"`
		SenderItemID string       `json:"sender_item_id"`
		Amount       payoutAmount `json:"amount"`
	}, 1)
	requestBody.Items[0].Receiver = c.config.Receiver
	requestBody.Items[0].SenderItemID = senderItemID
	requestBody.Items[0].Amount.Value = c.config.Amount
	requestBody.Items[0].Amount.Currency = c.config.Currency

	var response batchResponse
	if err := c.call(ctx, http.MethodPost, "/v1/payments/payouts", requestBody, &response); err != nil {
		return demouc.Batch{}, err
	}
	if response.BatchHeader.ID == "" {
		return demouc.Batch{}, errors.New("PayPal create response missing payout batch ID")
	}
	return demouc.Batch{ID: response.BatchHeader.ID}, nil
}

func (c *Client) GetBatchItem(ctx context.Context, batchID string) (demouc.Item, error) {
	var response batchResponse
	if err := c.call(ctx, http.MethodGet, "/v1/payments/payouts/"+url.PathEscape(batchID), nil, &response); err != nil {
		return demouc.Item{}, err
	}
	if response.BatchHeader.ID != batchID {
		return demouc.Item{}, errors.New("PayPal payout batch identity mismatch")
	}
	if len(response.Items) == 0 {
		return demouc.Item{BatchID: batchID, Status: "PENDING"}, nil
	}
	if len(response.Items) != 1 {
		return demouc.Item{}, fmt.Errorf("expected one payout item, got %d", len(response.Items))
	}
	item := response.Items[0]
	return demouc.Item{ID: item.ID, BatchID: batchID, SenderItemID: item.PayoutItem.SenderItemID, Status: item.Status}, nil
}

func (c *Client) call(ctx context.Context, method, path string, body any, dst any) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.config.BaseURL, "/")+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("PayPal %s %s: %w", method, path, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("PayPal %s %s returned HTTP %d", method, path, response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(dst); err != nil {
		return fmt.Errorf("decode PayPal response: %w", err)
	}
	return nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.token != "" && time.Now().Before(c.tokenExpiry) {
		return c.token, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.config.BaseURL, "/")+"/v1/oauth2/token",
		strings.NewReader("grant_type=client_credentials"))
	if err != nil {
		return "", err
	}
	req.SetBasicAuth(c.config.ClientID, c.config.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	response, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("PayPal OAuth: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("PayPal OAuth returned HTTP %d", response.StatusCode)
	}
	var token struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&token); err != nil {
		return "", err
	}
	if token.AccessToken == "" {
		return "", errors.New("PayPal OAuth response missing access token")
	}
	c.token = token.AccessToken
	c.tokenExpiry = time.Now().Add(time.Duration(max(token.ExpiresIn-60, 0)) * time.Second)
	return c.token, nil
}
