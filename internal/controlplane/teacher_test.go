package controlplane

import (
	"testing"

	"github.com/google/uuid"
	"github.com/starprince1234/Nebula-api/internal/infrastructure/db/ent/modelbinding"
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

func TestValidBindingAdapters(t *testing.T) {
	t.Parallel()
	for _, adapter := range []modelbinding.Adapter{
		modelbinding.AdapterOpenaiCompatible,
		modelbinding.AdapterOpenaiResponses,
		modelbinding.AdapterOpenaiEmbeddings,
		modelbinding.AdapterOpenaiImages,
		modelbinding.AdapterOpenaiAudio,
		modelbinding.AdapterOpenaiVideo,
		modelbinding.AdapterOpenaiRealtime,
		modelbinding.AdapterOpenaiModerations,
		modelbinding.AdapterAnthropic,
		modelbinding.AdapterCohereRerankV2,
		modelbinding.AdapterGoogleGeminiV1beta,
	} {
		if !validBindingAdapter(string(adapter)) {
			t.Fatalf("expected adapter %q to be valid", adapter)
		}
	}
	if validBindingAdapter("unknown") {
		t.Fatal("unknown adapter was accepted")
	}
}

func TestMentorCandidateCursorRejectsInvalidInput(t *testing.T) {
	if _, err := DecodeMentorCandidateCursor("not-a-cursor"); err == nil {
		t.Fatal("expected invalid cursor error")
	}
}
