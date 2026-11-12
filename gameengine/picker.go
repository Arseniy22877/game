package gameengine

import (
	"errors"
	"math"
	"math/rand/v2"
)

// Возвращает сумму вероятностей набора результатов.
func getSum(probs []ProbResult) float64 {
	sum := 0.0
	for _, prob := range probs {
		sum += prob.Probability
	}
	return sum
}

// Нормализует вероятности так, чтобы их сумма была равна единице.
// При нулевой сумме возвращает пустой набор.
func normalize(probs []ProbResult) []ProbResult {
	sum := getSum(probs)
	res := make([]ProbResult, 0, len(probs))
	if sum <= 0 {
		return res
	}

	for _, prob := range probs {
		res = append(res, ProbResult{
			Attractor:   prob.Attractor,
			Probability: prob.Probability / sum,
		})
	}

	return res
}

// Разделяет положительные и отрицательные события.
func split(probs []ProbResult) ([]ProbResult, []ProbResult) { // returns positive, negative probs
	positive := make([]ProbResult, 0)
	negative := make([]ProbResult, 0)
	for _, prob := range probs {
		if prob.Probability <= 0 {
			continue
		}
		if prob.Attractor.EventState.EventData.IsPositive {
			positive = append(positive, prob)
		} else {
			negative = append(negative, prob)
		}
	}

	return positive, negative
}

// Выбирает один результат случайным пропорционально его вероятности.
func pick(probs *[]ProbResult) (res ProbResult) {
	score := rand.Float64()
	acc := 0.0
	for i, prob := range *probs {
		acc += prob.Probability
		if acc >= score {
			res = prob
			*probs = append((*probs)[:i], (*probs)[i+1:]...)
			break
		}

	}
	return
}

// Normalize подготавливает вероятности к внешнему использованию.
func Normalize(probs []ProbResult) []ProbResult {
	return normalize(probs)
}

// Рассчитывает количество и случайный набор событий текущего цикла.
func (g *GameState) PickEvents(probs []ProbResult) (res []ProbResult, err error) {
	pos, neg := split(probs)
	ePos := getSum(pos)
	eNeg := getSum(neg)
	_eNeg := eNeg
	if int(math.Round(ePos))+int(math.Round(eNeg)) > g.GameData.MaxEventsPerTurn {
		proportion := float64(g.GameData.MaxEventsPerTurn) / (ePos + eNeg)
		ePos *= proportion
		eNeg *= proportion
	}

	cNeg := int(math.Round(eNeg))
	cPos := int(math.Round(ePos))

	if len(neg) >= 1 && cNeg == 0 && _eNeg >= g.GameData.Softness {
		cNeg = 1
	}

	if cNeg+cPos == 0 {
		if len(neg) > 0 {
			cNeg++
		} else if len(pos) > 0 {
			cPos++
		} else {
			err = errors.New("No probable Events at all")
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
