package cmd

import (
	"testing"

	"cc-switch-cli/internal/tester"
)

func TestShouldExitNonZero(t *testing.T) {
	tests := []struct {
		name    string
		results []tester.Result
		strict  bool
		want    bool
	}{
		{"no results", nil, false, false},
		{"all ok", []tester.Result{{Provider: "a", Status: tester.StatusOK}}, false, false},
		{"one fail", []tester.Result{
			{Provider: "a", Status: tester.StatusOK},
			{Provider: "b", Status: tester.StatusFail},
		}, false, true},
		{"unsupported tolerated by default", []tester.Result{
			{Provider: "a", Status: tester.StatusUnsupported},
		}, false, false},
		{"unsupported fails under strict", []tester.Result{
			{Provider: "a", Status: tester.StatusUnsupported},
		}, true, true},
		{"fail wins even under strict", []tester.Result{
			{Provider: "a", Status: tester.StatusUnsupported},
			{Provider: "b", Status: tester.StatusFail},
		}, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldExitNonZero(tt.results, tt.strict); got != tt.want {
				t.Errorf("shouldExitNonZero(strict=%v) = %v, want %v", tt.strict, got, tt.want)
			}
		})
	}
}
