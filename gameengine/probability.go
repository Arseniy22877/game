package gameengine

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
)

var ErrDimensionsNotMatch = errors.New("dimensions do not match")

type ProbResult struct {
	Event       *EventState
	Probability float64
}

// attributeIDsInScenarioOrder returns the stable order used by event Weights.
// Map iteration order is deliberately never used for vector coordinates.
func (g *GameState) attributeIDsInScenarioOrder() ([]AttributeId, error) {
	if g == nil || g.GameData == nil {
		return nil, errors.New("game data is nil")
	}

	ids := make([]AttributeId, 0, len(g.Attributes))
	seen := make(map[AttributeId]struct{}, len(g.Attributes))
	add := func(id AttributeId) error {
		if _, duplicate := seen[id]; duplicate {
			return fmt.Errorf("attribute %d is defined more than once in game data", id)
		}
		if _, exists := g.Attributes[id]; !exists {
			return fmt.Errorf("attribute %d is missing from game state", id)
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		return nil
	}

	for _, attribute := range g.GameData.BasicAttributes {
		if err := add(attribute.ID); err != nil {
			return nil, err
		}
	}
	for _, attribute := range g.GameData.ConfigurableAttributes {
		if err := add(attribute.ID); err != nil {
			return nil, err
		}
	}

	if len(seen) != len(g.Attributes) {
		unknown := make([]int, 0)
		for id := range g.Attributes {
			if _, exists := seen[id]; !exists {
				unknown = append(unknown, int(id))
			}
		}
		sort.Ints(unknown)
		return nil, fmt.Errorf("%w: attributes %v are not declared in game data", ErrDimensionsNotMatch, unknown)
	}
	return ids, nil
}

// GetProbability calculates the event's raw selection weight:
// BasicPower + sum(Weights[i] * attribute[i]).
// The returned value is a weight, not a normalized probability.
func (e *EventState) GetProbability(g *GameState) (ProbResult, error) {
	if e == nil || e.EventData == nil {
		return ProbResult{}, errors.New("nil event state")
	}
	if g == nil {
		return ProbResult{}, errors.New("nil game state")
	}
	if !g.eventCanOccur(e.EventData) {
		return ProbResult{Event: e}, nil
	}

	attributeIDs, err := g.attributeIDsInScenarioOrder()
	if err != nil {
		return ProbResult{}, err
	}
	if len(e.EventData.Weights) != len(attributeIDs) {
		return ProbResult{}, fmt.Errorf(
			"%w: event %d has %d weights, expected %d (basic attributes followed by configurable attributes)",
			ErrDimensionsNotMatch, e.EventData.ID, len(e.EventData.Weights), len(attributeIDs),
		)
	}

	probability := e.EventData.BasicPower
	for i, weight := range e.EventData.Weights {
		attribute := g.Attributes[attributeIDs[i]]
		probability += weight * attribute.FinalValue
	}
	if math.IsNaN(probability) || math.IsInf(probability, 0) {
		return ProbResult{}, fmt.Errorf("invalid weight for event %d", e.EventData.ID)
	}

	return ProbResult{Event: e, Probability: math.Max(0, probability)}, nil
}

// GetProbabilitiesForEvents returns raw weights for all currently available,
// neutral events. Iteration 0 events are included in every iteration.
func (g *GameState) GetProbabilitiesForEvents() ([]ProbResult, error) {
	if g == nil {
		return nil, errors.New("game state is nil")
	}
	result := make([]ProbResult, 0, len(g.Events))
	var combinedErr error
	for id, event := range g.Events {
		if event.Status != EventNeutral || !g.eventCanOccur(event.EventData) {
			continue
		}
		probability, err := event.GetProbability(g)
		if err != nil {
			combinedErr = errors.Join(combinedErr, fmt.Errorf("event %d: %w", id, err))
			continue
		}
		if probability.Probability > 0 {
			result = append(result, probability)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Event.EventData.ID < result[j].Event.EventData.ID
	})
	return result, combinedErr
}

// normalize converts raw weights to shares without overflowing when their sum
// is very large.
func normalize(probs []ProbResult) []ProbResult {
	if len(probs) == 0 {
		return nil
	}

	maxWeight := 0.0
	for _, probability := range probs {
		if probability.Probability > maxWeight {
			maxWeight = probability.Probability
		}
	}
	if maxWeight <= 0 || math.IsNaN(maxWeight) || math.IsInf(maxWeight, 0) {
		return nil
	}

	var scaledTotal float64
	for _, probability := range probs {
		scaledTotal += probability.Probability / maxWeight
	}
	if scaledTotal <= 0 || math.IsNaN(scaledTotal) || math.IsInf(scaledTotal, 0) {
		return nil
	}

	result := make([]ProbResult, len(probs))
	for i, probability := range probs {
		result[i] = probability
		result[i].Probability = (probability.Probability / maxWeight) / scaledTotal
	}
	return result
}

func Normalize(probs []ProbResult) []ProbResult {
	return normalize(probs)
}

func pick(probs []ProbResult) (ProbResult, bool) {
	if len(probs) == 0 {
		return ProbResult{}, false
	}

	score := rand.Float64()
	var accumulated float64
	for _, probability := range probs {
		accumulated += probability.Probability
		if score < accumulated {
			return probability, true
		}
	}
	// Protect against tiny floating-point rounding differences.
	return probs[len(probs)-1], true
}

// PickEvents chooses at most one event from each configured branch.
func (g *GameState) PickEvents(probs []ProbResult) ([]ProbResult, error) {
	if g == nil || g.GameData == nil {
		return nil, errors.New("game data is nil")
	}
	result := make([]ProbResult, 0, len(g.GameData.Branches))
	for _, branch := range g.GameData.Branches {
		probability, err := g.PickEventFromBranch(probs, branch)
		if err == nil {
			result = append(result, probability)
		}
	}
	if len(result) == 0 {
		return []ProbResult{}, errors.New("no probable events")
	}
	return result, nil
}

// PickEventFromBranch selects one event in the given branch, proportionally
// to that branch's raw event weights.
func (g *GameState) PickEventFromBranch(probs []ProbResult, branch Branch) (ProbResult, error) {
	temp := make([]ProbResult, 0, len(probs))
	for _, probability := range probs {
		if probability.Event == nil || probability.Event.EventData == nil || probability.Probability <= 0 {
			continue
		}
		if probability.Event.EventData.Branch == branch {
			temp = append(temp, probability)
		}
	}
	sum := 0.0
	for _, el := range temp {
		sum += max(0, el.Probability)
	}
	if sum < g.GameData.MinProbabilitySum {
		return ProbResult{}, errors.New("No events for branch")
	}
	normalized := normalize(temp)
	picked, ok := pick(normalized)
	if !ok {
		return ProbResult{}, errors.New("No events for branch")
	}
	return picked, nil
}
