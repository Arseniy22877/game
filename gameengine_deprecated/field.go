package gameengine

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

var ErrDimensionsNotMatch = errors.New("dimensions do not match")

type ProbResult struct {
	Event       *EventState
	Probability float64
}

// GetProbability calculates BasicPower plus the weighted attribute values.
func (e *EventState) GetProbability(g *GameState) (ProbResult, error) {
	if e == nil || e.EventData == nil {
		return ProbResult{}, fmt.Errorf("nil event state")
	}
	if !g.eventCanOccur(e.EventData) {
		return ProbResult{Event: e}, nil
	}
	probability := e.EventData.BasicPower
	attributeIDs := make([]AttributeId, 0, len(g.Attributes))
	for attributeID := range g.Attributes {
		attributeIDs = append(attributeIDs, attributeID)
	}
	sort.Slice(attributeIDs, func(i, j int) bool { return attributeIDs[i] < attributeIDs[j] })
	for i, weight := range e.EventData.Weights {
		if i >= len(attributeIDs) {
			return ProbResult{}, fmt.Errorf("%w: weight index %d", ErrDimensionsNotMatch, i)
		}
		attribute := g.Attributes[attributeIDs[i]]
		probability += weight * attribute.FinalValue
	}
	if math.IsNaN(probability) || math.IsInf(probability, 0) {
		return ProbResult{}, fmt.Errorf("invalid probability for event %d", e.EventData.ID)
	}
	return ProbResult{Event: e, Probability: max(0, probability)}, nil
}

func (g *GameState) GetProbabilitiesForEvents() ([]ProbResult, error) {
	result := make([]ProbResult, 0, len(g.Events))
	var err error
	for _, event := range g.Events {
		if event.Status != EventNeutral || event.EventData.Iteration != g.Iteration {
			continue
		}
		probability, eventErr := event.GetProbability(g)
		if eventErr != nil {
			err = errors.Join(err, eventErr)
			continue
		}
		if probability.Probability > 0 {
			result = append(result, probability)
		}
	}
	return result, err
}
