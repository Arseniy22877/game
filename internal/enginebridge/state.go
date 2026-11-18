package enginebridge

import (
	"sort"

	e "github.com/Arseniy22877/game/gameengine"
	"github.com/Arseniy22877/game/internal/api"
)

func attr(a e.GameAttribute, v float64) api.Attribute {
	return api.Attribute{ID: int(a.ID), Name: a.Name, Description: a.Description, Min: a.MinValue, Max: a.MaxValue, Value: v}
}

func (i *Instance) State() api.TeamGameState {
	s := &i.state
	dto := api.TeamGameState{Budget: api.Budget{Money: s.Budget.Money, MaxMoney: s.GameData.StartingBudget.MaxMoney, QP: s.Budget.QP}, Attributes: []api.Attribute{}, Configurable: []api.ConfigurableAttribute{}, Strategies: []api.Strategy{}, Hints: []api.Hint{}, History: []api.RoundEvents{}}
	for _, a := range s.GameData.BasicAttributes {
		dto.Attributes = append(dto.Attributes, attr(a, s.Attributes[a.ID].FinalValue))
	}
	for _, a := range s.GameData.ConfigurableAttributes {
		dto.Configurable = append(dto.Configurable, api.ConfigurableAttribute{Attribute: attr(a.GameAttribute, s.Attributes[a.ID].BaseValue), CostPerUnit: a.CostPerUnit})
	}
	available := map[e.StrategyId]bool{}
	for _, v := range s.GetAvailableStrategies() {
		available[v.StrategyData.ID] = true
	}
	for id, v := range s.Strategies {
		a := v.StrategyData
		active := v.Status == e.StrategyUsingStatus
		if active || available[id] {
			dto.Strategies = append(dto.Strategies, api.Strategy{ID: int(id), Name: a.Name, Description: a.Description, Cost: a.Cost, Active: active})
		}
	}
	hints := s.GetAvailableHints()
	for _, v := range s.Hints {
		if v.Status == e.HintUsedStatus {
			hints = append(hints, v)
		}
	}
	for _, v := range hints {
		a := v.HintData
		h := api.Hint{ID: int(a.ID), Name: a.Name, Description: a.Description, Cost: a.Cost, Reliability: a.Reliability, Used: v.Status == e.HintUsedStatus}
		if h.Used {
			h.Secret = a.Secret
		}
		dto.Hints = append(dto.Hints, h)
	}
	sort.Slice(dto.Attributes, func(a, b int) bool { return dto.Attributes[a].ID < dto.Attributes[b].ID })
	sort.Slice(dto.Configurable, func(a, b int) bool { return dto.Configurable[a].ID < dto.Configurable[b].ID })
	sort.Slice(dto.Strategies, func(a, b int) bool { return dto.Strategies[a].ID < dto.Strategies[b].ID })
	sort.Slice(dto.Hints, func(a, b int) bool { return dto.Hints[a].ID < dto.Hints[b].ID })
	return dto
}
