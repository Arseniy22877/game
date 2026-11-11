package main

import (
	"errors"
	"math"
	"math/rand/v2"
)

func getSum(probs []ProbResult) float64 {
	sum := 0.0
	for _, prob := range probs {
		sum += prob.probability
	}
	return sum
}

func normalize(probs []ProbResult) []ProbResult {
	sum := getSum(probs)
	res := make([]ProbResult, 0, len(probs))
	if sum <= 0 {
		return res
	}

	for _, prob := range probs {
		res = append(res, ProbResult{
			_attractor:  prob._attractor,
			probability: prob.probability / sum,
		})
	}

	return res
}

func split(probs []ProbResult) ([]ProbResult, []ProbResult) { // returns positive, negative probs
	positive := make([]ProbResult, 0)
	negative := make([]ProbResult, 0)
	for _, prob := range probs {
		if prob.probability <= 0 {
			continue
		}
		if prob._attractor._eventState._eventData.isPositive {
			positive = append(positive, prob)
		} else {
			negative = append(negative, prob)
		}
	}

	return positive, negative
}

func pick(probs *[]ProbResult) (res ProbResult) {
	score := rand.Float64()
	acc := 0.0
	for i, prob := range *probs {
		acc += prob.probability
		if acc >= score {
			res = prob
			*probs = append((*probs)[:i], (*probs)[i+1:]...)
			break
		}
	}
	return
}

func (g *GameState) pickEvents(probs []ProbResult) (res []ProbResult, err error) {
	pos, neg := split(probs)
	ePos := getSum(pos)
	eNeg := getSum(neg)
	_eNeg := eNeg
	if int(math.Round(ePos))+int(math.Round(eNeg)) > g._gameData.maxEventsPerTurn {
		proportion := float64(g._gameData.maxEventsPerTurn) / (ePos + eNeg)
		ePos *= proportion
		eNeg *= proportion
	}

	cNeg := int(math.Round(eNeg))
	cPos := int(math.Round(ePos))

	if len(neg) >= 1 && cNeg == 0 && _eNeg >= g._gameData.softness {
		cNeg = 1
	}

	if cNeg+cPos == 0 {
		if len(neg) > 0 {
			cNeg++
		} else if len(pos) > 0 {
			cPos++
		} else {
			err = errors.New("No probable events at all")
		}
	}

	res = make([]ProbResult, 0, cNeg+cPos)

	neg = normalize(neg)
	pos = normalize(pos)
	if cNeg > len(neg) {
		cNeg = len(neg)
	}
	if cPos > len(pos) {
		cPos = len(pos)
	}

	for cNeg > 0 {
		cNeg--
		res = append(res, pick(&neg))
	}

	for cPos > 0 {
		cPos--
		res = append(res, pick(&pos))
	}

	return res, err
}
