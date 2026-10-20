package app

import (
	"errors"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/KushnerykPavel/recoveryos/api/internal/service/paypal"
)

type Config struct {
	Address          string
	DatabaseURL      string
	CookieSecure     bool
	DailyPayoutLimit int
	PollInterval     time.Duration
	WaitTimeout      time.Duration
	PayPal           paypal.Config
}

func LoadConfig() (Config, error) {
	config := Config{
		Address:      envDefault("ADDRESS", "127.0.0.1:8080"),
		DatabaseURL:  os.Getenv("DB_URL"),
		PollInterval: time.Second,
		WaitTimeout:  25 * time.Second,
		PayPal: paypal.Config{
			BaseURL:      "https://api-m.sandbox.paypal.com",
			ClientID:     os.Getenv("PAYPAL_CLIENT_ID"),
			ClientSecret: os.Getenv("PAYPAL_CLIENT_SECRET"),
			Receiver:     os.Getenv("PAYPAL_SANDBOX_RECEIVER"),
			Amount:       envDefault("PAYOUT_AMOUNT", "0.01"),
			Currency:     envDefault("PAYOUT_CURRENCY", "USD"),
		},
	}
	secure, err := strconv.ParseBool(envDefault("COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, errors.New("COOKIE_SECURE must be true or false")
	}
	config.CookieSecure = secure
	limit, err := strconv.Atoi(envDefault("DAILY_PAYOUT_LIMIT", "10"))
	if err != nil || limit < 1 {
		return Config{}, errors.New("DAILY_PAYOUT_LIMIT must be a positive integer")
	}
	config.DailyPayoutLimit = limit
	if config.DatabaseURL == "" || config.PayPal.ClientID == "" ||
		config.PayPal.ClientSecret == "" || config.PayPal.Receiver == "" {
		return Config{}, errors.New("DB_URL, PAYPAL_CLIENT_ID, PAYPAL_CLIENT_SECRET and PAYPAL_SANDBOX_RECEIVER are required")
	}
	if strings.Contains(config.PayPal.Receiver, " ") || !strings.Contains(config.PayPal.Receiver, "@") {
		return Config{}, errors.New("PAYPAL_SANDBOX_RECEIVER must be an email address")
	}
	amount, err := strconv.ParseFloat(config.PayPal.Amount, 64)
	if err != nil || amount <= 0 || math.IsNaN(amount) || math.IsInf(amount, 0) || len(config.PayPal.Currency) != 3 {
		return Config{}, errors.New("PAYOUT_AMOUNT must be positive and PAYOUT_CURRENCY must have three letters")
	}
	for _, letter := range config.PayPal.Currency {
		if letter < 'A' || letter > 'Z' {
			return Config{}, errors.New("PAYOUT_CURRENCY must have three uppercase letters")
		}
	}
	return config, nil
}

func envDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
