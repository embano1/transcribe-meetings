package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/embano1/transcribe-meetings/internal/formatting"
	"github.com/embano1/transcribe-meetings/internal/types"
)

// S3Service handles S3 operations
type S3Service struct {
	client *s3.Client
}

// NewS3Service creates a new S3 service
func NewS3Service(client *s3.Client) *S3Service {
	return &S3Service{client: client}
}

// CheckObjectExists uses HeadObject to determine if the object already exists.
func (s *S3Service) CheckObjectExists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		if isNotFoundError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// UploadFile uploads the given file to the specified bucket and key.
func (s *S3Service) UploadFile(ctx context.Context, bucket, key, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   f,
	})
	return err
}

// GetTranscriptFromS3 downloads the transcription result JSON from S3 and extracts the transcript text.
func (s *S3Service) GetTranscriptFromS3(ctx context.Context, bucket, key string, cfg *types.AppConfig) (string, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return "", err
	}
	defer out.Body.Close()

	var result types.TranscriptionResult
	decoder := json.NewDecoder(out.Body)
	if err := decoder.Decode(&result); err != nil {
		return "", err
	}
	if len(result.Results.Transcripts) == 0 {
		return "", fmt.Errorf("no transcript found in result")
	}

	// If speaker diarization is enabled and speaker labels are available, format with speakers
	if cfg.SpeakerDiarization && result.Results.SpeakerLabels != nil {
		return formatting.FormatTranscriptWithSpeakers(&result), nil
	}

	// Default behavior: return simple transcript
	return result.Results.Transcripts[0].Transcript, nil
}

// HeadBucket checks if bucket exists and is accessible
func (s *S3Service) HeadBucket(ctx context.Context, bucket string) error {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: &bucket,
	})
	return err
}

// isNotFoundError determines if an error from AWS indicates a "not found" condition.
func isNotFoundError(err error) bool {
	// TODO: use the various S3 and Transcribe error types
	var apiErr smithy.APIError
	if err == nil {
		return false
	}
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == "NotFoundException" || apiErr.ErrorCode() == "404" {
			return true
		}
	}
	if strings.Contains(err.Error(), "NotFound:") {
		return true
	}
	return false
}
