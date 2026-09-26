package config

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/kzark/gwen/internal/wire"
)

// Patch applies a partial config from PATCH /v1/config to c, then parses and
// validates the result. An unknown section or key, a value of the wrong JSON
// type, or an invalid value is a *KeyError naming the dotted key.
func Patch(c Config, p wire.ConfigPatch) (Config, error) {
	var current map[string]map[string]any
	if err := roundTrip(c.Wire(), &current); err != nil {
		return Config{}, err
	}
	for _, section := range sortedKeys(p) {
		keys, ok := current[section]
		if !ok {
			return Config{}, keyErr(section, "unknown section")
		}
		for _, key := range sortedKeys(p[section]) {
			dotted := section + "." + key
			old, ok := keys[key]
			if !ok {
				return Config{}, keyErr(dotted, "unknown key")
			}
			v := p[section][key]
			if err := sameJSONType(old, v); err != nil {
				return Config{}, keyErr(dotted, "%v", err)
			}
			keys[key] = v
		}
	}
	var w wire.Config
	if err := roundTrip(current, &w); err != nil {
		return Config{}, err
	}
	return FromWire(w)
}

// sameJSONType checks that v, decoded from JSON, has the JSON type of old.
func sameJSONType(old, v any) error {
	switch old.(type) {
	case string:
		if _, ok := v.(string); ok {
			return nil
		}
		return fmt.Errorf("must be a string")
	case bool:
		if _, ok := v.(bool); ok {
			return nil
		}
		return fmt.Errorf("must be true or false")
	case []any:
		list, ok := v.([]any)
		if !ok {
			return fmt.Errorf("must be a list of strings")
		}
		for _, item := range list {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("must be a list of strings")
			}
		}
		return nil
	}
	return fmt.Errorf("unsupported value")
}

func roundTrip(from, to any) error {
	b, err := json.Marshal(from)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := json.Unmarshal(b, to); err != nil {
		return fmt.Errorf("decode config: %w", err)
	}
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
