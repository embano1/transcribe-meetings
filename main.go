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

// newConfig parses flags and performs initial validation.
func newConfig() *AppConfig {
	inputFilePath := flag.String("f", "", "Path to input m4a audio file")
	outputFilePath := flag.String("o", "", "Path to output text file")
	bucketName := flag.String("b", "", "S3 bucket name")
	region := flag.String("r", "us-east-1", "AWS region (default: us-east-1)")
	flag.Parse()

	if *inputFilePath == "" || *outputFilePath == "" || *bucketName == "" {
		flag.Usage()
		os.Exit(1)
	}

	// Input file must be an m4a file.
	if !strings.HasSuffix(strings.ToLower(*inputFilePath), ".m4a") {
		log.Fatalf("Input file must be an m4a file")
	}

	// Validate bucket name according to AWS naming rules (simple regex).
	if !isValidBucketName(*bucketName) {
		log.Fatalf("Invalid bucket name: %s", *bucketName)
	}

	return &AppConfig{
		InputFilePath:  *inputFilePath,
		OutputFilePath: *outputFilePath,
		BucketName:     *bucketName,
		Region:         *region,
	}
}

// isValidBucketName validates the S3 bucket name (simple version).
func isValidBucketName(bucket string) bool {
	// This regex is a basic check—adjust as necessary.
	re := regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	return re.MatchString(bucket)
}

func main() {
	// Load configuration from flags.
	cfgApp := newConfig()

	// Create a cancellable context that listens for OS interrupts, with a timeout.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()

	// Load AWS SDK configuration.
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfgApp.Region))
	if err != nil {
		log.Fatalf("Unable to load AWS SDK config: %v", err)
	}

	// Ensure the input file exists.
	fileInfo, err := os.Stat(cfgApp.InputFilePath)
	if err != nil {
		log.Fatalf("Error stating input file: %v", err)
	}
	if fileInfo.IsDir() {
		log.Fatalf("Input path is a directory, not a file")
	}

	// Open the file and compute its hash.
	f, err := os.Open(cfgApp.InputFilePath)
	if err != nil {
		log.Fatalf("Failed to open input file: %v", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		log.Fatalf("Failed to compute file hash: %v", err)
	}
	f.Close()

	fileHash := hex.EncodeToString(hasher.Sum(nil))[:16] // using first 16 hex digits

	// Use the file hash and original file name to form an S3 key and job name.
	fileName := filepath.Base(cfgApp.InputFilePath)
	s3Key := fmt.Sprintf("uploads/%s_%s", fileHash, fileName)
	jobName := fmt.Sprintf("transcribe-%s", fileHash)

	log.Printf("Using S3 key: %s", s3Key)
	log.Printf("Using transcription job name: %s", jobName)

	// Create an S3 client.
	s3Client := s3.NewFromConfig(awsCfg)

	// Check if the file is already in S3.
	exists, err := checkS3ObjectExists(ctx, s3Client, cfgApp.BucketName, s3Key)
	if err != nil {
		log.Fatalf("Failed to check S3 object existence: %v", err)
	}
	if exists {
		log.Printf("File already exists in S3; skipping upload.")
	} else {
		log.Printf("Uploading file to S3...")
		if err := uploadFileToS3(ctx, s3Client, cfgApp.BucketName, s3Key, cfgApp.InputFilePath); err != nil {
			log.Fatalf("Failed to upload file to S3: %v", err)
		}
		log.Printf("Upload completed.")
	}

	// Create a Transcribe client.
	transcribeClient := transcribe.NewFromConfig(awsCfg)

	// Ensure that a transcription job is running (or already exists).
	if err := ensureTranscriptionJob(ctx, transcribeClient, jobName, cfgApp.BucketName, s3Key); err != nil {
		log.Fatalf("Error ensuring transcription job: %v", err)
	}

	// The transcription result is stored in S3.
	// By default, Transcribe names the output file "<jobName>.json" in the provided bucket.
	transcriptionKey := fmt.Sprintf("%s.json", jobName)
	log.Printf("Retrieving transcription result from S3: %s", transcriptionKey)
	transcript, err := getTranscriptFromS3(ctx, s3Client, cfgApp.BucketName, transcriptionKey)
	if err != nil {
		log.Fatalf("Failed to retrieve transcription result: %v", err)
	}

	// Write the transcript to the local output file.
	if err := os.WriteFile(cfgApp.OutputFilePath, []byte(transcript), 0o644); err != nil {
		log.Fatalf("Failed to write transcript to file: %v", err)
	}
	log.Printf("Transcript saved to %q", cfgApp.OutputFilePath)
}

// ensureTranscriptionJob checks for an existing transcription job and starts one if not found.
func ensureTranscriptionJob(ctx context.Context, client *transcribe.Client, jobName, bucket, mediaKey string) error {
	jobExists, jobStatus, err := getTranscriptionJobStatus(ctx, client, jobName)
	if err != nil {
		return fmt.Errorf("error checking transcription job status: %w", err)
	}
	if jobExists {
		log.Printf("Transcription job %q already exists with status: %s", jobName, jobStatus)
		return nil
	}

	log.Printf("Starting transcription job...")
	if err := startTranscriptionJob(ctx, client, jobName, bucket, mediaKey); err != nil {
		return fmt.Errorf("failed to start transcription job: %w", err)
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
			log.Fatalf("Interrupted or timed out, exiting.")
		case <-ticker.C:
			_, jobStatus, err := getTranscriptionJobStatus(ctx, client, jobName)
			if err != nil {
				log.Fatalf("Error getting transcription job status: %v", err)
			}
			log.Printf("Job status: %s", jobStatus)
			if jobStatus == string(types.TranscriptionJobStatusCompleted) {
				break PollLoop
			} else if jobStatus == string(types.TranscriptionJobStatusFailed) {
				log.Fatalf("Transcription job failed.")
			}
		}
	}
	log.Printf("Transcription job completed.")
	return nil
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
