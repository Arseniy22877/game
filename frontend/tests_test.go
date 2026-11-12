package main

import (
	"math"
	"strings"
	"testing"

	"github.com/Arseniy22877/game/gameengine"
)

func TestGameStateSystems(t *testing.T) {
	if !runUnitTests(gameengine.CreateTestGame()) {
		t.Fail()
	}
}

func TestAttributeCostUsesSelectedValue(t *testing.T) {
	game := gameengine.CreateFirstGame()
	state := game.CreateInstance()
	zero := []gameengine.AttributeChange{
		{AttributeID: 4, AttributeShift: 0},
		{AttributeID: 5, AttributeShift: 0},
		{AttributeID: 6, AttributeShift: 0},
	}

	if Cost := state.AttributeCost(zero); Cost != 0 {
		t.Fatalf("zero selected Attributes Cost %d, want 0", Cost)
	}

	half := []gameengine.AttributeChange{
		{AttributeID: 4, AttributeShift: 0.5},
		{AttributeID: 5, AttributeShift: 0.5},
		{AttributeID: 6, AttributeShift: 0.5},
	}
	expected := gameengine.CycleBaseCost + int(math.Round(0.5*18)) + int(math.Round(0.5*14)) + int(math.Round(0.5*16))
	if Cost := gameengine.CycleBaseCost + state.AttributeCost(half); Cost != expected {
		t.Fatalf("selected attribute Cost %d, want %d", Cost, expected)
	}
}

func TestStrategyAttributeEffectsStayInRange(t *testing.T) {
	game := gameengine.CreateFirstGame()
	state := game.CreateInstance()
	if err := state.UnlockStrategies([]gameengine.StrategyId{2}); err != nil {
		t.Fatal(err)
	}
	if err := state.UseStrategies([]gameengine.StrategyId{2}); err != nil {
		t.Fatal(err)
	}
	if err := state.CalculateFinals(); err != nil {
		t.Fatal(err)
	}
	for ID, attribute := range state.Attributes {
		if attribute.FinalValue < attribute.AttributeData.MinValue || attribute.FinalValue > attribute.AttributeData.MaxValue {
			t.Fatalf("attribute %d final value %.3f is outside range", ID, attribute.FinalValue)
		}
	}
}

func TestEventBudgetChangesAreSparseAndDocumented(t *testing.T) {
	game := gameengine.CreateFirstGame()
	changed := 0
	for _, event := range game.Events {
		change := event.BudgetChange
		if change.MoneyShift == 0 && change.QPShift == 0 {
			continue
		}
		changed++
		if !strings.Contains(event.Description, "кредит") || !strings.Contains(event.Description, "очк") {
			t.Fatalf("event %d changes gameengine.Budget without documenting both effects: %q", event.ID, event.Description)
		}
	}
	if changed > len(game.Events)/8 {
		t.Fatalf("too many Events change gameengine.Budget: %d of %d", changed, len(game.Events))
	}
}

func TestFirstGameHasPlayableBranches(t *testing.T) {
	game := gameengine.CreateFirstGame()
	if len(game.Events) != 110 || len(game.Hints) != 15 || len(game.Strategies) != 15 {
		t.Fatalf("unexpected first game size: Events=%d Hints=%d Strategies=%d", len(game.Events), len(game.Hints), len(game.Strategies))
	}
	eventByID := make(map[gameengine.EventId]gameengine.GameEvent, len(game.Events))
	for _, event := range game.Events {
		eventByID[event.ID] = event
	}

	state := game.CreateInstance()
	if state.Events[40].BaseStatus != gameengine.EventLockedStatus || state.Events[41].BaseStatus != gameengine.EventLockedStatus {
		t.Fatal("strategy-only Events must not be available in the base branch")
	}

	if err := state.UnlockStrategies([]gameengine.StrategyId{2, 8}); err != nil {
		t.Fatal(err)
	}
	if err := state.UseStrategies([]gameengine.StrategyId{2}); err != nil {
		t.Fatal(err)
	}
	if err := state.CalculateFinals(); err != nil {
		t.Fatal(err)
	}
	if state.Events[40].FinalStatus != gameengine.EventAvailableStatus || state.Events[41].FinalStatus != gameengine.EventLockedStatus {
		t.Fatal("security strategy should unlock only its positive branch event")
	}

	if err := state.UseStrategies([]gameengine.StrategyId{8}); err != nil {
		t.Fatal(err)
	}
	if err := state.CalculateFinals(); err != nil {
		t.Fatal(err)
	}
	if state.Events[40].FinalStatus != gameengine.EventLockedStatus || state.Events[41].FinalStatus != gameengine.EventAvailableStatus {
		t.Fatal("red-team strategy should unlock only its negative branch event")
	}

	descriptions := make(map[string]bool)
	for _, event := range game.Events {
		if len(event.CenterPoint) != 8 || len(event.BasicParameters.Weights) != 8 {
			t.Fatalf("event %d has invalid field dimensions", event.ID)
		}
		if descriptions[event.Description] {
			t.Fatalf("duplicate Description for event %d", event.ID)
		}
		descriptions[event.Description] = true
		if strings.Contains(event.Description, "В сводке смены есть несостыковка") {
			t.Fatalf("generic Description fragment remains in event %d", event.ID)
		}
		hasZero := false
		hasNuance := false
		for _, weight := range event.BasicParameters.Weights {
			switch weight {
			case 0:
				hasZero = true
			case 0.2, 0.5:
				hasNuance = true
			case 1:
			default:
				t.Fatalf("event %d has unsupported weight %v", event.ID, weight)
			}
		}
		if !hasZero || !hasNuance {
			t.Fatalf("event %d does not distinguish irrelevant and secondary Attributes", event.ID)
		}
	}
	independent := 0
	for _, event := range game.Events {
		if event.ID >= 55 && len(event.EventsUnlocked) == 0 {
			independent++
		}
	}
	if independent < len(game.Events)/2 {
		t.Fatalf("independent event pool is too small: %d of %d", independent, len(game.Events))
	}

	if game.Events[0].EventsUnlocked[0] != 3 || game.Events[3].EventsUnlocked[0] != 6 {
		t.Fatal("the primary research branch is not clearly connected")
	}

	strategyBranches := map[gameengine.StrategyId]gameengine.EventId{
		0: 42, 1: 43, 2: 40, 3: 44, 4: 45, 5: 46, 6: 47,
		7: 48, 8: 41, 9: 49, 10: 50, 11: 51, 12: 52, 13: 53, 14: 54,
	}
	positiveBranches := map[gameengine.EventId]bool{
		40: true, 41: false, 42: true, 43: false, 44: true, 45: false,
		46: true, 47: false, 48: true, 49: false, 50: true, 51: false,
		52: true, 53: false, 54: true,
	}
	if len(positiveBranches) != len(strategyBranches) {
		t.Fatal("strategy branch polarity table is incomplete")
	}
	for strategyID, eventID := range strategyBranches {
		if eventByID[eventID].IsPositive != positiveBranches[eventID] {
			t.Fatalf("unexpected polarity for strategy-only event %d (%s): got %v want %v", eventID, eventByID[eventID].Name, eventByID[eventID].IsPositive, positiveBranches[eventID])
		}
		withoutStrategy := game.CreateInstance()
		if withoutStrategy.Events[eventID].BaseStatus != gameengine.EventLockedStatus {
			t.Fatalf("strategy-only event %d is available without strategy %d", eventID, strategyID)
		}

		withStrategy := game.CreateInstance()
		if err := withStrategy.UnlockStrategies([]gameengine.StrategyId{strategyID}); err != nil {
			t.Fatal(err)
		}
		if err := withStrategy.UseStrategies([]gameengine.StrategyId{strategyID}); err != nil {
			t.Fatal(err)
		}
		if err := withStrategy.CalculateFinals(); err != nil {
			t.Fatal(err)
		}
		if withStrategy.Events[eventID].FinalStatus != gameengine.EventAvailableStatus {
			t.Fatalf("strategy %d does not unlock event %d", strategyID, eventID)
		}
	}

	defaultState := game.CreateInstance()
	defaultProbabilities, err := defaultState.GetProbabilitiesForEvents()
	if err != nil {
		t.Fatal(err)
	}
	zeroState := game.CreateInstance()
	zeroChanges := make([]gameengine.AttributeChange, 0, len(game.ConfigurableAttributes))
	for _, attribute := range game.ConfigurableAttributes {
		zeroChanges = append(zeroChanges, gameengine.AttributeChange{AttributeID: attribute.ID, AttributeShift: 0})
	}
	if err := zeroState.UseAttributes(zeroChanges); err != nil {
		t.Fatal(err)
	}
	if err := zeroState.CalculateFinals(); err != nil {
		t.Fatal(err)
	}
	zeroProbabilities, err := zeroState.GetProbabilitiesForEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultProbabilities) != len(zeroProbabilities) {
		t.Fatalf("zero profile changed available event count: default=%d zero=%d", len(defaultProbabilities), len(zeroProbabilities))
	}
	defaultByID := make(map[gameengine.EventId]float64)
	for _, probability := range defaultProbabilities {
		defaultByID[probability.Attractor.EventState.EventData.ID] = probability.Probability
	}
	different := 0
	for _, probability := range zeroProbabilities {
		eventID := probability.Attractor.EventState.EventData.ID
		if defaultByID[eventID] != probability.Probability {
			different++
		}
	}
	if different < len(defaultProbabilities)/2 {
		t.Fatalf("zero profile barely changes event probabilities: only %d of %d differ", different, len(defaultProbabilities))
	}

	for _, event := range game.Events {
		if len(event.AttributeChanges) == 0 {
			t.Fatalf("event %d has no cumulative base-attribute effect", event.ID)
		}
		for _, change := range event.AttributeChanges {
			if event.IsPositive && change.AttributeShift <= 0 {
				t.Fatalf("positive event %d lowers a base attribute", event.ID)
			}
			if !event.IsPositive && change.AttributeShift >= 0 {
				t.Fatalf("negative event %d raises a base attribute", event.ID)
			}
		}
	}

	cumulativeState := game.CreateInstance()
	beforeSuccess := cumulativeState.Attributes[0].BaseValue
	successEvent := cumulativeState.Events[0]
	if err := successEvent.Fire(&cumulativeState); err != nil {
		t.Fatal(err)
	}
	if cumulativeState.Attributes[0].BaseValue <= beforeSuccess {
		t.Fatal("successful event did not accumulate team success")
	}
	beforeFailure := cumulativeState.Attributes[2].BaseValue
	failureEvent := cumulativeState.Events[2]
	if err := failureEvent.Fire(&cumulativeState); err != nil {
		t.Fatal(err)
	}
	if cumulativeState.Attributes[2].BaseValue >= beforeFailure {
		t.Fatal("negative event did not reduce accumulated team success")
	}
}
