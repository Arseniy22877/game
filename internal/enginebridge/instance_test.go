package enginebridge_test

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/enginebridge"
)

func TestCatalog(t *testing.T) {
	c, err := enginebridge.LoadCatalog("../../scenarios")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"republic_n_scenario", "republic_n_scenario_v2"} {
		s, ok := c.Get(id)
		if !ok || len(s.Raw()) == 0 || s.Opening() == nil || s.Opening().ID != 0 {
			t.Fatal(id)
		}
		if s.Opening().ImageURL == "" {
			t.Fatal("missing opening image", id)
		}
	}
}

func TestRound(t *testing.T) {
	raw, err := os.ReadFile("testdata/full.json")
	if err != nil {
		t.Fatal(err)
	}
	scenario, err := enginebridge.ParseScenario("small", raw)
	if err != nil {
		t.Fatal(err)
	}
	i, other := scenario.NewInstance(), scenario.NewInstance()
	i.SetRound(1)
	initial := i.State()
	bytes, _ := json.Marshal(initial)
	if strings.Contains(string(bytes), "null") || strings.Contains(string(bytes), "secret\":\"secret") {
		t.Fatal(string(bytes))
	}
	d := i.DefaultDecision()
	d.Strategies = []int{2}
	d.Hints = []int{3}
	cost, err := i.Submit(d)
	if err != nil || cost != (api.Cost{Money: 10, QP: 2}) {
		t.Fatal(cost, err)
	}
	if _, err = i.Submit(api.Decision{}); err == nil {
		t.Fatal("incomplete sliders accepted")
	}
	bad := d
	bad.Hints = []int{99}
	if _, err = i.Submit(bad); err == nil || !reflect.DeepEqual(initial, i.State()) {
		t.Fatal("failed submit changed state")
	}
	p, err := i.Calculate()
	if err != nil {
		t.Fatal(err)
	}
	after := i.State()
	if initial.Budget.Money-after.Budget.Money != cost.Money || after.Budget.QP != 1 || after.Hints[0].Secret != "secret" || !reflect.DeepEqual(other.State(), initial) {
		t.Fatal(after)
	}
	if len(p) != 4 {
		t.Fatal(p)
	}
	for j, w := range []float64{1, .75, .25, 0} {
		if p[j].Probability != w {
			t.Fatal(p)
		}
	}
	if len(i.Suggest()) != 4 {
		t.Fatal("suggest dropped candidates")
	}
	i.SetRound(2)
	d = i.DefaultDecision()
	if d.Attributes[0].Value != 1 || i.Bankrupt() {
		t.Fatal(d)
	}
	d.Hints = []int{4}
	if _, err = i.Submit(d); err == nil {
		t.Fatal("hint requires retained strategy")
	}
	d.Hints = nil
	if _, err = i.Submit(d); err != nil {
		t.Fatal(err)
	}
	p, err = i.Calculate()
	if err != nil || len(p) != 5 || !i.Bankrupt() {
		t.Fatal(p, err)
	}
	before := i.State().Budget
	i.SetRound(3)
	if _, err = i.SkipRound(); err != nil || i.State().Budget != before {
		t.Fatal("skip", err)
	}
}

func TestInsufficientFunds(t *testing.T) {
	raw, err := os.ReadFile("testdata/full.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		field string
		code  string
	}{{"InitialMoney", api.CodeInsufficientMoney}, {"InitialQP", api.CodeInsufficientQP}} {
		var def map[string]json.RawMessage
		if err := json.Unmarshal(raw, &def); err != nil {
			t.Fatal(err)
		}
		var budget map[string]int
		if err := json.Unmarshal(def["StartingBudget"], &budget); err != nil {
			t.Fatal(err)
		}
		budget[tc.field] = 0
		def["StartingBudget"], err = json.Marshal(budget)
		if err != nil {
			t.Fatal(err)
		}
		data, err := json.Marshal(def)
		if err != nil {
			t.Fatal(err)
		}
		s, err := enginebridge.ParseScenario("test", data)
		if err != nil {
			t.Fatal(err)
		}
		i := s.NewInstance()
		d := i.DefaultDecision()
		d.Hints = []int{3}
		_, err = i.Submit(d)
		var decisionErr *enginebridge.DecisionError
		if !errors.As(err, &decisionErr) || decisionErr.Code != tc.code {
			t.Fatalf("%s: %v", tc.field, err)
		}
	}
}
