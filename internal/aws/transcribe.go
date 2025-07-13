package aws

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/transcribe"
	"github.com/aws/aws-sdk-go-v2/service/transcribe/types"

	appTypes "github.com/embano1/transcribe-meetings/internal/types"
)

// TranscribeService handles Transcribe operations
type TranscribeService struct {
	client *transcribe.Client
}

// NewTranscribeService creates a new Transcribe service
func NewTranscribeService(client *transcribe.Client) *TranscribeService {
	return &TranscribeService{client: client}
}

// EnsureTranscriptionJob checks for an existing transcription job and starts one if not found.
func (t *TranscribeService) EnsureTranscriptionJob(ctx context.Context, jobName, bucket, mediaKey string, cfg *appTypes.AppConfig) error {
	jobExists, jobStatus, err := t.getTranscriptionJobStatus(ctx, jobName)
	if err != nil {
		return fmt.Errorf("checking transcription job status: %w", err)
	}
	if jobExists {
		log.Printf("Transcription job %q already exists with status: %s", jobName, jobStatus)
		return nil
	}

	log.Printf("Starting transcription job...")
	if err := t.startTranscriptionJob(ctx, jobName, bucket, mediaKey, cfg); err != nil {
		return fmt.Errorf("start transcription job: %w", err)
	}
	log.Printf("Transcription job started.")

	// Poll for transcription job completion.
	log.Printf("Waiting for transcription job to complete...")
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

PollLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			_, jobStatus, err := t.getTranscriptionJobStatus(ctx, jobName)
			if err != nil {
				return fmt.Errorf("retrieving transcription job status: %w", err)
			}
			log.Printf("Job status: %s", jobStatus)
			if jobStatus == string(types.TranscriptionJobStatusCompleted) {
				break PollLoop
			} else if jobStatus == string(types.TranscriptionJobStatusFailed) {
				return fmt.Errorf("transcription job failed")
			}
		}
	}
	return nil
}

// getTranscriptionJobStatus checks whether the transcription job exists and returns its status.
func (t *TranscribeService) getTranscriptionJobStatus(ctx context.Context, jobName string) (bool, string, error) {
	out, err := t.client.GetTranscriptionJob(ctx, &transcribe.GetTranscriptionJobInput{
		TranscriptionJobName: &jobName,
	})
	if err != nil {
		if strings.Contains(err.Error(), "The requested job couldn't be found") {
			return false, "", nil
		}
		return false, "", err
	}
	return true, string(out.TranscriptionJob.TranscriptionJobStatus), nil
}

// startTranscriptionJob starts a transcription job using the provided S3 file.
func (t *TranscribeService) startTranscriptionJob(ctx context.Context, jobName, bucket, mediaKey string, cfg *appTypes.AppConfig) error {
	mediaURI := fmt.Sprintf("s3://%s/%s", bucket, mediaKey)
	input := &transcribe.StartTranscriptionJobInput{
		TranscriptionJobName: &jobName,
		LanguageCode:         types.LanguageCode(cfg.LanguageCode),
		MediaFormat:          "m4a",
		Media: &types.Media{
			MediaFileUri: &mediaURI,
		},
		OutputBucketName: &bucket,
	}

	// Add speaker diarization settings if enabled
	if cfg.SpeakerDiarization {
		maxSpeakers := int32(cfg.MaxSpeakers)
		input.Settings = &types.Settings{
			ShowSpeakerLabels:  &cfg.SpeakerDiarization,
			MaxSpeakerLabels:   &maxSpeakers,
		}
	}
	_, err := t.client.StartTranscriptionJob(ctx, input)
	return err
}