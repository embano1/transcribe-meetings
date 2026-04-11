package aws

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/transcribe"
	transcribetypes "github.com/aws/aws-sdk-go-v2/service/transcribe/types"

	appTypes "github.com/embano1/transcribe-meetings/internal/types"
)

type getJobResponse struct {
	output *transcribe.GetTranscriptionJobOutput
	err    error
}

type fakeTranscribeClient struct {
	getResponses []*getJobResponse
	startInputs  []*transcribe.StartTranscriptionJobInput
	deleteInputs []*transcribe.DeleteTranscriptionJobInput
}

func (f *fakeTranscribeClient) GetTranscriptionJob(context.Context, *transcribe.GetTranscriptionJobInput, ...func(*transcribe.Options)) (*transcribe.GetTranscriptionJobOutput, error) {
	if len(f.getResponses) == 0 {
		return nil, errors.New("unexpected GetTranscriptionJob call")
	}

	response := f.getResponses[0]
	f.getResponses = f.getResponses[1:]
	return response.output, response.err
}

func (f *fakeTranscribeClient) DeleteTranscriptionJob(context.Context, *transcribe.DeleteTranscriptionJobInput, ...func(*transcribe.Options)) (*transcribe.DeleteTranscriptionJobOutput, error) {
	f.deleteInputs = append(f.deleteInputs, &transcribe.DeleteTranscriptionJobInput{})
	return &transcribe.DeleteTranscriptionJobOutput{}, nil
}

func (f *fakeTranscribeClient) StartTranscriptionJob(_ context.Context, input *transcribe.StartTranscriptionJobInput, _ ...func(*transcribe.Options)) (*transcribe.StartTranscriptionJobOutput, error) {
	f.startInputs = append(f.startInputs, input)
	return &transcribe.StartTranscriptionJobOutput{}, nil
}

func TestEnsureTranscriptionJobReusesCompletedJob(t *testing.T) {
	t.Parallel()

	client := &fakeTranscribeClient{
		getResponses: []*getJobResponse{
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusCompleted)},
		},
	}
	service := NewTranscribeServiceWithPollInterval(client, time.Millisecond)

	err := service.EnsureTranscriptionJob(context.Background(), "job", "bucket", "uploads/file.m4a", &appTypes.AppConfig{})
	if err != nil {
		t.Fatalf("EnsureTranscriptionJob returned error: %v", err)
	}
	if len(client.startInputs) != 0 {
		t.Fatalf("StartTranscriptionJob called %d times", len(client.startInputs))
	}
	if len(client.deleteInputs) != 0 {
		t.Fatalf("DeleteTranscriptionJob called %d times", len(client.deleteInputs))
	}
}

func TestEnsureTranscriptionJobWaitsForExistingRunningJob(t *testing.T) {
	t.Parallel()

	client := &fakeTranscribeClient{
		getResponses: []*getJobResponse{
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusInProgress)},
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusCompleted)},
		},
	}
	service := NewTranscribeServiceWithPollInterval(client, time.Millisecond)

	err := service.EnsureTranscriptionJob(context.Background(), "job", "bucket", "uploads/file.m4a", &appTypes.AppConfig{})
	if err != nil {
		t.Fatalf("EnsureTranscriptionJob returned error: %v", err)
	}
	if len(client.startInputs) != 0 {
		t.Fatalf("StartTranscriptionJob called %d times", len(client.startInputs))
	}
}

func TestEnsureTranscriptionJobStartsMissingJob(t *testing.T) {
	t.Parallel()

	client := &fakeTranscribeClient{
		getResponses: []*getJobResponse{
			{err: errors.New("The requested job couldn't be found")},
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusCompleted)},
		},
	}
	service := NewTranscribeServiceWithPollInterval(client, time.Millisecond)

	cfg := &appTypes.AppConfig{LanguageCode: "en-US"}
	err := service.EnsureTranscriptionJob(context.Background(), "job", "bucket", "uploads/file.m4a", cfg)
	if err != nil {
		t.Fatalf("EnsureTranscriptionJob returned error: %v", err)
	}
	if len(client.startInputs) != 1 {
		t.Fatalf("StartTranscriptionJob called %d times", len(client.startInputs))
	}

	input := client.startInputs[0]
	if input == nil || input.Media == nil || input.Media.MediaFileUri == nil {
		t.Fatal("expected media input to be set")
	}
	if got := *input.Media.MediaFileUri; got != "s3://bucket/uploads/file.m4a" {
		t.Fatalf("MediaFileUri = %q", got)
	}
}

func TestEnsureTranscriptionJobForceDeletesAndRestartsExistingJob(t *testing.T) {
	t.Parallel()

	client := &fakeTranscribeClient{
		getResponses: []*getJobResponse{
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusCompleted)},
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusCompleted)},
		},
	}
	service := NewTranscribeServiceWithPollInterval(client, time.Millisecond)

	err := service.EnsureTranscriptionJob(context.Background(), "job", "bucket", "uploads/file.m4a", &appTypes.AppConfig{
		LanguageCode: "en-US",
		Force:        true,
	})
	if err != nil {
		t.Fatalf("EnsureTranscriptionJob returned error: %v", err)
	}
	if len(client.deleteInputs) != 1 {
		t.Fatalf("DeleteTranscriptionJob called %d times", len(client.deleteInputs))
	}
	if len(client.startInputs) != 1 {
		t.Fatalf("StartTranscriptionJob called %d times", len(client.startInputs))
	}
}

func TestEnsureTranscriptionJobReturnsFailureStatus(t *testing.T) {
	t.Parallel()

	client := &fakeTranscribeClient{
		getResponses: []*getJobResponse{
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusInProgress)},
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusFailed)},
		},
	}
	service := NewTranscribeServiceWithPollInterval(client, time.Millisecond)

	err := service.EnsureTranscriptionJob(context.Background(), "job", "bucket", "uploads/file.m4a", &appTypes.AppConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "transcription job failed" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnsureTranscriptionJobHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	client := &fakeTranscribeClient{
		getResponses: []*getJobResponse{
			{output: jobStatusResponse(transcribetypes.TranscriptionJobStatusInProgress)},
		},
	}
	service := NewTranscribeServiceWithPollInterval(client, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := service.EnsureTranscriptionJob(ctx, "job", "bucket", "uploads/file.m4a", &appTypes.AppConfig{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EnsureTranscriptionJob() error = %v, want %v", err, context.Canceled)
	}
}

func jobStatusResponse(status transcribetypes.TranscriptionJobStatus) *transcribe.GetTranscriptionJobOutput {
	jobName := "job"

	return &transcribe.GetTranscriptionJobOutput{
		TranscriptionJob: &transcribetypes.TranscriptionJob{
			TranscriptionJobName:   &jobName,
			TranscriptionJobStatus: status,
		},
	}
}
