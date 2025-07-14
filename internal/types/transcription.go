package types

// TranscriptionResult represents the JSON structure returned by Transcribe.
type TranscriptionResult struct {
	Results struct {
		Transcripts []struct {
			Transcript string `json:"transcript"`
		} `json:"transcripts"`
		SpeakerLabels *SpeakerLabels `json:"speaker_labels,omitempty"`
		Items         []Item         `json:"items,omitempty"`
	} `json:"results"`
	Status string `json:"status"`
}

// SpeakerLabels contains speaker diarization information
type SpeakerLabels struct {
	Speakers int `json:"speakers"`
	Segments []struct {
		StartTime    string `json:"start_time"`
		EndTime      string `json:"end_time"`
		SpeakerLabel string `json:"speaker_label"`
		Items        []struct {
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
		} `json:"items"`
	} `json:"segments"`
}

// Item represents individual words/items in the transcription
type Item struct {
	StartTime    string        `json:"start_time,omitempty"`
	EndTime      string        `json:"end_time,omitempty"`
	Type         string        `json:"type"`
	Alternatives []Alternative `json:"alternatives"`
	SpeakerLabel string        `json:"speaker_label,omitempty"`
}

// Alternative represents word alternatives
type Alternative struct {
	Confidence string `json:"confidence"`
	Content    string `json:"content"`
}

// AppConfig holds the command-line parameters.
type AppConfig struct {
	InputFilePath      string
	OutputFilePath     string
	BucketName         string
	Region             string
	LanguageCode       string
	SpeakerDiarization bool
	MaxSpeakers        int
	Force              bool
}
