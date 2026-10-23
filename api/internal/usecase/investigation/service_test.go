package investigation

import (
	"reflect"
	"testing"

	"github.com/KushnerykPavel/recoveryos/api/internal/domain/recovery"
	recoveryuc "github.com/KushnerykPavel/recoveryos/api/internal/usecase/recovery"
)

func TestResultFormatRestrictsEvidenceToExactSourceReferences(t *testing.T) {
	snapshot := recovery.Snapshot{
		Incident: &recovery.Incident{ID: "incident-1"},
		Payout:   &recovery.Payout{ID: "payout-1"},
		Events: []recovery.Event{
			{ID: 42, Source: "injected"},
			{ID: 43, Source: "system"},
		},
	}
	format := resultFormat(snapshot, recoveryuc.Item{ID: "item-1"})["format"].(map[string]any)
	schema := format["schema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	evidence := properties["evidence"].(map[string]any)
	items := evidence["items"].(map[string]any)
	options := items["anyOf"].([]map[string]any)
	got := make(map[string][]string, len(options))
	for _, option := range options {
		fields := option["properties"].(map[string]any)
		source := fields["source"].(map[string]any)["enum"].([]string)[0]
		got[source] = fields["reference"].(map[string]any)["enum"].([]string)
	}
	want := map[string][]string{
		"provider": {"item-1"},
		"internal": {"payout-1", "incident-1"},
		"policy":   {"record-correction-policy"},
		"injected": {"42"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("evidence source/reference combinations = %v, want %v", got, want)
	}
}

func TestWebhookCanBeCitedWithoutReplacingCurrentItemEvidence(t *testing.T) {
	snapshot := recovery.Snapshot{
		Incident: &recovery.Incident{ID: "incident-1"},
		Payout:   &recovery.Payout{ID: "payout-1", Status: "pending"},
		Events: []recovery.Event{
			{ID: 42, Source: "injected"},
			{ID: 44, Source: "paypal_webhook"},
		},
	}
	item := recoveryuc.Item{ID: "item-1", Status: "SUCCESS"}
	format := resultFormat(snapshot, item)["format"].(map[string]any)
	schema := format["schema"].(map[string]any)
	properties := schema["properties"].(map[string]any)
	evidence := properties["evidence"].(map[string]any)
	options := evidence["items"].(map[string]any)["anyOf"].([]map[string]any)
	providerReferences := options[0]["properties"].(map[string]any)["reference"].(map[string]any)["enum"].([]string)
	if !reflect.DeepEqual(providerReferences, []string{"item-1", "44"}) {
		t.Fatalf("provider references = %v", providerReferences)
	}
	result := recovery.Investigation{
		IncidentID: "incident-1", ItemID: "item-1", ItemStatus: "SUCCESS",
		Diagnosis: "Verified payout item succeeded", ProposedAction: "correct_payout_record",
		Evidence: []recovery.Evidence{
			{Source: "provider", Reference: "item-1", Finding: "Current item SUCCESS"},
			{Source: "provider", Reference: "44", Finding: "Signed webhook"},
			{Source: "internal", Reference: "payout-1", Finding: "Pending record"},
			{Source: "injected", Reference: "42", Finding: "Skipped update"},
			{Source: "policy", Reference: "record-correction-policy", Finding: "Correct record only"},
		},
	}
	if err := validate(result, snapshot, item); err != nil {
		t.Fatalf("valid webhook citation rejected: %v", err)
	}
	result.Evidence = result.Evidence[1:]
	if err := validate(result, snapshot, item); err == nil {
		t.Fatal("webhook citation replaced required current item evidence")
	}
}
