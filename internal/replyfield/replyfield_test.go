package replyfield

import (
	"slices"
	"testing"
)

func TestALabelWithTheOldSeparatorSurvivesTheRoundTrip(t *testing.T) {
	in := []string{"Now", "Today | 18:00", "[later]"}
	if got := Decode(Encode(in)); !slices.Equal(got, in) {
		t.Fatalf("round trip: got %q, want %q", got, in)
	}
}

func TestTheOldJoinedFormStillReads(t *testing.T) {
	if got := Decode("Now | Today 20:28"); !slices.Equal(got, []string{"Now", "Today 20:28"}) {
		t.Fatalf("old form: got %q", got)
	}
}

func TestNoButtonsIsAnEmptyField(t *testing.T) {
	if Encode(nil) != "" || Decode("") != nil {
		t.Fatal("no buttons must be an empty field both ways")
	}
}

func TestAnOldLabelThatLooksLikeJSONIsNotLost(t *testing.T) {
	// An old record whose first label began with "[" is not a JSON array.
	if got := Decode("[x | y"); !slices.Equal(got, []string{"[x", "y"}) {
		t.Fatalf("got %q", got)
	}
}
