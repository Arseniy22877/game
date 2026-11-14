package api

import (
	"encoding/json"
	"testing"
)

// Встроенные структуры должны давать плоский JSON, как описано в docs/api.md.
func TestEmbeddedStructsAreFlat(t *testing.T) {
	var req SubmitDecisionRequest
	body := `{"round":2,"attributes":[{"id":10,"value":0.4}],"strategies":[1],"hints":[],"report":{"why":"w","alternative":"a","risks":"r","correction":"c"}}`
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatal(err)
	}
	if req.Round != 2 || len(req.Attributes) != 1 || req.Attributes[0].Value != 0.4 || req.Report.Correction != "c" {
		t.Fatalf("unexpected decode: %+v", req)
	}

	out, err := json.Marshal(AdminTeam{TeamSummary: TeamSummary{ID: 1, Name: "x"}})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	_ = json.Unmarshal(out, &m)
	for _, k := range []string{"id", "name", "submitted", "state", "predictions"} {
		if _, ok := m[k]; !ok {
			t.Errorf("AdminTeam JSON has no %q: %s", k, out)
		}
	}
}
