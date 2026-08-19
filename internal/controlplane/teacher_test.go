package controlplane

import (
	"testing"

	"github.com/google/uuid"
)

func TestMentorCandidateCursorRoundTrip(t *testing.T) {
	id := uuid.Must(uuid.NewV7())
	encoded := encodeMentorCandidateCursor("导师甲", id)
	decoded, err := DecodeMentorCandidateCursor(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Name != "导师甲" || decoded.ID != id {
		t.Fatalf("unexpected cursor: %#v", decoded)
	}
}

func TestMentorCandidateCursorRejectsInvalidInput(t *testing.T) {
	if _, err := DecodeMentorCandidateCursor("not-a-cursor"); err == nil {
		t.Fatal("expected invalid cursor error")
	}
}
