package gameengine

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
	EventState *EventState
	center     Vector
	Weights    Vector
	Power      Power
}

// Преобразует состояние события в аттрактор пространства вероятностей.
func (e *EventState) ToAttractor() (*Attractor, error) {
	if len(e.EventData.CenterPoint) != len(e.FinalField.Weights) {
		return nil, fmt.Errorf("Error, invalid event state with ID: %d, %w", e.EventData.ID, ErrDimensionsNotMatch)
	}
	return &Attractor{
		EventState: e,
		center:     e.EventData.CenterPoint,
		Weights:    e.FinalField.Weights,
		Power: Power{
			Amplitude: clamp(e.FinalField.Power.Amplitude, 0.0, 1.0),
			Density:   clamp(e.FinalField.Power.Density, 0.0, 1.0),
		},
	}, nil
}

// Представляет состояние игры в виде вектора для расчёта расстояний.
func (g *GameState) ToVector() *Vector {
	vec := make(Vector, 0)
	vec = append(vec, float64(g.Budget.Money)/float64(g.Budget.BudgetData.InitialMoney))
	attributeIDs := make([]AttributeId, 0, len(g.Attributes))
	for ID := range g.Attributes {
		attributeIDs = append(attributeIDs, ID)
	}
	sort.Slice(attributeIDs, func(i, j int) bool {
		return attributeIDs[i] < attributeIDs[j]
	})
	for _, ID := range attributeIDs {
		attr := g.Attributes[ID]
		vec = append(vec, (attr.FinalValue-attr.AttributeData.MinValue)/(attr.AttributeData.MaxValue-attr.AttributeData.MinValue))
	}
	return &vec
}

// Вычисляет взвешенное расстояние от состояния до центра аттрактора.
func (a *Attractor) getDistance(vec *Vector) (distance float64, err error) {
	if len(*vec) != len(a.center) {
		return 0.0, fmt.Errorf("Error getting distance: %w", ErrDimensionsNotMatch)
	}

	for i, x := range *vec {
		diff := (x - a.center[i])
		diff = diff * diff * a.Weights[i]
		distance += diff
	}
	return distance, nil
}

type ProbResult struct {
	Attractor   *Attractor
	Probability float64
}

// Рассчитывает вероятность аттрактора для заданного состояния.
func (a *Attractor) GetProbability(vec *Vector) (ProbResult, error) {
	if distance, err := a.getDistance(vec); err == nil {
		if distance <= 0.0 {
			return ProbResult{
				Attractor: a, Probability: a.Power.Amplitude,
			}, nil
		}

		if a.Power.Density <= 0 {
			return ProbResult{
				Attractor: a, Probability: 0,
			}, nil
		}

		ratio := distance / a.Power.Density

		return ProbResult{
			Attractor: a, Probability: a.Power.Amplitude * math.Exp(-math.Log(100.0)*ratio*ratio),
		}, nil
	} else {
		return ProbResult{
			Attractor: a, Probability: 0.0,
		}, fmt.Errorf("Error getting probability: %w", err)
	}
}

// Возвращает вероятности всех доступных событий текущего поля.
func (g *GameState) GetProbabilitiesForEvents() ([]ProbResult, error) {
	state := g.ToVector()
	var err error = nil
	result := make([]ProbResult, 0)
	for _, event := range g.Events {
		if event.FinalStatus != EventAvailableStatus {
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
