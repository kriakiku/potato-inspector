package rules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvalDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.expr")
	if err := os.WriteFile(path, []byte(`{ "dest": "cf" }`), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New(path)
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}
	r, err := e.Eval("response", "example.com", "/", nil, map[string]string{"Via": "1.1 cloudfront"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Dest != "cf" {
		t.Fatalf("%+v", r)
	}
}

func TestEvalCloudfront(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.expr")
	src := `if lower(header(response, "via")) contains "cloudfront" { { "dest": "aws-eu-central-1" } } else { { "dest": "cf" } }`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	e := New(path)
	if err := e.Reload(); err != nil {
		t.Fatal(err)
	}
	r, err := e.Eval("response", "cdn.example.com", "/x", nil, map[string]string{"via": "1.1 cloudfront.net (CloudFront)"})
	if err != nil {
		t.Fatal(err)
	}
	if r.Dest != "aws-eu-central-1" {
		t.Fatalf("%+v", r)
	}
}
