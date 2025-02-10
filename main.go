// main.go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/transcribe"
	"github.com/aws/aws-sdk-go-v2/service/transcribe/types"
	"github.com/aws/smithy-go"
)

// TranscriptionResult represents the JSON structure returned by Transcribe.
type TranscriptionResult struct {
	Results struct {
		Transcripts []struct {
			Transcript string `json:"transcript"`
		} `json:"transcripts"`
	} `json:"results"`
	Status string `json:"status"`
}

// AppConfig holds the command-line parameters.
type AppConfig struct {
	InputFilePath  string
	OutputFilePath string
	BucketName     string
	Region         string
}

// build info set by goreleaser
var (
	version = "unknown"
	commit  = "unknown"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		log.Fatalf("Could not transcribe audio: %v", err)
	}
}

func run(ctx context.Context, args []string) error {
	cfgApp, err := newConfig(args)
	if err != nil {
		return err
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfgApp.Region))
	if err != nil {
		return fmt.Errorf("load AWS SDK config: %w", err)
	}

	f, err := os.Open(cfgApp.InputFilePath)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return fmt.Errorf("compute file hash: %w", err)
	}
	f.Close()

	fileHash := hex.EncodeToString(hasher.Sum(nil))[:16] // using first 16 hex digits

	// Use the file hash and original file name to form an S3 key and job name.
	fileName := filepath.Base(cfgApp.InputFilePath)
	s3Key := fmt.Sprintf("uploads/%s_%s", fileHash, fileName)
	jobName := fmt.Sprintf("transcribe-%s", fileHash)

	log.Printf("Using S3 key: %s", s3Key)
	log.Printf("Using transcription job name: %s", jobName)

	s3Client := s3.NewFromConfig(awsCfg)
	exists, err := checkS3ObjectExists(ctx, s3Client, cfgApp.BucketName, s3Key)
	if err != nil {
		return fmt.Errorf("check S3 object existence: %w", err)
	}
	if exists {
		log.Printf("File already exists in S3; skipping upload.")
	} else {
		log.Printf("Uploading file to S3...")
		if err := uploadFileToS3(ctx, s3Client, cfgApp.BucketName, s3Key, cfgApp.InputFilePath); err != nil {
			return fmt.Errorf("upload file to S3: %w", err)
		}
		log.Printf("Upload completed.")
	}

	transcribeClient := transcribe.NewFromConfig(awsCfg)
	if err := ensureTranscriptionJob(ctx, transcribeClient, jobName, cfgApp.BucketName, s3Key); err != nil {
		return fmt.Errorf("ensuring transcription job: %w", err)
	}
	log.Printf("Transcription job completed.")

	// By default, Transcribe names the output file "<jobName>.json" in the provided bucket.
	transcriptionKey := fmt.Sprintf("%s.json", jobName)
	log.Printf("Retrieving transcription result from S3: %s", transcriptionKey)
	transcript, err := getTranscriptFromS3(ctx, s3Client, cfgApp.BucketName, transcriptionKey)
	if err != nil {
		return fmt.Errorf("retrieve transcription result: %w", err)
	}

	if err := os.WriteFile(cfgApp.OutputFilePath, []byte(transcript), 0o644); err != nil {
		return fmt.Errorf("write transcript to file: %w", err)
	}
	log.Printf("Transcript saved to %q", cfgApp.OutputFilePath)
	return nil
}

// newConfig parses flags and performs initial validation.
func newConfig(args []string) (*AppConfig, error) {
	fs := flag.NewFlagSet("", flag.ExitOnError)

	inputFilePath := fs.String("f", "", "Path to input m4a audio file")
	outputFilePath := fs.String("o", "", "Path to output text file")
	bucketName := fs.String("b", "", "S3 bucket name")
	region := fs.String("r", "us-east-1", "AWS region (default: us-east-1)")
	version := fs.Bool("v", false, "Print version and exit")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	if *version {
		printVersion()
	}

	// fail fast
	if *inputFilePath == "" || *outputFilePath == "" || *bucketName == "" {
		fs.Usage()
		os.Exit(1)
	}

	if !strings.HasSuffix(strings.ToLower(*inputFilePath), ".m4a") {
		return nil, fmt.Errorf("input file must be an m4a file")
	}

	isValid, err := validateBucketName(*bucketName)
	if err != nil {
		return nil, fmt.Errorf("invalid bucket name %q: %w", *bucketName, err)
	}

	if !isValid {
		return nil, fmt.Errorf("invalid bucket name %q", *bucketName)
	}

	return &AppConfig{
		InputFilePath:  *inputFilePath,
		OutputFilePath: *outputFilePath,
		BucketName:     *bucketName,
		Region:         *region,
	}, nil
}

func printVersion() {
	fmt.Printf("Version: %s\n", version)
	fmt.Printf("Commit: %s\n", commit[:7])
	os.Exit(0)
}

func validateBucketName(bucket string) (bool, error) {
	re, err := regexp.Compile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	if err != nil {
		return false, fmt.Errorf("compile regex: %w", err)
	}
	return re.MatchString(bucket), nil
}

// checkS3ObjectExists uses HeadObject to determine if the object already exists.
func checkS3ObjectExists(ctx context.Context, client *s3.Client, bucket, key string) (bool, error) {
	_, err := client.HeadObject(ctx, &s3.HeadObjectInput{
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

// uploadFileToS3 uploads the given file to the specified bucket and key.
func uploadFileToS3(ctx context.Context, client *s3.Client, bucket, key, filePath string) error {
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &bucket,
		Key:    &key,
		Body:   f,
	})
	return err
}

// ensureTranscriptionJob checks for an existing transcription job and starts one if not found.
func ensureTranscriptionJob(ctx context.Context, client *transcribe.Client, jobName, bucket, mediaKey string) error {
	jobExists, jobStatus, err := getTranscriptionJobStatus(ctx, client, jobName)
	if err != nil {
		return fmt.Errorf("checking transcription job status: %w", err)
	}
	if jobExists {
		log.Printf("Transcription job %q already exists with status: %s", jobName, jobStatus)
		return nil
	}

	log.Printf("Starting transcription job...")
	if err := startTranscriptionJob(ctx, client, jobName, bucket, mediaKey); err != nil {
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
			_, jobStatus, err := getTranscriptionJobStatus(ctx, client, jobName)
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
func getTranscriptionJobStatus(ctx context.Context, client *transcribe.Client, jobName string) (bool, string, error) {
	out, err := client.GetTranscriptionJob(ctx, &transcribe.GetTranscriptionJobInput{
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
func startTranscriptionJob(ctx context.Context, client *transcribe.Client, jobName, bucket, mediaKey string) error {
	mediaURI := fmt.Sprintf("s3://%s/%s", bucket, mediaKey)
	input := &transcribe.StartTranscriptionJobInput{
		TranscriptionJobName: &jobName,
		LanguageCode:         types.LanguageCodeEnUs, // adjust if needed
		MediaFormat:          "m4a",
		Media: &types.Media{
			MediaFileUri: &mediaURI,
		},
		OutputBucketName: &bucket,
	}
	_, err := client.StartTranscriptionJob(ctx, input)
	return err
}

// isNotFoundError determines if an error from AWS indicates a “not found” condition.
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

// getTranscriptFromS3 downloads the transcription result JSON from S3 and extracts the transcript text.
func getTranscriptFromS3(ctx context.Context, client *s3.Client, bucket, key string) (string, error) {
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return "", err
	}
	defer out.Body.Close()

	var result TranscriptionResult
	decoder := json.NewDecoder(out.Body)
	if err := decoder.Decode(&result); err != nil {
		return "", err
	}
	if len(result.Results.Transcripts) == 0 {
		return "", fmt.Errorf("no transcript found in result")
	}
	return result.Results.Transcripts[0].Transcript, nil
}
