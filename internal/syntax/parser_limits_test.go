package syntax

import (
	"errors"
	"testing"

	"github.com/MyungSub0519/gopd/internal/pdfmodel"
)

func TestParseObjectValueBudget(t *testing.T) {
	for _, tt := range []struct {
		name       string
		input      string
		remaining  int
		wantValues int
		wantError  bool
	}{
		{"exact array", "[1 [2]]", 4, 4, false},
		{"array overflow", "[1 [2]]", 3, 3, true},
		{"dictionary values", "<< /A 1 /B [2] >>", 4, 4, false},
		{"reference is one value", "1 0 R", 1, 1, false},
		{"empty remaining", "null", 0, 0, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, used, err := ParseObjectWithValueBudget([]byte(tt.input), 1, 0, pdfmodel.Limits{}, tt.remaining)
			if (err != nil) != tt.wantError || used != tt.wantValues {
				t.Fatalf("used=%d err=%v; want used=%d error=%v", used, err, tt.wantValues, tt.wantError)
			}
		})
	}
}

func TestParseObjectConfiguredValueLimit(t *testing.T) {
	if _, _, err := ParseObjectWithLimits([]byte("[1 2 3]"), 1, 0, pdfmodel.Limits{MaxValues: 3}); err == nil {
		t.Fatal("array plus three values exceeded configured value limit")
	}
	if _, _, err := ParseObjectWithLimits([]byte("null"), 1, 0, pdfmodel.Limits{MaxValues: -1}); err == nil {
		t.Fatal("negative value limit accepted")
	}
}

func TestSyntaxResourceErrorsAreClassifiable(t *testing.T) {
	for _, tt := range []struct {
		input  string
		limits pdfmodel.Limits
	}{
		{"[1 2]", pdfmodel.Limits{MaxValues: 2}},
		{"[[0]]", pdfmodel.Limits{MaxDepth: 1}},
		{"(abc)", pdfmodel.Limits{MaxTokenBytes: 4}},
	} {
		_, _, err := ParseObjectWithLimits([]byte(tt.input), 1, 0, tt.limits)
		if !errors.Is(err, pdfmodel.ErrLimit) {
			t.Fatalf("%q: expected resource error, got %v", tt.input, err)
		}
	}
}
