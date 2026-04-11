package formatting

import (
	"testing"

	"github.com/embano1/transcribe-meetings/internal/types"
)

func TestFormatTranscriptWithSpeakers(t *testing.T) {
	t.Parallel()

	result := &types.TranscriptionResult{}
	result.Results.Items = []types.Item{
		{Type: "pronunciation", SpeakerLabel: "spk_0", Alternatives: []types.Alternative{{Content: "Hello"}}},
		{Type: "pronunciation", SpeakerLabel: "spk_0", Alternatives: []types.Alternative{{Content: "world"}}},
		{Type: "punctuation", Alternatives: []types.Alternative{{Content: "."}}},
		{Type: "pronunciation", SpeakerLabel: "spk_1", Alternatives: []types.Alternative{{Content: "Hi"}}},
		{Type: "pronunciation", SpeakerLabel: "spk_1", Alternatives: []types.Alternative{{Content: "there"}}},
		{Type: "punctuation", Alternatives: []types.Alternative{{Content: "!"}}},
	}

	got := FormatTranscriptWithSpeakers(result)
	want := "Speaker 0: Hello world.\n\nSpeaker 1: Hi there!"
	if got != want {
		t.Fatalf("FormatTranscriptWithSpeakers() = %q, want %q", got, want)
	}
}

func TestFormatTranscriptWithSpeakersWithoutLabels(t *testing.T) {
	t.Parallel()

	result := &types.TranscriptionResult{}
	result.Results.Items = []types.Item{
		{Type: "pronunciation", Alternatives: []types.Alternative{{Content: "Hello"}}},
		{Type: "pronunciation", Alternatives: []types.Alternative{{Content: "again"}}},
		{Type: "punctuation", Alternatives: []types.Alternative{{Content: "."}}},
	}

	got := FormatTranscriptWithSpeakers(result)
	want := "Hello again."
	if got != want {
		t.Fatalf("FormatTranscriptWithSpeakers() = %q, want %q", got, want)
	}
}
