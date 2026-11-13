package gameengine

import "testing"

func TestCalculateStateChargesHintsInQP(t *testing.T) {
	game := Game{
		StartingBudget: Budget{InitialMoney: 100, MaxMoney: 100, InitialQP: 5},
		Hints:          []Hint{{ID: 1, Cost: 3}},
	}
	state := game.CreateInstance()
	state.Hints[1] = HintState{HintData: &game.Hints[0], Status: HintUnlockedStatus}

	if err := state.PassHints([]int{1}); err != nil {
		t.Fatal(err)
	}
	if err := state.CalculateState(); err != nil {
		t.Fatal(err)
	}
	if state.Budget.Money != 100-CycleBaseCost {
		t.Fatalf("money = %d, want %d", state.Budget.Money, 100-CycleBaseCost)
	}
	if state.Budget.QP != 2 {
		t.Fatalf("QP = %d, want 2", state.Budget.QP)
	}
}

func TestCalculateStateRejectsInsufficientQP(t *testing.T) {
	game := Game{
		StartingBudget: Budget{InitialMoney: 100, MaxMoney: 100, InitialQP: 2},
		Hints:          []Hint{{ID: 1, Cost: 3}},
	}
	state := game.CreateInstance()
	state.Hints[1] = HintState{HintData: &game.Hints[0], Status: HintUnlockedStatus}

	if err := state.PassHints([]int{1}); err != nil {
		t.Fatal(err)
	}
	if err := state.CalculateState(); err != ErrInsufficientQP {
		t.Fatalf("error = %v, want %v", err, ErrInsufficientQP)
	}
	if state.Budget.Money != 100 || state.Budget.QP != 2 {
		t.Fatalf("budget changed after rejected cycle: money=%d QP=%d", state.Budget.Money, state.Budget.QP)
	}
}
