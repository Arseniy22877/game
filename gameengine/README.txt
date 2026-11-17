Game engine parser and instance creation

Files in this package:
- api.go: public backend-facing API and scenario registry
- parser_create.go: JSON parsing, migration, validation, saving and Game.CreateInstance
- types.go: game and runtime data structures
- rules.go: validation and state transition rules
- probability.go: event weights and branch selection
- game_cycle.go: transactional state-cloning helpers

Important behavior:
1. Event Weights must have exactly one value per attribute.
2. Weight order is BasicAttributes first, then ConfigurableAttributes.
3. Iteration == 0 means an event is not tied to a particular iteration.
4. Legacy Thresholds using Value + Higher are migrated to Min/Max ranges.
5. A parsed scenario is validated before it is returned.
6. GetGameInstance deep-copies the scenario before creating the runtime state,
   so independent sessions do not share mutable scenario slices.

Use this package as one unit. Do not keep old duplicate definitions of
ParseGameJSON, CreateGameJSON, ValidateGame or (*Game).CreateInstance.
