package types

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// decisionJSON validates the serialized shape, including typed Go containers.
func decisionJSON(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func decisionDescription(value any, nullable bool) bool {
	switch value.(type) {
	case string, map[string]any, []any:
		return true
	case nil:
		return nullable
	default:
		return false
	}
}

// Validate rejects invalid request shapes before provider invocation.
func (r DecisionsRequest) Validate() error {
	var errs ValidationErrors
	if strings.TrimSpace(r.Model) == "" {
		errs.Add("model", "required", nil, "model must be specified")
	}
	state, err := decisionJSON(r.State)
	if err != nil || !decisionDescription(state, false) {
		errs.Add("state", "json_shape", nil, "state must serialize to a JSON string, object or array")
	}
	if len(r.Questions) == 0 {
		errs.Add("questions", "required", nil, "at least one question must be provided")
	}
	for name, q := range r.Questions {
		field := "questions." + name
		instructions, err := decisionJSON(q.Instructions)
		if err != nil || !decisionDescription(instructions, false) {
			errs.Add(field+".instructions", "json_shape", nil, "instructions must serialize to a JSON string, object or array")
		}
		criteria, err := decisionJSON(q.Criteria)
		if err != nil {
			errs.Add(field+".criteria", "json_shape", nil, "criteria must be valid JSON")
			continue
		}
		valid := true
		switch q.Type {
		case DecisionNoul:
			if criteria == nil {
				break
			}
			object, ok := criteria.(map[string]any)
			valid = ok
			for key, description := range object {
				if (key != "true" && key != "false") || !decisionDescription(description, false) {
					valid = false
				}
			}
		case DecisionChoice:
			object, ok := criteria.(map[string]any)
			valid = ok && len(object) > 0 && len(object) <= 255
			for _, description := range object {
				if !decisionDescription(description, true) {
					valid = false
				}
			}
		case DecisionScore:
			levels, ok := criteria.([]any)
			valid = ok && len(levels) >= 2 && len(levels) <= 10
			for _, description := range levels {
				if !decisionDescription(description, false) {
					valid = false
				}
			}
		default:
			errs.Add(field+".type", "enum", nil, "type must be noul, choice or score")
			continue
		}
		if !valid {
			errs.Add(field+".criteria", "decision_criteria", nil, "criteria do not match the decision family")
		}
	}
	return errs.Error()
}

func decisionUnit(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

// ValidateFor checks required answers, ranges and distribution keys. It leaves
// provider probabilities untouched and does not require floating-point sums of 1.
func (r DecisionsResponse) ValidateFor(request DecisionsRequest) error {
	for name, question := range request.Questions {
		answer, ok := r.Answers[name]
		if !ok || answer.Type != question.Type {
			return fmt.Errorf("answer %q is missing or has an unexpected type", name)
		}
		criteria, err := decisionJSON(question.Criteria)
		if err != nil {
			return fmt.Errorf("invalid request criteria for %q", name)
		}
		keys := make(map[string]bool)
		switch question.Type {
		case DecisionNoul:
			if answer.Noul == nil || !decisionUnit(*answer.Noul) {
				return fmt.Errorf("answer %q requires noul in [0,1]", name)
			}
		case DecisionChoice:
			options, ok := criteria.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid choice criteria for %q", name)
			}
			for key := range options {
				keys[key] = true
			}
			if answer.Choice == nil || !keys[*answer.Choice] {
				return fmt.Errorf("answer %q requires a choice from its criteria", name)
			}
		case DecisionScore:
			levels, ok := criteria.([]any)
			if !ok {
				return fmt.Errorf("invalid score criteria for %q", name)
			}
			if answer.Score == nil || math.IsNaN(*answer.Score) || math.IsInf(*answer.Score, 0) || *answer.Score < 0 || *answer.Score > float64(len(levels)-1) {
				return fmt.Errorf("answer %q has an invalid score", name)
			}
			for i := range levels {
				keys[strconv.Itoa(i)] = true
			}
			if len(answer.Legend) != len(keys) {
				return fmt.Errorf("answer %q requires a complete legend", name)
			}
			for key := range keys {
				if _, ok := answer.Legend[key]; !ok {
					return fmt.Errorf("answer %q is missing legend level %q", name, key)
				}
			}
		default:
			return fmt.Errorf("answer %q has an unknown decision type", name)
		}
		if question.Type != DecisionNoul {
			if answer.Confidence == nil || !decisionUnit(*answer.Confidence) {
				return fmt.Errorf("answer %q requires confidence in [0,1]", name)
			}
			if len(answer.Probabilities) != len(keys) {
				return fmt.Errorf("answer %q requires a complete probability distribution", name)
			}
			for key := range keys {
				value, ok := answer.Probabilities[key]
				if !ok || !decisionUnit(value) {
					return fmt.Errorf("answer %q has an invalid probability for %q", name, key)
				}
			}
		}
		// Even optional known confidence/probability fields must have valid ranges.
		if answer.Confidence != nil && !decisionUnit(*answer.Confidence) {
			return fmt.Errorf("answer %q has invalid confidence", name)
		}
		for _, value := range answer.Probabilities {
			if !decisionUnit(value) {
				return fmt.Errorf("answer %q has invalid probabilities", name)
			}
		}
	}
	return nil
}
