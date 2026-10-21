package recovery

import "time"

const (
	ScenarioStandard     = "standard"
	ScenarioMissedUpdate = "missed-update"
)

// Session is the anonymous visitor's isolated workspace.
type Session struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Payout is the internal operational record, not an accounting ledger.
type Payout struct {
	ID             string    `json:"id"`
	SessionID      string    `json:"-"`
	CreationKey    string    `json:"creation_key"`
	SenderItemID   string    `json:"sender_item_id"`
	BatchID        string    `json:"batch_id,omitempty"`
	ItemID         string    `json:"item_id,omitempty"`
	ProviderStatus string    `json:"provider_status,omitempty"`
	Status         string    `json:"status"`
	Scenario       string    `json:"scenario"`
	Origin         string    `json:"origin"`
	CreatedAt      time.Time `json:"created_at"`
}

type Incident struct {
	ID        string    `json:"id"`
	PayoutID  string    `json:"payout_id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Event struct {
	ID         int64     `json:"id"`
	Source     string    `json:"source"`
	Kind       string    `json:"kind"`
	Details    string    `json:"details"`
	OccurredAt time.Time `json:"occurred_at"`
}

type Evidence struct {
	Source    string `json:"source"`
	Reference string `json:"reference"`
	Finding   string `json:"finding"`
}

type Investigation struct {
	IncidentID     string     `json:"incident_id"`
	ItemID         string     `json:"item_id"`
	ItemStatus     string     `json:"item_status"`
	Diagnosis      string     `json:"diagnosis"`
	ProposedAction string     `json:"proposed_action"`
	Evidence       []Evidence `json:"evidence"`
	Origin         string     `json:"origin"`
}

type Snapshot struct {
	Session       Session        `json:"session"`
	Payout        *Payout        `json:"payout,omitempty"`
	Incident      *Incident      `json:"incident,omitempty"`
	Investigation *Investigation `json:"investigation,omitempty"`
	Events        []Event        `json:"events"`
}
