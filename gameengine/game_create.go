package gameengine

func (game *Game) CreateInstance() GameState {
	state := GameState{
		ID:         "",
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
			Status:        AttributeNeutral,
		}
	}
	for i := range game.ConfigurableAttributes {
		attribute := &game.ConfigurableAttributes[i].GameAttribute
		state.Attributes[attribute.ID] = AttributeState{
			AttributeData: attribute,
			BaseValue:     attribute.DefaultValue,
			FinalValue:    attribute.DefaultValue,
			Status:        AttributeNeutral,
		}
	}

	StartingEvents := make(map[EventId]bool, len(game.StartingEvents))
	for _, ID := range game.StartingEvents {
		StartingEvents[ID] = true
	}
	for i := range game.Events {
		event := &game.Events[i]
		Status := EventLockedStatus
		if StartingEvents[event.ID] {
			Status = EventAvailableStatus
		}
		if event.ID == game.OpeningEvent {
			Status = EventFiredStatus
		}
		BaseField := Field{
			Weights: copyVector(event.BasicParameters.Weights),
			Power:   event.BasicParameters.Power,
		}
		state.Events[event.ID] = EventState{
			EventData: event,
			BaseField: BaseField,
			FinalField: Field{
				Weights: copyVector(BaseField.Weights),
				Power:   BaseField.Power,
			},
			BaseStatus:  Status,
			FinalStatus: Status,
		}
	}

	for i := range game.Hints {
		hint := &game.Hints[i]
		state.Hints[hint.ID] = HintState{
			HintData: hint,
			Status:   HintLockedStatus,
		}
	}
	for i := range game.Strategies {
		strategy := &game.Strategies[i]
		state.Strategies[strategy.ID] = StrategyState{
			StrategyData: strategy,
			Status:       StrategyLockedStatus,
		}
	}

	return state
}
