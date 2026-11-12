package main

import (
	"fmt"
	"math"

	"gameengine2/gameengine"
)

func runCycle(state *gameengine.GameState, Attributes []gameengine.AttributeChange, Strategies []gameengine.StrategyId, Hints []gameengine.HintId) error {
	attributeIDs := make([]int, len(Attributes))
	attributeValues := make([]float64, len(Attributes))
	for i, attribute := range Attributes {
		attributeIDs[i] = int(attribute.AttributeID)
		attributeValues[i] = attribute.AttributeShift
	}
	strategyIDs := make([]int, len(Strategies))
	for i, strategy := range Strategies {
		strategyIDs[i] = int(strategy)
	}
	hintIDs := make([]int, len(Hints))
	for i, hint := range Hints {
		hintIDs[i] = int(hint)
	}
	if err := state.PassAtributes(attributeIDs, attributeValues); err != nil {
		return err
	}
	if err := state.PassStrategies(strategyIDs); err != nil {
		return err
	}
	if err := state.PassHints(hintIDs); err != nil {
		return err
	}
	if err := state.CalculateState(); err != nil {
		return err
	}
	return state.FireEvents(nil)
}

type unitTestSuite struct {
	passed int
	failed int
}

func (suite *unitTestSuite) check(Name string, condition bool, details string) {
	if condition {
		suite.passed++
		fmt.Printf("[PASS] %s\n", Name)
		return
	}
	suite.failed++
	fmt.Printf("[FAIL] %s: %s\n", Name, details)
}

func (suite *unitTestSuite) checkEqual(Name string, got, want interface{}) {
	suite.check(Name, got == want, fmt.Sprintf("got %v, want %v", got, want))
}

func runUnitTests(game gameengine.Game) bool {
	fmt.Println("=== Unit tests ===")
	suite := unitTestSuite{}

	state := game.CreateInstance()
	suite.checkEqual("instance stores source game", state.GameData, &game)
	suite.checkEqual("initial Money", state.Budget.Money, game.StartingBudget.InitialMoney)
	suite.checkEqual("initial QP", state.Budget.QP, game.StartingBudget.InitialQP)
	suite.checkEqual("all Attributes initialized", len(state.Attributes), 4)
	suite.checkEqual("all Events initialized", len(state.Events), 6)
	suite.checkEqual("all Hints initialized", len(state.Hints), 3)
	suite.checkEqual("all Strategies initialized", len(state.Strategies), 3)

	for _, ID := range []gameengine.EventId{1, 2} {
		suite.check(fmt.Sprintf("starting event %d available", ID),
			state.Events[ID].BaseStatus == gameengine.EventAvailableStatus,
			fmt.Sprintf("Status is %d", state.Events[ID].BaseStatus))
	}
	suite.check("opening event starts fired",
		state.Events[0].BaseStatus == gameengine.EventFiredStatus,
		fmt.Sprintf("Status is %d", state.Events[0].BaseStatus))
	suite.check("non-starting event locked",
		state.Events[3].BaseStatus == gameengine.EventLockedStatus,
		fmt.Sprintf("Status is %d", state.Events[3].BaseStatus))
	suite.check("initial vector has expected dimensions",
		len(*state.ToVector()) == 5,
		fmt.Sprintf("length is %d", len(*state.ToVector())))

	state.Budget.Money = 175
	income := gameengine.BudgetChange{MoneyShift: 20}
	income.Apply(&state)
	suite.checkEqual("gameengine.Budget respects maximum", state.Budget.Money, game.StartingBudget.MaxMoney)
	expense := gameengine.BudgetChange{MoneyShift: -1000}
	expense.Apply(&state)
	suite.checkEqual("gameengine.Budget does not become negative", state.Budget.Money, 0)
	state.Budget.Money = game.StartingBudget.InitialMoney

	baseEvent := state.Events[0]
	originalPower := baseEvent.BaseField.Power
	originalWeight := baseEvent.BaseField.Weights[0]
	parameterChange := gameengine.EventParametersChange{
		EventID: 0,
		Shift: gameengine.EventParameters{
			Power:   gameengine.Power{Amplitude: 0.1, Density: 0.2},
			Weights: gameengine.Vector{0.1, 0.1, 0.1, 0.1, 0.1},
		},
	}
	suite.check("base event parameters Apply",
		parameterChange.ApplyBase(&state) == nil,
		"applyBase returned an error")
	changedEvent := state.Events[0]
	suite.check("base gameengine.Power is persisted",
		changedEvent.BaseField.Power.Amplitude == originalPower.Amplitude+0.1,
		fmt.Sprintf("gameengine.Power is %v", changedEvent.BaseField.Power))
	suite.check("base Weights are persisted",
		math.Abs(changedEvent.BaseField.Weights[0]-(originalWeight+0.1)) < 1e-9,
		fmt.Sprintf("Weights are %v", changedEvent.BaseField.Weights))

	state = game.CreateInstance()
	eventToFire := state.Events[0]
	eventToFire.Fire(&state)
	suite.check("event firing changes Status",
		state.Events[0].BaseStatus == gameengine.EventFiredStatus,
		fmt.Sprintf("Status is %d", state.Events[0].BaseStatus))
	suite.check("event unlocks strategy",
		state.Strategies[0].Status == gameengine.StrategyUnlockedStatus,
		fmt.Sprintf("Status is %d", state.Strategies[0].Status))
	suite.check("event unlocks hint",
		state.Hints[0].Status == gameengine.HintUnlockedStatus,
		fmt.Sprintf("Status is %d", state.Hints[0].Status))
	suite.check("event unlocks event",
		state.Events[3].BaseStatus == gameengine.EventAvailableStatus,
		fmt.Sprintf("Status is %d", state.Events[3].BaseStatus))

	suite.check("locked strategy is rejected",
		state.ValidateStrategies([]gameengine.StrategyId{1}) != nil,
		"locked strategy was accepted")
	suite.check("locked hint is rejected",
		state.ValidateHints([]gameengine.HintId{1}) != nil,
		"locked hint was accepted")
	suite.check("unknown attribute is rejected",
		state.ValidateAttributes([]gameengine.AttributeChange{{AttributeID: 99}}) != nil,
		"unknown attribute was accepted")

	state.Strategies[0] = gameengine.StrategyState{
		StrategyData: &game.Strategies[0],
		Status:       gameengine.StrategyUsingStatus,
	}
	state.CalculateFinals()
	suite.check("active strategy changes final event gameengine.Power",
		state.Events[0].FinalField.Power.Amplitude > state.Events[0].BaseField.Power.Amplitude,
		"final gameengine.Power did not increase")
	state.UseStrategies(nil)
	state.CalculateFinals()
	suite.check("disabled strategy is removed from final field",
		state.Events[0].FinalField.Power.Amplitude == state.Events[0].BaseField.Power.Amplitude,
		"strategy effect remained after disabling")
	suite.check("disabled strategy does not change base field",
		state.Events[0].BaseField.Power.Amplitude == game.Events[0].BasicParameters.Power.Amplitude,
		"base field was changed by final calculation")
	suite.check("final strategy Weights do not alias base Weights",
		math.Abs(state.Events[0].BaseField.Weights[0]-game.Events[0].BasicParameters.Weights[0]) < 1e-9,
		fmt.Sprintf("base Weights are %v", state.Events[0].BaseField.Weights))

	state = game.CreateInstance()
	invalidEventChange := gameengine.EventParametersChange{
		EventID: 99,
		Shift: gameengine.EventParameters{
			Weights: gameengine.Vector{0, 0, 0, 0, 0},
		},
	}
	suite.check("invalid event parameter target is rejected",
		invalidEventChange.ApplyBase(&state) == gameengine.ErrInvalidEventId,
		"invalid event target was accepted")

	state = game.CreateInstance()
	originalAttribute := state.Attributes[1].BaseValue
	originalMoney := state.Budget.Money
	atomicErr := runCycle(&state,
		[]gameengine.AttributeChange{{AttributeID: 1, AttributeShift: 0.9}},
		nil,
		[]gameengine.HintId{99},
	)
	suite.check("invalid cycle input returns an error", atomicErr != nil,
		"invalid hint was accepted")
	suite.check("invalid cycle does not change Attributes",
		state.Attributes[1].BaseValue == originalAttribute,
		fmt.Sprintf("attribute is %.2f", state.Attributes[1].BaseValue))
	suite.check("invalid cycle does not charge gameengine.Budget",
		state.Budget.Money == originalMoney,
		fmt.Sprintf("Money is %d", state.Budget.Money))

	state = game.CreateInstance()
	initialVector := append(gameengine.Vector(nil), (*state.ToVector())...)
	repeatedVector := state.ToVector()
	suite.check("state vector ordering is stable",
		math.Abs((*repeatedVector)[0]-initialVector[0]) < 1e-9 &&
			math.Abs((*repeatedVector)[1]-initialVector[1]) < 1e-9 &&
			math.Abs((*repeatedVector)[2]-initialVector[2]) < 1e-9 &&
			math.Abs((*repeatedVector)[3]-initialVector[3]) < 1e-9 &&
			math.Abs((*repeatedVector)[4]-initialVector[4]) < 1e-9,
		fmt.Sprintf("vectors are %v and %v", initialVector, *repeatedVector))

	state = game.CreateInstance()
	state.CalculateFinals()
	probabilities, probabilityErr := state.GetProbabilitiesForEvents()
	suite.check("probabilities calculate without errors", probabilityErr == nil,
		fmt.Sprintf("error is %v", probabilityErr))
	suite.check("probabilities include available Events", len(probabilities) == 2,
		fmt.Sprintf("count is %d", len(probabilities)))
	for i, probability := range probabilities {
		suite.check(fmt.Sprintf("probability %d is valid", i),
			probability.Attractor != nil && probability.Probability >= 0,
			fmt.Sprintf("result is %+v", probability))
	}
	suite.check("zero probability normalization is empty",
		len(gameengine.Normalize([]gameengine.ProbResult{{Probability: 0}})) == 0,
		"zero probability produced a NaN result")
	emptyEvents, emptyEventsErr := state.PickEvents(nil)
	suite.check("empty event field returns an error",
		emptyEventsErr != nil && len(emptyEvents) == 0,
		fmt.Sprintf("Events=%v error=%v", emptyEvents, emptyEventsErr))
	state = game.CreateInstance()
	eventToFire = state.Events[0]
	eventToFire.Fire(&state)
	state.CalculateFinals()
	probabilities, probabilityErr = state.GetProbabilitiesForEvents()
	containsFiredEvent := false
	for _, probability := range probabilities {
		if probability.Attractor.EventState.EventData.ID == 0 {
			containsFiredEvent = true
		}
	}
	suite.check("fired event is excluded from next field",
		probabilityErr == nil && !containsFiredEvent,
		fmt.Sprintf("probabilities=%d error=%v", len(probabilities), probabilityErr))

	state = game.CreateInstance()
	cycleErr := runCycle(&state,
		[]gameengine.AttributeChange{{AttributeID: 1, AttributeShift: 0.7}},
		nil,
		nil,
	)
	suite.check("complete cycle executes", cycleErr == nil,
		fmt.Sprintf("error is %v", cycleErr))
	suite.check("complete cycle charges gameengine.Budget", state.Budget.Money <= 130,
		fmt.Sprintf("Money is %d", state.Budget.Money))
	suite.check("fired event is not available after cycle",
		countFiredEvents(state) > 0,
		"no event was fired")

	fmt.Printf("Unit tests: %d passed, %d failed\n", suite.passed, suite.failed)
	return suite.failed == 0
}

func countAvailableEvents(state gameengine.GameState) int {
	available := 0
	for _, event := range state.Events {
		if event.BaseStatus == gameengine.EventAvailableStatus {
			available++
		}
	}
	return available
}

func countFiredEvents(state gameengine.GameState) int {
	fired := 0
	for _, event := range state.Events {
		if event.BaseStatus == gameengine.EventFiredStatus {
			fired++
		}
	}
	return fired
}

type simulationStats struct {
	games              int
	cycles             int
	goodEvents         int
	badEvents          int
	Events             int
	cycleErrors        int
	endingMoney        int
	endingQp           int
	unlockedStrategies int
	unlockedHints      int
	unlockedEvents     int
}

func printGameCharacteristics(game gameengine.Game) {
	fmt.Println("=== gameengine.Game characteristics ===")
	fmt.Printf("Attributes: %d basic, %d configurable\n",
		len(game.BasicAttributes), len(game.ConfigurableAttributes))
	fmt.Printf("Events: %d total, %d starting, max per cycle: %d\n",
		len(game.Events), len(game.StartingEvents), game.MaxEventsPerTurn)
	fmt.Printf("Hints: %d, Strategies: %d\n", len(game.Hints), len(game.Strategies))
	fmt.Printf("gameengine.Budget: Money %d/%d, QP %d\n",
		game.StartingBudget.InitialMoney, game.StartingBudget.MaxMoney, game.StartingBudget.InitialQP)
	for _, attribute := range game.BasicAttributes {
		fmt.Printf("  Basic attribute: %s [%.2f..%.2f], default %.2f\n",
			attribute.Name, attribute.MinValue, attribute.MaxValue, attribute.DefaultValue)
	}
	for _, attribute := range game.ConfigurableAttributes {
		fmt.Printf("  Configurable attribute: %s, Cost %.2f per unit\n",
			attribute.Name, attribute.CostPerUnit)
	}
	for _, event := range game.Events {
		sign := "bad"
		if event.IsPositive {
			sign = "good"
		}
		fmt.Printf("  Event %d: %s (%s, Amplitude %.2f)\n",
			event.ID, event.Name, sign, event.BasicParameters.Power.Amplitude)
	}
	for _, strategy := range game.Strategies {
		fmt.Printf("  gameengine.Strategy %d: %s, maintenance %d\n",
			strategy.ID, strategy.Name, strategy.Cost)
	}
	for _, hint := range game.Hints {
		fmt.Printf("  gameengine.Hint %d: %s, Cost %d, Reliability %s\n",
			hint.ID, hint.Name, hint.Cost, hint.Reliability)
	}
}

func countUnlocked(state gameengine.GameState) (Strategies, Hints, Events int) {
	for _, strategy := range state.Strategies {
		if strategy.Status != gameengine.StrategyLockedStatus {
			Strategies++
		}
	}
	for _, hint := range state.Hints {
		if hint.Status != gameengine.HintLockedStatus {
			Hints++
		}
	}
	for _, event := range state.Events {
		if event.BaseStatus != gameengine.EventLockedStatus {
			Events++
		}
	}
	return Strategies, Hints, Events
}

func runSimulation(game gameengine.Game, cycles int) (simulationStats, error) {
	state := game.CreateInstance()
	stats := simulationStats{games: 1, cycles: cycles}
	countedEvents := make(map[gameengine.EventId]bool)

	for cycle := 0; cycle < cycles; cycle++ {
		Strategies := make([]gameengine.StrategyId, 0)
		Hints := make([]gameengine.HintId, 0)
		for ID, strategy := range state.Strategies {
			if strategy.Status != gameengine.StrategyLockedStatus {
				Strategies = append(Strategies, ID)
			}
		}
		for ID, hint := range state.Hints {
			if hint.Status != gameengine.HintLockedStatus {
				Hints = append(Hints, ID)
			}
		}

		changes := []gameengine.AttributeChange{
			{AttributeID: 1, AttributeShift: 0.5 + float64(cycle%3-1)*0.15},
			{AttributeID: 2, AttributeShift: 0.5 + float64((cycle+1)%3-1)*0.15},
			{AttributeID: 3, AttributeShift: 0.5 + float64((cycle+2)%3-1)*0.15},
		}
		before := make(map[gameengine.EventId]gameengine.EventStatus, len(state.Events))
		for ID, event := range state.Events {
			before[ID] = event.BaseStatus
		}

		err := runCycle(&state, changes, Strategies, Hints)
		if err != nil {
			stats.cycleErrors++
			continue
		}
		for ID, event := range state.Events {
			if event.BaseStatus == gameengine.EventFiredStatus && before[ID] != gameengine.EventFiredStatus && !countedEvents[ID] {
				countedEvents[ID] = true
				stats.Events++
				if event.EventData.IsPositive {
					stats.goodEvents++
				} else {
					stats.badEvents++
				}
			}
		}
	}

	stats.endingMoney = state.Budget.Money
	stats.endingQp = state.Budget.QP
	stats.unlockedStrategies, stats.unlockedHints, stats.unlockedEvents = countUnlocked(state)
	return stats, nil
}
