package model

import "testing"

func TestValidateNameAccepts(t *testing.T) {
	cases := []string{"claude", "packy-bridge", "mimo_plan", "abc123", "a"}
	for _, c := range cases {
		if err := ValidateName(c); err != nil {
			t.Errorf("ValidateName(%q) rejected valid name: %v", c, err)
		}
	}
}

func TestValidateNameRejects(t *testing.T) {
	cases := map[string]string{
		"":            "empty",
		".":           "dot",
		"..":          "double dot",
		".hidden":     "leading dot",
		" leading":    "leading space",
		"trailing ":   "trailing space",
		"has/slash":   "slash",
		"has\\slash":  "backslash",
		"has:colon":   "colon",
		"has space":   "space",
		"has*star":    "star",
		"CON":         "windows reserved",
		"con":         "windows reserved lowercase",
		"nul":         "windows reserved",
		"path/../etc": "path traversal",
	}
	for name, label := range cases {
		if err := ValidateName(name); err == nil {
			t.Errorf("ValidateName(%q) accepted invalid name (%s)", name, label)
		}
	}
}

func TestValidate(t *testing.T) {
	if err := Validate(nil); err == nil {
		t.Fatal("nil model must be invalid")
	}
	m := &Model{}
	if err := Validate(m); err == nil {
		t.Fatal("model without base URL must be invalid")
	}
	m.Env.BaseURL = "https://api.example.com"
	if err := Validate(m); err == nil {
		t.Fatal("model without model id must be invalid")
	}
	m.Env.Model = "claude"
	if err := Validate(m); err != nil {
		t.Fatalf("valid model rejected: %v", err)
	}
}
