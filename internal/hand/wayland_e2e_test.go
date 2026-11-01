package hand

import (
	"os"
	"testing"
)

func TestRealWaylandPointer(t *testing.T) {
	if os.Getenv("RIG_HAND_E2E") == "" {
		t.Skip("RIG_HAND_E2E is not set")
	}
	h, err := Open(1)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	screen := h.Screen()
	want := Pt{X: screen.X + screen.W/3, Y: screen.Y + screen.H/2}
	if err := h.MoveTo(want); err != nil {
		t.Fatal(err)
	}
	got, err := h.Pointer()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("live pointer = %+v, want moved position %+v", got, want)
	}
}
