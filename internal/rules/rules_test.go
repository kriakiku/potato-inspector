package rules

import (
	"testing"
)

func TestParseResult(t *testing.T) {
	r := parseResult(map[string]any{"dest": "aws-eu-central-1", "delay_ms": 12})
	if r.Dest != "aws-eu-central-1" || r.DelayMs != 12 {
		t.Fatalf("%+v", r)
	}
}

func TestMatchRegex(t *testing.T) {
	ok, err := matchRegex(`(?i)cloudfront`, "Via: 1.1 cloudfront.net")
	if err != nil || !ok {
		t.Fatal(ok, err)
	}
}
