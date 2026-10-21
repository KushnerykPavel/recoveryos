package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/KushnerykPavel/recoveryos/api/internal/usecase/investigation"
)

type Client struct {
	APIKey  string
	Model   string
	BaseURL string
	HTTP    *http.Client
}

func New(apiKey, model string, client *http.Client) *Client {
	if model == "" {
		model = "gpt-6-luna"
	}
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Client{APIKey: apiKey, Model: model, BaseURL: "https://api.openai.com", HTTP: client}
}

func (c *Client) Respond(ctx context.Context, input investigation.Request) (investigation.Response, error) {
	if c.APIKey == "" {
		return investigation.Response{}, errors.New("OpenAI API key is not configured")
	}
	body, err := json.Marshal(map[string]any{
		"model": c.Model, "instructions": input.Instructions, "input": input.Input,
		"tools": input.Tools, "tool_choice": input.ToolChoice, "text": input.Text,
		"store": false, "parallel_tool_calls": true, "reasoning": map[string]string{"effort": "none"},
	})
	if err != nil {
		return investigation.Response{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.BaseURL, "/")+"/v1/responses", bytes.NewReader(body))
	if err != nil {
		return investigation.Response{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return investigation.Response{}, fmt.Errorf("OpenAI Responses request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return investigation.Response{}, fmt.Errorf("OpenAI Responses returned HTTP %d", resp.StatusCode)
	}
	var result investigation.Response
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result); err != nil {
		return investigation.Response{}, fmt.Errorf("decode OpenAI Responses result: %w", err)
	}
	return result, nil
}
