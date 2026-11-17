package gameengine

import (
	"errors"
	"math/rand/v2"
)

func normalize(probs []ProbResult) []ProbResult {
	var total float64
	for _, probability := range probs {
		total += probability.Probability
	}
	if total <= 0 {
		return nil
	}
	result := make([]ProbResult, len(probs))
	for i, probability := range probs {
		result[i] = probability
		result[i].Probability /= total
	}
	return result
}

func Normalize(probs []ProbResult) []ProbResult {
	return normalize(probs)
}

func pick(probs []ProbResult) ProbResult {
	score := rand.Float64()
	var accumulated float64
	for _, probability := range probs {
		accumulated += probability.Probability
		if score <= accumulated {
			return probability
		}
	}
	return probs[len(probs)-1]
}

// PickEvents chooses at most one event from every branch.
func (g *GameState) PickEvents(probs []ProbResult) ([]ProbResult, error) {
	result := make([]ProbResult, 0, len(g.GameData.Branches))
	for _, branch := range g.GameData.Branches {
		prob, err := g.PickEventFromBranch(probs, branch)
		if err == nil {
			result = append(result, prob)
		}
	}
	if len(result) == 0 {
		return []ProbResult{}, errors.New("no probable events")
	}
	return result, nil
}

// PickEvents chooses at most one event from every branch.
func (g *GameState) PickEventFromBranch(probs []ProbResult, branch Branch) (ProbResult, error) {
	temp := make([]ProbResult, 0, len(probs))
	for _, probability := range probs {
		if probability.Event == nil || probability.Probability <= 0 {
			continue
		}
		if probability.Event.EventData.Branch == branch {
			temp = append(temp, probability)
		}
	}
	if len(temp) == 0 {
		return ProbResult{}, errors.New("no probable events for branch")
	}
	return pick(normalize(temp)), nil
}
