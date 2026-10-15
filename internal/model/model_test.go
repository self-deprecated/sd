package model

import (
	"strings"
	"testing"
)

var targetOption = TaskOption{Type: "enum", Default: "dev", Values: map[string]OptionValue{
	"dev":  {Vars: map[string]string{"profile": "debug"}},
	"prod": {Vars: map[string]string{"profile": "release"}},
}}

func TestResolveOptionValuesPrecedence(t *testing.T) {
	options := map[string]TaskOption{"target": targetOption}
	cases := []struct {
		name     string
		overlays []map[string]string
		profile  string
	}{
		{"default selection derives vars", nil, "debug"},
		{"overlay selection derives vars", []map[string]string{{"target": "prod"}}, "release"},
		{"explicit value beats derived var", []map[string]string{{"target": "prod"}, {"profile": "explicit"}}, "explicit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values, err := ResolveOptionValues(options, tc.overlays...)
			if err != nil || values["profile"] != tc.profile {
				t.Fatalf("profile = %q err = %v, want %q", values["profile"], err, tc.profile)
			}
		})
	}
}

func TestResolveOptionValuesRejectsConflictingDerivedVars(t *testing.T) {
	options := map[string]TaskOption{
		"first":  {Type: "enum", Default: "on", Values: map[string]OptionValue{"on": {Vars: map[string]string{"shared": "FIRST"}}}},
		"second": {Type: "enum", Default: "on", Values: map[string]OptionValue{"on": {Vars: map[string]string{"shared": "SECOND"}}}},
	}
	_, err := ResolveOptionValues(options)
	if err == nil || !strings.Contains(err.Error(), `"shared"`) {
		t.Fatalf("err = %v, want a conflict on shared", err)
	}
	if _, err := ResolveOptionValues(options, map[string]string{"shared": "explicit"}); err != nil {
		t.Fatalf("explicit value should settle the conflict: %v", err)
	}
}
