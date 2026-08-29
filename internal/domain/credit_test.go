package domain

import "testing"

func TestParseCreditsUsesMilliUnits(t *testing.T) {
	for _, test := range []struct {
		input string
		want  int64
	}{
		{"0", 0}, {"0.7", 700}, {"2000.000", 2_000_000}, {"1.005", 1005},
	} {
		got, err := ParseCredits(test.input)
		if err != nil || got != test.want {
			t.Fatalf("ParseCredits(%q) = %d, %v; want %d", test.input, got, err, test.want)
		}
	}
}

func TestParseCreditsRejectsUnsafeForms(t *testing.T) {
	for _, input := range []string{"-1", "+1", "1e3", "1.0000", "1000000001.000"} {
		if _, err := ParseCredits(input); err == nil {
			t.Fatalf("ParseCredits(%q) unexpectedly succeeded", input)
		}
	}
}
