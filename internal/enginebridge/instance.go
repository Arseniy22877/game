package enginebridge

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"

	e "github.com/Arseniy22877/game/gameengine"
	"github.com/Arseniy22877/game/internal/api"
)

type Instance struct {
	scenario    *Scenario
	state       e.GameState
	raw         []e.ProbResult
	predictions []api.Prediction
}

type DecisionError struct{ Code, Message string }

func (e *DecisionError) Error() string { return e.Message }
func (i *Instance) SetRound(r int)     { i.state.Iteration = r; i.raw = nil; i.predictions = nil }
func (i *Instance) Submit(d api.Decision) (api.Cost, error) {
	next := i.state
	invalid := func(msg string) (api.Cost, error) { return api.Cost{}, &DecisionError{api.CodeInvalidDecision, msg} }
	if len(d.Attributes) != len(next.GameData.ConfigurableAttributes) {
		return invalid("Укажите все настраиваемые атрибуты")
	}
	var ids []int
	var values []float64
	for _, a := range d.Attributes {
		if math.IsNaN(a.Value) || math.IsInf(a.Value, 0) {
			return invalid("Значение атрибута должно быть конечным числом")
		}
		ids = append(ids, a.ID)
		values = append(values, a.Value)
	}
	if err := next.PassAttributes(ids, values); err != nil {
		return invalid("Недопустимый выбор атрибутов: " + err.Error())
	}
	if err := next.PassStrategies(d.Strategies); err != nil {
		return invalid("Недопустимый выбор стратегий: " + err.Error())
	}
	if err := next.PassHints(d.Hints); err != nil {
		return invalid("Недопустимый выбор исследований: " + err.Error())
	}
	trial := next
	if err := trial.CalculateState(); err != nil {
		switch {
		case errors.Is(err, e.ErrInsufficientBudget):
			return api.Cost{}, &DecisionError{api.CodeInsufficientMoney, fmt.Sprintf("Не хватает денег на решение, доступно %d", next.Budget.Money)}
		case errors.Is(err, e.ErrInsufficientQP):
			return api.Cost{}, &DecisionError{api.CodeInsufficientQP, fmt.Sprintf("Не хватает QP на исследования, доступно %d", next.Budget.QP)}
		default:
			return invalid("Решение несовместимо с выбранными стратегиями: " + err.Error())
		}
	}
	i.state = next
	return api.Cost{Money: next.Budget.Money - trial.Budget.Money, QP: next.Budget.QP - trial.Budget.QP}, nil
}

func (i *Instance) defaultLevels(minimum bool) (api.Decision, bool) {
	d := api.Decision{Attributes: []api.AttributeChoice{}, Strategies: []int{}, Hints: []int{}}
	var changes []e.AttributeChange
	for _, a := range i.state.GameData.ConfigurableAttributes {
		v := i.state.Attributes[a.ID].BaseValue
		if minimum {
			v = a.MinValue
		}
		d.Attributes = append(d.Attributes, api.AttributeChoice{ID: int(a.ID), Value: v})
		changes = append(changes, e.AttributeChange{AttributeID: a.ID, AttributeShift: v})
	}
	return d, i.state.Budget.Money >= BaseRoundCost && i.state.AttributeCost(changes) <= i.state.Budget.Money-BaseRoundCost
}

func (i *Instance) Bankrupt() bool { _, ok := i.defaultLevels(true); return !ok }
func (i *Instance) DefaultDecision() api.Decision {
	d, ok := i.defaultLevels(false)
	if !ok {
		d, _ = i.defaultLevels(true)
	}
	return d
}

func (i *Instance) Calculate() ([]api.Prediction, error) {
	if err := i.state.CalculateState(); err != nil {
		return nil, err
	}
	return i.predict()
}

func (i *Instance) SkipRound() ([]api.Prediction, error) {
	s := &i.state
	s.PendingAttributes = nil
	s.PendingStrategies = nil
	s.PendingHints = nil
	if err := s.UseStrategies(nil); err != nil {
		return nil, err
	}
	return i.predict()
}

func (i *Instance) predict() ([]api.Prediction, error) {
	raw, err := i.state.GetPredicts()
	if err != nil {
		return nil, err
	}
	i.raw = raw
	groups := map[e.Branch][]e.ProbResult{}
	for _, p := range raw {
		b := p.Event.EventData.Branch
		groups[b] = append(groups[b], p)
	}
	result := []api.Prediction{}
	for _, group := range groups {
		var sum float64
		for _, p := range group {
			sum += p.Probability
		}
		normalized := e.Normalize(group)
		for _, p := range normalized {
			weight := p.Probability
			if sum < i.state.GameData.MinProbabilitySum {
				weight = 0
			}
			result = append(result, api.Prediction{Event: i.scenario.event(*p.Event.EventData), Probability: weight})
		}
	}
	sort.Slice(result, func(a, b int) bool {
		if result[a].Probability == result[b].Probability {
			return result[a].Event.ID < result[b].Event.ID
		}
		return result[a].Probability > result[b].Probability
	})
	i.predictions = result
	return slices.Clone(result), nil
}

func (i *Instance) Suggest() []api.Prediction {
	picked, _ := i.state.PickEvents(i.raw)
	ids := map[int]bool{}
	for _, p := range picked {
		ids[int(p.Event.EventData.ID)] = true
	}
	result := append([]api.Prediction{}, i.predictions...)
	for j := range result {
		result[j].Suggested = ids[result[j].Event.ID]
	}
	return result
}
