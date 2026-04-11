package aws

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"testing"

	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"

	"github.com/embano1/transcribe-meetings/internal/types"
)

type fakeS3Client struct {
	headObjectErr error
	putObjectErr  error
	getObjectOut  *awss3.GetObjectOutput
	getObjectErr  error
	headBucketErr error
}

func (f *fakeS3Client) HeadObject(context.Context, *awss3.HeadObjectInput, ...func(*awss3.Options)) (*awss3.HeadObjectOutput, error) {
	return &awss3.HeadObjectOutput{}, f.headObjectErr
}

func (f *fakeS3Client) PutObject(context.Context, *awss3.PutObjectInput, ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	return &awss3.PutObjectOutput{}, f.putObjectErr
}

func (f *fakeS3Client) GetObject(context.Context, *awss3.GetObjectInput, ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	if f.getObjectErr != nil {
		return nil, f.getObjectErr
	}
	return f.getObjectOut, nil
}

func (f *fakeS3Client) HeadBucket(context.Context, *awss3.HeadBucketInput, ...func(*awss3.Options)) (*awss3.HeadBucketOutput, error) {
	return &awss3.HeadBucketOutput{}, f.headBucketErr
}

func TestCheckObjectExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		err     error
		want    bool
		wantErr bool
	}{
		{
			name: "object exists",
			want: true,
		},
		{
			name: "not found",
			err:  &smithy.GenericAPIError{Code: "404", Message: "not found"},
			want: false,
		},
		{
			name:    "unexpected error",
			err:     errors.New("boom"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			service := NewS3Service(&fakeS3Client{headObjectErr: tt.err})
			got, err := service.CheckObjectExists(context.Background(), "bucket", "key")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckObjectExists returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("CheckObjectExists() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestGetTranscriptFromS3PlainTranscript(t *testing.T) {
	t.Parallel()

	service := NewS3Service(&fakeS3Client{
		getObjectOut: &awss3.GetObjectOutput{Body: mustFixtureBody(t, "testdata/plain_transcript.json")},
	})

	got, err := service.GetTranscriptFromS3(context.Background(), "bucket", "plain.json", &types.AppConfig{})
	if err != nil {
		t.Fatalf("GetTranscriptFromS3 returned error: %v", err)
	}
	if got != "Hello world." {
		t.Fatalf("GetTranscriptFromS3() = %q", got)
	}
}

func TestGetTranscriptFromS3FormatsDiarizedTranscript(t *testing.T) {
	t.Parallel()

	service := NewS3Service(&fakeS3Client{
		getObjectOut: &awss3.GetObjectOutput{Body: mustFixtureBody(t, "testdata/diarized_transcript.json")},
	})

	got, err := service.GetTranscriptFromS3(context.Background(), "bucket", "diarized.json", &types.AppConfig{SpeakerDiarization: true})
	if err != nil {
		t.Fatalf("GetTranscriptFromS3 returned error: %v", err)
	}

	want := "Speaker 0: Hello world.\n\nSpeaker 1: Hi there."
	if got != want {
		t.Fatalf("GetTranscriptFromS3() = %q, want %q", got, want)
	}
}

func TestGetTranscriptFromS3UsesPlainTranscriptWhenDiarizationDisabled(t *testing.T) {
	t.Parallel()

	service := NewS3Service(&fakeS3Client{
		getObjectOut: &awss3.GetObjectOutput{Body: mustFixtureBody(t, "testdata/diarized_transcript.json")},
	})

	got, err := service.GetTranscriptFromS3(context.Background(), "bucket", "diarized.json", &types.AppConfig{SpeakerDiarization: false})
	if err != nil {
		t.Fatalf("GetTranscriptFromS3 returned error: %v", err)
	}
	if got != "Hello world. Hi there." {
		t.Fatalf("GetTranscriptFromS3() = %q", got)
	}
}

func TestGetTranscriptFromS3ReturnsErrorWhenTranscriptMissing(t *testing.T) {
	t.Parallel()

	service := NewS3Service(&fakeS3Client{
		getObjectOut: &awss3.GetObjectOutput{
			Body: io.NopCloser(bytes.NewReader([]byte(`{"results":{"transcripts":[]}}`))),
		},
	})

	_, err := service.GetTranscriptFromS3(context.Background(), "bucket", "empty.json", &types.AppConfig{})
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "no transcript found in result" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestHeadBucket(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("denied")
	service := NewS3Service(&fakeS3Client{headBucketErr: wantErr})
	err := service.HeadBucket(context.Background(), "bucket")
	if !errors.Is(err, wantErr) {
		t.Fatalf("HeadBucket() error = %v, want %v", err, wantErr)
	}
}

func mustFixtureBody(t *testing.T, path string) io.ReadCloser {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", path, err)
	}
	return io.NopCloser(bytes.NewReader(data))
}
