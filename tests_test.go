package main

import (
	"math"
	"strings"
	"testing"
)

func TestGameStateSystems(t *testing.T) {
	if !runUnitTests(createTestGame()) {
		t.Fail()
	}
}

func TestAttributeCostUsesSelectedValue(t *testing.T) {
	game := createFirstGame()
	state := game.createInstance()
	zero := []AttributeChange{
		{attributeId: 4, attributeShift: 0},
		{attributeId: 5, attributeShift: 0},
		{attributeId: 6, attributeShift: 0},
	}

	if cost := state.attributeCost(zero); cost != 0 {
		t.Fatalf("zero selected attributes cost %d, want 0", cost)
	}

	half := []AttributeChange{
		{attributeId: 4, attributeShift: 0.5},
		{attributeId: 5, attributeShift: 0.5},
		{attributeId: 6, attributeShift: 0.5},
	}
	expected := cycleBaseCost + int(math.Round(0.5*18)) + int(math.Round(0.5*14)) + int(math.Round(0.5*16))
	if cost := cycleBaseCost + state.attributeCost(half); cost != expected {
		t.Fatalf("selected attribute cost %d, want %d", cost, expected)
	}
}

func TestStrategyAttributeEffectsStayInRange(t *testing.T) {
	game := createFirstGame()
	state := game.createInstance()
	if err := state.unlockStrategies([]StrategyId{2}); err != nil {
		t.Fatal(err)
	}
	if err := state.useStrategies([]StrategyId{2}); err != nil {
		t.Fatal(err)
	}
	if err := state.calcFinals(); err != nil {
		t.Fatal(err)
	}
	for id, attribute := range state.attributes {
		if attribute.finalValue < attribute._attributeData.minValue || attribute.finalValue > attribute._attributeData.maxValue {
			t.Fatalf("attribute %d final value %.3f is outside range", id, attribute.finalValue)
		}
	}
}

func TestEventBudgetChangesAreSparseAndDocumented(t *testing.T) {
	game := createFirstGame()
	changed := 0
	for _, event := range game.events {
		change := event.budgetChange
		if change.moneyShift == 0 && change.qpShift == 0 {
			continue
		}
		changed++
		if !strings.Contains(event.description, "кредит") || !strings.Contains(event.description, "очк") {
			t.Fatalf("event %d changes budget without documenting both effects: %q", event.id, event.description)
		}
	}
	if changed > len(game.events)/8 {
		t.Fatalf("too many events change budget: %d of %d", changed, len(game.events))
	}
}

func TestFirstGameHasPlayableBranches(t *testing.T) {
	game := createFirstGame()
	if len(game.events) != 110 || len(game.hints) != 15 || len(game.strategies) != 15 {
		t.Fatalf("unexpected first game size: events=%d hints=%d strategies=%d", len(game.events), len(game.hints), len(game.strategies))
	}
	eventByID := make(map[EventId]GameEvent, len(game.events))
	for _, event := range game.events {
		eventByID[event.id] = event
	}

	state := game.createInstance()
	if state.events[40].baseStatus != EventLockedStatus || state.events[41].baseStatus != EventLockedStatus {
		t.Fatal("strategy-only events must not be available in the base branch")
	}

	if err := state.unlockStrategies([]StrategyId{2, 8}); err != nil {
		t.Fatal(err)
	}
	if err := state.useStrategies([]StrategyId{2}); err != nil {
		t.Fatal(err)
	}
	if err := state.calcFinals(); err != nil {
		t.Fatal(err)
	}
	if state.events[40].finalStatus != EventAvailableStatus || state.events[41].finalStatus != EventLockedStatus {
		t.Fatal("security strategy should unlock only its positive branch event")
	}

	if err := state.useStrategies([]StrategyId{8}); err != nil {
		t.Fatal(err)
	}
	if err := state.calcFinals(); err != nil {
		t.Fatal(err)
	}
	if state.events[40].finalStatus != EventLockedStatus || state.events[41].finalStatus != EventAvailableStatus {
		t.Fatal("red-team strategy should unlock only its negative branch event")
	}

	descriptions := make(map[string]bool)
	for _, event := range game.events {
		if len(event.centerPoint) != 8 || len(event.basicParameters.weights) != 8 {
			t.Fatalf("event %d has invalid field dimensions", event.id)
		}
		if descriptions[event.description] {
			t.Fatalf("duplicate description for event %d", event.id)
		}
		descriptions[event.description] = true
		if strings.Contains(event.description, "В сводке смены есть несостыковка") {
			t.Fatalf("generic description fragment remains in event %d", event.id)
		}
		hasZero := false
		hasNuance := false
		for _, weight := range event.basicParameters.weights {
			switch weight {
			case 0:
				hasZero = true
			case 0.2, 0.5:
				hasNuance = true
			case 1:
			default:
				t.Fatalf("event %d has unsupported weight %v", event.id, weight)
			}
		}
		if !hasZero || !hasNuance {
			t.Fatalf("event %d does not distinguish irrelevant and secondary attributes", event.id)
		}
	}
	independent := 0
	for _, event := range game.events {
		if event.id >= 55 && len(event.eventsUnlocked) == 0 {
			independent++
		}
	}
	if independent < len(game.events)/2 {
		t.Fatalf("independent event pool is too small: %d of %d", independent, len(game.events))
	}

	if game.events[0].eventsUnlocked[0] != 3 || game.events[3].eventsUnlocked[0] != 6 {
		t.Fatal("the primary research branch is not clearly connected")
	}

	strategyBranches := map[StrategyId]EventId{
		0: 42, 1: 43, 2: 40, 3: 44, 4: 45, 5: 46, 6: 47,
		7: 48, 8: 41, 9: 49, 10: 50, 11: 51, 12: 52, 13: 53, 14: 54,
	}
	positiveBranches := map[EventId]bool{
		40: true, 41: false, 42: true, 43: false, 44: true, 45: false,
		46: true, 47: false, 48: true, 49: false, 50: true, 51: false,
		52: true, 53: false, 54: true,
	}
	if len(positiveBranches) != len(strategyBranches) {
		t.Fatal("strategy branch polarity table is incomplete")
	}
	for strategyID, eventID := range strategyBranches {
		if eventByID[eventID].isPositive != positiveBranches[eventID] {
			t.Fatalf("unexpected polarity for strategy-only event %d (%s): got %v want %v", eventID, eventByID[eventID].name, eventByID[eventID].isPositive, positiveBranches[eventID])
		}
		withoutStrategy := game.createInstance()
		if withoutStrategy.events[eventID].baseStatus != EventLockedStatus {
			t.Fatalf("strategy-only event %d is available without strategy %d", eventID, strategyID)
		}

		withStrategy := game.createInstance()
		if err := withStrategy.unlockStrategies([]StrategyId{strategyID}); err != nil {
			t.Fatal(err)
		}
		if err := withStrategy.useStrategies([]StrategyId{strategyID}); err != nil {
			t.Fatal(err)
		}
		if err := withStrategy.calcFinals(); err != nil {
			t.Fatal(err)
		}
		if withStrategy.events[eventID].finalStatus != EventAvailableStatus {
			t.Fatalf("strategy %d does not unlock event %d", strategyID, eventID)
		}
	}

	defaultState := game.createInstance()
	defaultProbabilities, err := defaultState.getProbabilitiesForEvents()
	if err != nil {
		t.Fatal(err)
	}
	zeroState := game.createInstance()
	zeroChanges := make([]AttributeChange, 0, len(game.configurableAttributes))
	for _, attribute := range game.configurableAttributes {
		zeroChanges = append(zeroChanges, AttributeChange{attributeId: attribute.id, attributeShift: 0})
	}
	if err := zeroState.useAttributes(zeroChanges); err != nil {
		t.Fatal(err)
	}
	if err := zeroState.calcFinals(); err != nil {
		t.Fatal(err)
	}
	zeroProbabilities, err := zeroState.getProbabilitiesForEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(defaultProbabilities) != len(zeroProbabilities) {
		t.Fatalf("zero profile changed available event count: default=%d zero=%d", len(defaultProbabilities), len(zeroProbabilities))
	}
	defaultByID := make(map[EventId]float64)
	for _, probability := range defaultProbabilities {
		defaultByID[probability._attractor._eventState._eventData.id] = probability.probability
	}
	different := 0
	for _, probability := range zeroProbabilities {
		eventID := probability._attractor._eventState._eventData.id
		if defaultByID[eventID] != probability.probability {
			different++
		}
	}
	if different < len(defaultProbabilities)/2 {
		t.Fatalf("zero profile barely changes event probabilities: only %d of %d differ", different, len(defaultProbabilities))
	}

	for _, event := range game.events {
		if len(event.attributeChanges) == 0 {
			t.Fatalf("event %d has no cumulative base-attribute effect", event.id)
		}
		for _, change := range event.attributeChanges {
			if event.isPositive && change.attributeShift <= 0 {
				t.Fatalf("positive event %d lowers a base attribute", event.id)
			}
			if !event.isPositive && change.attributeShift >= 0 {
				t.Fatalf("negative event %d raises a base attribute", event.id)
			}
		}
	}

	cumulativeState := game.createInstance()
	beforeSuccess := cumulativeState.attributes[0].baseValue
	successEvent := cumulativeState.events[0]
	if err := successEvent.fire(&cumulativeState); err != nil {
		t.Fatal(err)
	}
	if cumulativeState.attributes[0].baseValue <= beforeSuccess {
		t.Fatal("successful event did not accumulate team success")
	}
	beforeFailure := cumulativeState.attributes[2].baseValue
	failureEvent := cumulativeState.events[2]
	if err := failureEvent.fire(&cumulativeState); err != nil {
		t.Fatal(err)
	}
	if cumulativeState.attributes[2].baseValue >= beforeFailure {
		t.Fatal("negative event did not reduce accumulated team success")
	}
}
