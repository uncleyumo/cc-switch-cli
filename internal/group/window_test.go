package group

import "testing"

func TestParseWindow(t *testing.T) {
	cases := map[string]int64{
		"":        0,
		"0":       0,
		"500k":    500000,
		"567k":    567000,
		"1m":      1000000,
		"2m":      2000000,
		"200K":    200000,
		" 1M ":    1000000,
		"600000":  600000,
		"0.5m":    500000,
	}
	for in, want := range cases {
		got, err := ParseWindow(in)
		if err != nil {
			t.Errorf("ParseWindow(%q) errored: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseWindow(%q) = %d, want %d", in, got, want)
		}
	}
	for _, bad := range []string{"abc", "k", "m", "-5k", "1x"} {
		if _, err := ParseWindow(bad); err == nil {
			t.Errorf("ParseWindow(%q) should fail", bad)
		}
	}
}

func TestFormatWindow(t *testing.T) {
	cases := map[int64]string{
		0:       "",
		500000:  "500k",
		567000:  "567k",
		1000000: "1m",
		2000000: "2m",
		1234:    "1234",
	}
	for in, want := range cases {
		if got := FormatWindow(in); got != want {
			t.Errorf("FormatWindow(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestWindowRoundTrip(t *testing.T) {
	for _, n := range []int64{200000, 500000, 567000, 1000000, 2000000} {
		got, err := ParseWindow(FormatWindow(n))
		if err != nil || got != n {
			t.Errorf("round trip %d -> %q -> %d (%v)", n, FormatWindow(n), got, err)
		}
	}
}
