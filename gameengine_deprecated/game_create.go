package gameengine

func (game *Game) CreateInstance() GameState {
	state := GameState{
		GameData:   game,
		Attributes: make(map[AttributeId]AttributeState),
		Events:     make(map[EventId]EventState),
		Hints:      make(map[HintId]HintState),
		Strategies: make(map[StrategyId]StrategyState),
		Budget: BudgetState{
			BudgetData: &game.StartingBudget,
			Money:      game.StartingBudget.InitialMoney,
			QP:         game.StartingBudget.InitialQP,
		},
	}

	for i := range game.BasicAttributes {
		attribute := &game.BasicAttributes[i]
		state.Attributes[attribute.ID] = AttributeState{
			AttributeData: attribute,
			BaseValue:     attribute.DefaultValue,
			FinalValue:    attribute.DefaultValue,
		}
	}
	for i := range game.ConfigurableAttributes {
		attribute := &game.ConfigurableAttributes[i].GameAttribute
		state.Attributes[attribute.ID] = AttributeState{
			AttributeData: attribute,
			BaseValue:     attribute.DefaultValue,
			FinalValue:    attribute.DefaultValue,
		}
	}
	for i := range game.Events {
		event := &game.Events[i]
		state.Events[event.ID] = EventState{EventData: event}
	}
	for i := range game.Hints {
		hint := &game.Hints[i]
		state.Hints[hint.ID] = HintState{HintData: hint}
	}
	for i := range game.Strategies {
		strategy := &game.Strategies[i]
		state.Strategies[strategy.ID] = StrategyState{StrategyData: strategy}
	}
	if event, ok := state.Events[game.OpeningEvent]; ok {
		event.Status = EventFiredStatus
		state.Events[game.OpeningEvent] = event
	}
	return state
}
