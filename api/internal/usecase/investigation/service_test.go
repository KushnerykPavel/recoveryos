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
