package main

import (
	"errors"
	"fmt"
	"math"
	"sort"
)

var (
	ErrDimensionsNotMatch = errors.New("Dimensions do not match")
)

type Attractor struct {
	_eventState *EventState
	center      Vector
	weights     Vector
	power       Power
}

func (e *EventState) ToAttractor() (*Attractor, error) {
	if len(e._eventData.centerPoint) != len(e.finalField.weights) {
		return nil, fmt.Errorf("Error, invalid event state with id: %d, %w", e._eventData.id, ErrDimensionsNotMatch)
	}
	return &Attractor{
		_eventState: e,
		center:      e._eventData.centerPoint,
		weights:     e.finalField.weights,
		power: Power{
			amplitude: clamp(e.finalField.power.amplitude, 0.0, 1.0),
			density:   clamp(e.finalField.power.density, 0.0, 1.0),
		},
	}, nil
}

func (g *GameState) ToVector() *Vector {
	vec := make(Vector, 0)
	vec = append(vec, float64(g.budget.money)/float64(g.budget._budgetData.initialMoney))
	attributeIDs := make([]AttributeId, 0, len(g.attributes))
	for id := range g.attributes {
		attributeIDs = append(attributeIDs, id)
	}
	sort.Slice(attributeIDs, func(i, j int) bool {
		return attributeIDs[i] < attributeIDs[j]
	})
	for _, id := range attributeIDs {
		attr := g.attributes[id]
		vec = append(vec, (attr.finalValue-attr._attributeData.minValue)/(attr._attributeData.maxValue-attr._attributeData.minValue))
	}
	return &vec
}

func (a *Attractor) getDistance(vec *Vector) (distance float64, err error) {
	if len(*vec) != len(a.center) {
		return 0.0, fmt.Errorf("Error getting distance: %w", ErrDimensionsNotMatch)
	}

	for i, x := range *vec {
		diff := (x - a.center[i])
		diff = diff * diff * a.weights[i]
		distance += diff
	}
	return distance, nil
}

type ProbResult struct {
	_attractor  *Attractor
	probability float64
}

func (a *Attractor) GetProbability(vec *Vector) (ProbResult, error) {
	if distance, err := a.getDistance(vec); err == nil {
		if distance <= 0.0 {
			return ProbResult{
				_attractor: a, probability: a.power.amplitude,
			}, nil
		}

		if a.power.density <= 0 {
			return ProbResult{
				_attractor: a, probability: 0,
			}, nil
		}

		ratio := distance / a.power.density

		return ProbResult{
			_attractor: a, probability: a.power.amplitude * math.Exp(-math.Log(100.0)*ratio*ratio),
		}, nil
	} else {
		return ProbResult{
			_attractor: a, probability: 0.0,
		}, fmt.Errorf("Error getting probability: %w", err)
	}
}

func (g *GameState) getProbabilitiesForEvents() ([]ProbResult, error) {
	state := g.ToVector()
	var err error = nil
	result := make([]ProbResult, 0)
	for _, event := range g.events {
		if event.finalStatus != EventAvailableStatus {
			continue
		}

		attractor, e := event.ToAttractor()
		if e != nil {
			err = fmt.Errorf("%w, %w", e, err)
			continue
		}

		prob, e2 := attractor.GetProbability(state)

		if e2 != nil {
			err = fmt.Errorf("%w, %w", e, err)
			continue
		}

		result = append(result, prob)
	}

	return result, err
}
