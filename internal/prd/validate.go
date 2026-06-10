package prd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

var allowedFields = map[string]struct{}{
	"category":    {},
	"description": {},
	"steps":       {},
	"passes":      {},
}

// ValidateFile validates a PRD JSON array file using the strict import schema.
func ValidateFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read PRD file: %w", err)
	}
	return Validate(data)
}

// Validate validates PRD JSON array bytes using the strict import schema.
func Validate(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	var items []map[string]json.RawMessage
	if err := decoder.Decode(&items); err != nil {
		return fmt.Errorf("decode PRD JSON array: %w", err)
	}
	if err := ensureNoTrailingJSON(decoder); err != nil {
		return err
	}

	seenDescriptions := make(map[string]int, len(items))
	for index, raw := range items {
		if raw == nil {
			return fmt.Errorf("item %d must be an object", index)
		}
		item, err := validateItem(index, raw)
		if err != nil {
			return err
		}

		descriptionKey := strings.TrimSpace(item.Description)
		if previous, ok := seenDescriptions[descriptionKey]; ok {
			return fmt.Errorf("item %d duplicates description from item %d: %q", index, previous, item.Description)
		}
		seenDescriptions[descriptionKey] = index
	}

	return nil
}

func ensureNoTrailingJSON(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == nil {
		return errors.New("PRD file must contain one top-level JSON array")
	} else if err != io.EOF {
		return fmt.Errorf("decode trailing PRD JSON: %w", err)
	}
	return nil
}

func validateItem(index int, raw map[string]json.RawMessage) (Item, error) {
	for field := range raw {
		if _, ok := allowedFields[field]; !ok {
			return Item{}, fmt.Errorf("item %d has unknown field %q", index, field)
		}
	}

	category, err := requiredNonEmptyString(raw, "category")
	if err != nil {
		return Item{}, fmt.Errorf("item %d field category: %w", index, err)
	}
	description, err := requiredNonEmptyString(raw, "description")
	if err != nil {
		return Item{}, fmt.Errorf("item %d field description: %w", index, err)
	}
	steps, err := requiredSteps(raw)
	if err != nil {
		return Item{}, fmt.Errorf("item %d field steps: %w", index, err)
	}
	passes, err := requiredBool(raw, "passes")
	if err != nil {
		return Item{}, fmt.Errorf("item %d field passes: %w", index, err)
	}

	return Item{
		Category:    category,
		Description: description,
		Steps:       steps,
		Passes:      passes,
	}, nil
}

func requiredNonEmptyString(raw map[string]json.RawMessage, field string) (string, error) {
	value, ok := raw[field]
	if !ok {
		return "", errors.New("required")
	}
	var decoded *string
	if err := json.Unmarshal(value, &decoded); err != nil || decoded == nil {
		return "", errors.New("must be a string")
	}
	if strings.TrimSpace(*decoded) == "" {
		return "", errors.New("must be non-empty")
	}
	return *decoded, nil
}

func requiredSteps(raw map[string]json.RawMessage) ([]string, error) {
	value, ok := raw["steps"]
	if !ok {
		return nil, errors.New("required")
	}
	var steps *[]string
	if err := json.Unmarshal(value, &steps); err != nil || steps == nil {
		return nil, errors.New("must be an array of strings")
	}
	for index, step := range *steps {
		if strings.TrimSpace(step) == "" {
			return nil, fmt.Errorf("item %d must be non-empty", index)
		}
	}
	return append([]string(nil), (*steps)...), nil
}

func requiredBool(raw map[string]json.RawMessage, field string) (bool, error) {
	value, ok := raw[field]
	if !ok {
		return false, errors.New("required")
	}
	var decoded *bool
	if err := json.Unmarshal(value, &decoded); err != nil || decoded == nil {
		return false, errors.New("must be a boolean")
	}
	return *decoded, nil
}
