package gameengine

import "testing"

func TestSimpleProbabilityPrerequisitesAndThresholds(t *testing.T) {
	game := Game{
		BasicAttributes: []GameAttribute{
			{ID: 10, DefaultValue: 0.6, MinValue: 0, MaxValue: 1},
		},
		Events: []GameEvent{
			{ID: 1, Iteration: 0, BasicPower: 0.2, Weights: Vector{0.5}},
			{ID: 2, Iteration: 0, BasicPower: 0.9, Prerequisites: Prerequisite{Events: []EventId{1}}, Thresholds: []Threshold{{AttributeID: 10, Min: 0.5, Max: 1}}},
		},
	}
	state := game.CreateInstance()
	predictions, err := state.GetPredicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(predictions) != 1 || predictions[0].Event.EventData.ID != 1 {
		t.Fatalf("unexpected initial predictions: %#v", predictions)
	}
	if err := state.FireEvents([]int{1}); err != nil {
		t.Fatal(err)
	}
	predictions, err = state.GetPredicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(predictions) != 1 || predictions[0].Event.EventData.ID != 2 {
		t.Fatalf("unexpected predictions after prerequisite: %#v", predictions)
	}
}

func TestPickEventsSelectsOnePerBranch(t *testing.T) {
	game := Game{
		Branches: []Branch{"main", "side"},
		Events: []GameEvent{
			{ID: 1, Branch: "main", Iteration: 0, BasicPower: 1},
			{ID: 2, Branch: "main", Iteration: 0, BasicPower: 3},
			{ID: 3, Branch: "side", Iteration: 0, BasicPower: 2},
		},
	}
	state := game.CreateInstance()
	predictions, err := state.GetPredicts()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := state.PickEvents(predictions)
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 2 {
		t.Fatalf("selected %d events, want 2", len(selected))
	}
}

func TestStrategyAndHintPrerequisitesAreCheckedLive(t *testing.T) {
	game := Game{
		Events:     []GameEvent{{ID: 1, Iteration: 0, BasicPower: 1}},
		Strategies: []Strategy{{ID: 2, Prerequisites: Prerequisite{Events: []EventId{1}}}},
		Hints:      []Hint{{ID: 3, Prerequisites: Prerequisite{Events: []EventId{1}}}},
	}
	state := game.CreateInstance()
	if err := state.PassStrategies([]int{2}); err == nil {
		t.Fatal("strategy was accepted before prerequisite")
	}
	if err := state.PassHints([]int{3}); err == nil {
		t.Fatal("hint was accepted before prerequisite")
	}
}
