package types

import (
	"encoding/json"
	"strings"
	"testing"
)

func validDecisionRequest() DecisionsRequest {
	return DecisionsRequest{Model: "typesafe/jev-1.13", State: struct {
		Message string `json:"message"`
	}{"hello"}, Questions: map[string]DecisionQuestion{
		"yes":  {Type: DecisionNoul, Instructions: []string{"decide"}},
		"pick": {Type: DecisionChoice, Instructions: map[string]string{"task": "pick"}, Criteria: map[string]any{"a": nil, "b": "B"}},
		"rank": {Type: DecisionScore, Instructions: json.RawMessage(`{"task":"rank"}`), Criteria: []string{"low", "high"}},
	}}
}

func TestDecisionsRequestValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		change func(*DecisionsRequest)
		valid  bool
	}{
		{"typed JSON", func(*DecisionsRequest) {}, true},
		{"empty shapes", func(r *DecisionsRequest) {
			r.State = ""
			r.Questions = map[string]DecisionQuestion{"q": {Type: DecisionNoul, Instructions: []string{}}}
		}, true},
		{"raw arrays", func(r *DecisionsRequest) { r.State = json.RawMessage(`[1,{"large":9007199254740993}]`) }, true},
		{"missing model", func(r *DecisionsRequest) { r.Model = " " }, false},
		{"no questions", func(r *DecisionsRequest) { r.Questions = nil }, false},
		{"null state", func(r *DecisionsRequest) { r.State = json.RawMessage(`null`) }, false},
		{"bool state", func(r *DecisionsRequest) { r.State = true }, false},
		{"number state", func(r *DecisionsRequest) { r.State = 1 }, false},
		{"unmarshalable", func(r *DecisionsRequest) { r.State = make(chan int) }, false},
		{"nil instructions", func(r *DecisionsRequest) { r.Questions["yes"] = DecisionQuestion{Type: DecisionNoul} }, false},
		{"numeric instructions", func(r *DecisionsRequest) { r.Questions["yes"] = DecisionQuestion{Type: DecisionNoul, Instructions: 3} }, false},
		{"unknown type", func(r *DecisionsRequest) { r.Questions["yes"] = DecisionQuestion{Type: "chat", Instructions: "x"} }, false},
		{"noul keys", func(r *DecisionsRequest) {
			r.Questions["yes"] = DecisionQuestion{Type: DecisionNoul, Instructions: "x", Criteria: map[string]string{"maybe": "x"}}
		}, false},
		{"noul descriptions", func(r *DecisionsRequest) {
			r.Questions["yes"] = DecisionQuestion{Type: DecisionNoul, Instructions: "x", Criteria: map[string]any{"true": nil}}
		}, false},
		{"noul criteria", func(r *DecisionsRequest) {
			r.Questions["yes"] = DecisionQuestion{Type: DecisionNoul, Instructions: "x", Criteria: map[string]any{"true": []string{"yes"}, "false": map[string]string{"x": "no"}}}
		}, true},
		{"empty choice", func(r *DecisionsRequest) {
			r.Questions["pick"] = DecisionQuestion{Type: DecisionChoice, Instructions: "x", Criteria: map[string]string{}}
		}, false},
		{"choice boundary", func(r *DecisionsRequest) {
			options := map[string]string{}
			for i := 0; i < 255; i++ {
				options[strings.Repeat("x", i)] = "x"
			}
			r.Questions["pick"] = DecisionQuestion{Type: DecisionChoice, Instructions: "x", Criteria: options}
		}, true},
		{"choice limit", func(r *DecisionsRequest) {
			options := map[string]string{}
			for i := 0; i < 256; i++ {
				options[strings.Repeat("x", i)] = "x"
			}
			r.Questions["pick"] = DecisionQuestion{Type: DecisionChoice, Instructions: "x", Criteria: options}
		}, false},
		{"choice invalid description", func(r *DecisionsRequest) {
			r.Questions["pick"] = DecisionQuestion{Type: DecisionChoice, Instructions: "x", Criteria: map[string]int{"a": 1}}
		}, false},
		{"short score", func(r *DecisionsRequest) {
			r.Questions["rank"] = DecisionQuestion{Type: DecisionScore, Instructions: "x", Criteria: []string{"one"}}
		}, false},
		{"score boundary", func(r *DecisionsRequest) {
			r.Questions["rank"] = DecisionQuestion{Type: DecisionScore, Instructions: "x", Criteria: make([]string, 10)}
		}, true},
		{"long score", func(r *DecisionsRequest) {
			r.Questions["rank"] = DecisionQuestion{Type: DecisionScore, Instructions: "x", Criteria: make([]string, 11)}
		}, false},
		{"null score level", func(r *DecisionsRequest) {
			r.Questions["rank"] = DecisionQuestion{Type: DecisionScore, Instructions: "x", Criteria: []any{"one", nil}}
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := validDecisionRequest()
			tc.change(&r)
			err := r.Validate()
			if (err == nil) != tc.valid {
				t.Fatalf("Validate=%v, want valid=%v", err, tc.valid)
			}
		})
	}
}

func TestDecisionsResponseValidation(t *testing.T) {
	t.Parallel()
	base := `{"answers":{"yes":{"type":"noul","noul":0},"pick":{"type":"choice","choice":"a","probabilities":{"a":0,"b":1},"confidence":0},"rank":{"type":"score","score":0.25,"probabilities":{"0":0.75,"1":0.25},"confidence":0.75,"legend":{"0":"low","1":"high"}}}}`
	cases := []struct {
		name, old, new string
		valid          bool
	}{
		{"valid", "", "", true},
		{"missing answer", `"yes":`, `"other":`, false},
		{"null noul", `"noul":0`, `"noul":null`, false},
		{"noul range", `"noul":0`, `"noul":1.1`, false},
		{"wrong type", `"type":"noul"`, `"type":"choice"`, false},
		{"null choice", `"choice":"a"`, `"choice":null`, false},
		{"unknown choice", `"choice":"a"`, `"choice":"c"`, false},
		{"missing probability", `"a":0,"b":1`, `"a":0`, false},
		{"wrong probability key", `"a":0,"b":1`, `"a":0,"c":1`, false},
		{"null probability", `"a":0,"b":1`, `"a":null,"b":1`, false},
		{"null legend", `"1":"high"`, `"1":null`, false},
		{"probability range", `"a":0,"b":1`, `"a":-0.1,"b":1`, false},
		{"confidence null", `"confidence":0}`, `"confidence":null}`, false},
		{"confidence range", `"confidence":0}`, `"confidence":2}`, false},
		{"null score", `"score":0.25`, `"score":null`, false},
		{"score range", `"score":0.25`, `"score":1.25`, false},
		{"missing legend", `"legend":{"0":"low","1":"high"}`, `"legend":null`, false},
		{"wrong legend key", `"1":"high"`, `"2":"high"`, false},
		{"wrong score distribution", `"1":0.25`, `"2":0.25`, false},
		{"no exact sum constraint", `"a":0,"b":1`, `"a":0.25,"b":0.25`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := base
			if tc.old != "" {
				body = strings.Replace(body, tc.old, tc.new, 1)
			}
			var r DecisionsResponse
			err := json.Unmarshal([]byte(body), &r)
			if err == nil {
				err = r.ValidateFor(validDecisionRequest())
			}
			if (err == nil) != tc.valid {
				t.Fatalf("validation=%v want valid=%v", err, tc.valid)
			}
		})
	}
}
