package feedback

import (
	"strings"
	"testing"
)

func TestPrecision(t *testing.T) {
	if (Totals{}).Precision() != nil {
		t.Fatal("no ratings means no precision")
	}
	if p := (Totals{Useful: 7, NotUseful: 3}).Precision(); p == nil || *p != 0.7 {
		t.Fatalf("precision: %v", p)
	}
	if p := (Totals{NotUseful: 2}).Precision(); p == nil || *p != 0 {
		t.Fatalf("all not useful: %v", p)
	}
}

func TestRatingAndNoteValidation(t *testing.T) {
	if !ValidRating(Useful) || !ValidRating(NotUseful) || ValidRating("meh") || ValidRating("") {
		t.Fatal("ratings")
	}
	if n, err := CleanNote("  too noisy \n"); err != nil || n != "too noisy" {
		t.Fatalf("trim: %q %v", n, err)
	}
	for name, bad := range map[string]string{"too long": strings.Repeat("a", MaxNote+1), "control": "bad\x07note"} {
		if _, err := CleanNote(bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := CleanNote(strings.Repeat("é", MaxNote)); err != nil {
		t.Fatalf("the limit counts characters, not bytes: %v", err)
	}
}
