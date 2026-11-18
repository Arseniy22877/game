package game

import (
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/Arseniy22877/game/internal/api"
	"github.com/Arseniy22877/game/internal/enginebridge"
	"github.com/Arseniy22877/game/internal/store"
)

func (m *Manager) replay(j store.Journal) error {
	g := j.Game
	var scenario *enginebridge.Scenario
	switch g.Phase {
	case api.PhaseLobby:
	case api.PhaseRoundOpen, api.PhaseRoundClosed, api.PhaseFinished:
		var err error
		scenario, err = m.catalog.ParseSnapshot(g.ScenarioID, g.ScenarioJSON)
		if err != nil {
			return fmt.Errorf("replay game %d: %w", g.ID, err)
		}
	default:
		return fmt.Errorf("replay game %d: unknown phase %q", g.ID, g.Phase)
	}

	teams := make([]team, 0, len(j.Teams))
	for _, record := range j.Teams {
		t := team{record: record.Team}
		if scenario != nil {
			t.instance = scenario.NewInstance()
			if err := t.replayRounds(g, record.Rounds); err != nil {
				return fmt.Errorf("replay game %d team %d: %w", g.ID, record.ID, err)
			}
		}
		teams = append(teams, t)
	}
	m.teams = teams
	m.scenario = scenario
	m.info = gameInfo(g)
	return nil
}

func (t *team) replayRounds(g store.Game, journal []store.JournalRound) error {
	closedThrough := g.Round
	if g.Phase == api.PhaseRoundOpen {
		closedThrough--
	}
	if len(journal) < closedThrough || len(journal) > g.Round || closedThrough < 0 {
		return fmt.Errorf("round %d: incomplete round journal", g.Round)
	}
	for i, entry := range journal {
		round := i + 1
		if entry.Round != round {
			return fmt.Errorf("round %d: journal has round %d", round, entry.Round)
		}
		t.setRound(round)
		eventsApplied := round < g.Round || g.Phase == api.PhaseFinished
		if err := t.replayRound(entry, round <= closedThrough, eventsApplied); err != nil {
			return fmt.Errorf("round %d: %w", round, err)
		}
	}
	if g.Phase == api.PhaseFinished {
		t.finish(g.Round)
	} else if len(journal) < g.Round {
		t.setRound(g.Round)
	}
	return nil
}

func (t *team) replayRound(entry store.JournalRound, closed, eventsApplied bool) error {
	if closed != (entry.Result != nil) {
		return errors.New("calculation does not match game phase")
	}
	t.decision = entry.Decision
	if t.decision != nil {
		if t.bankrupt || (t.decision.Auto && !closed) {
			return errors.New("unexpected decision in round journal")
		}
		cost, err := t.instance.Submit(t.decision.Decision)
		if err != nil {
			return fmt.Errorf("decision: %w", err)
		}
		if cost != t.decision.Cost {
			return errors.New("decision cost differs from journal")
		}
	}
	if !closed {
		return nil
	}

	result := entry.Result
	if eventsApplied != (result.FiredEventIDs != nil) {
		return errors.New("event selection does not match game phase")
	}
	var predictions []api.Prediction
	var err error
	if t.decision == nil {
		if !t.bankrupt {
			return errors.New("missing decision for a team that could play")
		}
		predictions, err = t.instance.SkipRound()
	} else {
		predictions, err = t.instance.Calculate()
	}
	if err != nil {
		return fmt.Errorf("calculation: %w", err)
	}
	if !slices.EqualFunc(predictions, result.Predictions, func(a, b api.Prediction) bool {
		return a.Event.ID == b.Event.ID && a.Probability == b.Probability
	}) {
		return errors.New("predictions differ from journal")
	}
	if err := t.verifySnapshot(result.StateAfterCalc); err != nil {
		return fmt.Errorf("after calculation: %w", err)
	}
	t.predictions = result.Predictions
	if !eventsApplied {
		return nil
	}

	events, err := result.Events()
	if err != nil {
		return err
	}
	if err := t.instance.Fire(result.FiredEventIDs); err != nil {
		return fmt.Errorf("apply events: %w", err)
	}
	t.recordEvents(api.RoundEvents{Round: entry.Round, Events: events})
	return t.verifySnapshot(*result.StateAfterEvents)
}

func (t *team) verifySnapshot(saved api.TeamGameState) error {
	state := t.state()
	if !reflect.DeepEqual(state.History, saved.History) {
		return errors.New("event history differs from journal")
	}
	state.History, saved.History = nil, nil
	if !reflect.DeepEqual(state, saved) {
		return errors.New("team state differs from journal")
	}
	return nil
}
