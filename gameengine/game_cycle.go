package gameengine

// State cloning helpers used by transactional API operations.

func cloneAttributeStates(source map[AttributeId]AttributeState) map[AttributeId]AttributeState {
	result := make(map[AttributeId]AttributeState, len(source))
	for id, value := range source {
		result[id] = value
	}
	return result
}

func cloneStrategyStates(source map[StrategyId]StrategyState) map[StrategyId]StrategyState {
	result := make(map[StrategyId]StrategyState, len(source))
	for id, value := range source {
		result[id] = value
	}
	return result
}

func cloneHintStates(source map[HintId]HintState) map[HintId]HintState {
	result := make(map[HintId]HintState, len(source))
	for id, value := range source {
		result[id] = value
	}
	return result
}
