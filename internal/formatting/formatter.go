package formatting

import (
	"fmt"
	"strings"

	"github.com/embano1/transcribe-meetings/internal/types"
)

// FormatTranscriptWithSpeakers formats the transcript with speaker labels for better readability
func FormatTranscriptWithSpeakers(result *types.TranscriptionResult) string {
	var formatted strings.Builder
	currentSpeaker := ""

	// Process each item (word) in the transcript
	for _, item := range result.Results.Items {
		switch item.Type {
		case "punctuation":
			// Add punctuation without space
			if len(item.Alternatives) > 0 {
				formatted.WriteString(item.Alternatives[0].Content)
			}
		case "pronunciation":
			// Check if speaker has changed
			if item.SpeakerLabel != "" && item.SpeakerLabel != currentSpeaker {
				currentSpeaker = item.SpeakerLabel
				// Add a new line for new speaker (except for the first speaker)
				if formatted.Len() > 0 {
					formatted.WriteString("\n\n")
				}
				// Convert speaker label to more readable format
				speakerNum := strings.TrimPrefix(currentSpeaker, "spk_")
				formatted.WriteString(fmt.Sprintf("Speaker %s: ", speakerNum))
			}

			// Add the word with a space (unless it's the first word for this speaker)
			if len(item.Alternatives) > 0 {
				word := item.Alternatives[0].Content
				// Only add space if we're not at the beginning of speaker's section
				if !strings.HasSuffix(formatted.String(), ": ") {
					formatted.WriteString(" ")
				}
				formatted.WriteString(word)
			}
		}
	}

	return formatted.String()
}
