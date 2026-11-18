package enginebridge

import (
	"bytes"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"

	e "github.com/Arseniy22877/game/gameengine"
	"github.com/Arseniy22877/game/internal/api"
)

type Catalog struct {
	dir       string
	scenarios map[string]*Scenario
}
type Scenario struct {
	id     string
	raw    []byte
	game   e.Game
	images map[string]string
}

const BaseRoundCost = e.CycleBaseCost

func ParseScenario(id string, raw []byte) (*Scenario, error) {
	g, err := e.ParseGameJSON(raw)
	if err != nil {
		return nil, err
	}
	if len(g.Events) == 0 {
		return nil, fmt.Errorf("scenario %q has no events", id)
	}
	return &Scenario{id: id, raw: bytes.Clone(raw), game: g}, nil
}

func LoadCatalog(dir string) (*Catalog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	c := &Catalog{dir: dir, scenarios: make(map[string]*Scenario)}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		raw, err := fs.ReadFile(root.FS(), name)
		id := strings.TrimSuffix(name, ".json")
		var s *Scenario
		if err == nil {
			s, err = ParseScenario(id, raw)
		}
		if err != nil {
			slog.Warn("skip scenario", "file", name, "error", err)
			continue
		}
		s.resolveImages(root)
		c.scenarios[id] = s
	}
	return c, nil
}

// ParseSnapshot replays the saved definition even if its catalog file has changed.
func (c *Catalog) ParseSnapshot(id string, raw []byte) (*Scenario, error) {
	s, err := ParseScenario(id, raw)
	if err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(c.dir)
	if err == nil {
		defer root.Close()
		s.resolveImages(root)
	}
	return s, nil
}
func (s *Scenario) resolveImages(root *os.Root) {
	s.images = map[string]string{}
	for _, event := range s.game.Events {
		s.images[event.ImageURL] = resolveImage(root, s.id, event.ImageURL)
	}
}

func resolveImage(root *os.Root, id, image string) string {
	if remoteImage(image) {
		return image
	}
	name, ok := strings.CutPrefix(image, "images/")
	if !ok || !fs.ValidPath(name) || name == "." {
		return ""
	}
	sets := []string{id}
	if id == "republic_n_scenario" {
		sets = append(sets, "republic_n_scenario_v2")
	}
	for _, set := range sets {
		info, err := root.Stat(filepath.Join(set, "images", name))
		if err == nil && info.Mode().IsRegular() {
			return (&url.URL{Path: "/images/" + set + "/" + name}).String()
		}
	}
	return ""
}

func remoteImage(image string) bool {
	u, err := url.Parse(image)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

func (c *Catalog) IDs() []string                   { return slices.Sorted(maps.Keys(c.scenarios)) }
func (c *Catalog) Get(id string) (*Scenario, bool) { s, ok := c.scenarios[id]; return s, ok }
func (s *Scenario) Raw() []byte                    { return bytes.Clone(s.raw) }
func (s *Scenario) event(v e.GameEvent) api.Event {
	image := s.images[v.ImageURL]
	if remoteImage(v.ImageURL) {
		image = v.ImageURL
	}
	return api.Event{ID: int(v.ID), Name: v.Name, Description: v.Description, ImageURL: image, IsPositive: v.IsPositive}
}

func (s *Scenario) Opening() *api.Event {
	for _, v := range s.game.Events {
		if v.ID == s.game.OpeningEvent {
			dto := s.event(v)
			return &dto
		}
	}
	return nil
}

func (s *Scenario) NewInstance() *Instance {
	state := s.game.CreateInstance()
	state.ID = s.id
	return &Instance{scenario: s, state: state}
}
