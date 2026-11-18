package gameengine

import (
	"errors"
	"reflect"
	"testing"
)

func eventSelectionScenario() Game {
	return Game{
		BasicAttributes: []GameAttribute{{ID: 1, DefaultValue: 8, MinValue: 0, MaxValue: 10}},
		StartingBudget:  Budget{InitialMoney: 8, MaxMoney: 10, InitialQP: 2},
		Branches:        []Branch{"economy", "politics"},
		OpeningEvent:    10,
		Events: []GameEvent{
			{
				ID: 1, Branch: "economy", Weights: Vector{0}, BasicPower: 1,
				AttributeChanges: []AttributeChange{{AttributeID: 1, AttributeShift: 5}},
				BudgetChange:     BudgetChange{MoneyShift: 5, QPShift: -4},
			},
			{
				ID: 2, Branch: "economy", Weights: Vector{0}, BasicPower: 1,
				Thresholds:       []Threshold{{AttributeID: 1, Min: 8, Max: 8}},
				AttributeChanges: []AttributeChange{{AttributeID: 1, AttributeShift: -4}},
				BudgetChange:     BudgetChange{MoneyShift: -4, QPShift: 3},
			},
			{
				ID: 3, Branch: "politics", Weights: Vector{0}, BasicPower: 1,
				AttributeChanges: []AttributeChange{{AttributeID: 1, AttributeShift: -4}},
				BudgetChange:     BudgetChange{MoneyShift: -4},
			},
			{
				ID: 4, Branch: "politics", Weights: Vector{0}, BasicPower: 1,
				Prerequisites: Prerequisite{Events: []EventId{1}},
				BudgetChange:  BudgetChange{MoneyShift: 2},
			},
			{ID: 5, Branch: "politics", Weights: Vector{0}, BasicPower: 1, Iteration: 2},
			{ID: 10, Branch: "politics", Weights: Vector{0}, BasicPower: 1},
		},
	}
}

func TestFireSelectedEventsExactSelection(t *testing.T) {
	tests := []struct {
		name   string
		ids    []int
		want   int // attribute and money have the same initial value and shifts
		wantQP int
	}{
		{name: "empty", ids: []int{}, want: 8, wantQP: 2},
		{name: "nil", ids: nil, want: 8, wantQP: 2},
		{name: "partial", ids: []int{1}, want: 10, wantQP: 0},
		// Both events start available. Event 1 takes the attribute outside event 2's threshold.
		{name: "same branch and upper bounds", ids: []int{1, 2}, want: 6, wantQP: 3},
		{name: "reverse order", ids: []int{2, 1}, want: 9, wantQP: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := eventSelectionScenario()
			state := game.CreateInstance()
			state.Iteration = 1

			if err := state.FireSelectedEvents(tt.ids); err != nil {
				t.Fatalf("FireSelectedEvents(%v): %v", tt.ids, err)
			}
			attr := state.Attributes[1]
			if attr.BaseValue != float64(tt.want) || attr.FinalValue != float64(tt.want) {
				t.Errorf("attribute base/final = %v/%v, want %d/%d", attr.BaseValue, attr.FinalValue, tt.want, tt.want)
			}
			if state.Budget.Money != tt.want || state.Budget.QP != tt.wantQP {
				t.Errorf("money/QP = %d/%d, want %d/%d", state.Budget.Money, state.Budget.QP, tt.want, tt.wantQP)
			}
			wantStatuses := map[EventId]EventStatus{10: EventExtinguishedStatus}
			for _, id := range tt.ids {
				wantStatuses[EventId(id)] = EventFiredStatus
			}
			for id, event := range state.Events {
				if want := wantStatuses[id]; event.Status != want {
					t.Errorf("event %d status = %v, want %v", id, event.Status, want)
				}
			}
			if state.Iteration != 1 {
				t.Errorf("iteration = %d, want 1", state.Iteration)
			}
		})
	}
}

func TestFireSelectedEventsRejectsInvalidSelectionAtomically(t *testing.T) {
	tests := []struct {
		name    string
		second  int
		wantErr error
	}{
		{name: "duplicate", second: 1},
		{name: "unknown", second: 99, wantErr: ErrInvalidEventId},
		{name: "already fired", second: 10, wantErr: ErrEventNotAvailable},
		{name: "prerequisite unlocked by first event", second: 4, wantErr: ErrEventNotAvailable},
		{name: "wrong iteration", second: 5, wantErr: ErrEventNotAvailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			game := eventSelectionScenario()
			state, want := game.CreateInstance(), game.CreateInstance()
			state.Iteration, want.Iteration = 1, 1

			err := state.FireSelectedEvents([]int{1, tt.second})
			if err == nil || (tt.wantErr != nil && !errors.Is(err, tt.wantErr)) {
				t.Errorf("FireSelectedEvents([1 %d]) error = %v, want rejection (sentinel: %v)", tt.second, err, tt.wantErr)
			}
			if !reflect.DeepEqual(state, want) {
				t.Errorf("state changed on rejection: %+v, want %+v", state, want)
			}
		})
	}
}

func TestFireSelectedEventsRollsBackFailedEffect(t *testing.T) {
	game := eventSelectionScenario()
	game.Events[1].AttributeChanges[0].AttributeID = 99
	state, want := game.CreateInstance(), game.CreateInstance()

	if err := state.FireSelectedEvents([]int{1, 2}); !errors.Is(err, ErrInvalidAttributeId) {
		t.Errorf("FireSelectedEvents([1 2]) error = %v, want %v", err, ErrInvalidAttributeId)
	}
	if !reflect.DeepEqual(state, want) {
		t.Errorf("state changed after a failed effect: %+v, want %+v", state, want)
	}
}

func TestFireSelectedEventsReplay(t *testing.T) {
	game, replayGame := eventSelectionScenario(), eventSelectionScenario()
	state, replay := game.CreateInstance(), replayGame.CreateInstance()
	for i, ids := range [][]int{{1, 2}, {}, {4}} {
		round := i + 1
		state.Iteration, replay.Iteration = round, round
		if err := state.FireSelectedEvents(ids); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if err := replay.FireSelectedEvents(ids); err != nil {
			t.Fatalf("replay round %d: %v", round, err)
		}
		if !reflect.DeepEqual(state, replay) {
			t.Fatalf("round %d replay = %+v, want %+v", round, replay, state)
		}
	}
	if state.Events[1].Status != EventExtinguishedStatus || state.Events[4].Status != EventFiredStatus {
		t.Errorf("unexpected event statuses after replay: %v", state.Events)
	}
}

func TestFireEventsFillsUnselectedBranchesAndCombinesEffects(t *testing.T) {
	game := eventSelectionScenario()
	state := game.CreateInstance()
	state.Iteration = 1
	if err := state.FireEvents([]int{1, 2}); err == nil {
		t.Fatal("FireEvents accepted two events from the same branch")
	}
	if err := state.FireEvents([]int{1}); err != nil {
		t.Fatalf("FireEvents([1]): %v", err)
	}
	if state.Events[1].Status != EventFiredStatus || state.Events[3].Status != EventFiredStatus {
		t.Errorf("FireEvents did not apply events from both branches: %v", state.Events)
	}
	if attr := state.Attributes[1].FinalValue; attr != 9 || state.Budget.Money != 9 {
		t.Errorf("attribute/money = %v/%d, want 9/9", attr, state.Budget.Money)
	}
}
