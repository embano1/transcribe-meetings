// main.go
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/transcribe"

	"github.com/embano1/transcribe-meetings/internal/aws"
	appConfig "github.com/embano1/transcribe-meetings/internal/config"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		log.Fatalf("Could not transcribe audio: %v", err)
	}
}

func run(ctx context.Context, args []string) error {
	cfgApp, err := appConfig.New(args)
	if err != nil {
		return err
	}

	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(cfgApp.Region))
	if err != nil {
		return fmt.Errorf("load AWS SDK config: %w", err)
	}

	// fail fast if the client is not authorized
	s3Client := s3.NewFromConfig(awsCfg)
	s3Service := aws.NewS3Service(s3Client)
	err = s3Service.HeadBucket(ctx, cfgApp.BucketName)
	if err != nil {
		return fmt.Errorf("verify bucket %q exists: %w", cfgApp.BucketName, err)
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

	exists, err := s3Service.CheckObjectExists(ctx, cfgApp.BucketName, s3Key)
	if err != nil {
		return fmt.Errorf("check S3 object existence: %w", err)
	}
	if exists {
		log.Printf("File already exists in S3; skipping upload.")
	} else {
		log.Printf("Uploading file to S3...")
		if err := s3Service.UploadFile(ctx, cfgApp.BucketName, s3Key, cfgApp.InputFilePath); err != nil {
			return fmt.Errorf("upload file to S3: %w", err)
		}
		log.Printf("Upload completed.")
	}

	transcribeClient := transcribe.NewFromConfig(awsCfg)
	transcribeService := aws.NewTranscribeService(transcribeClient)
	if err := transcribeService.EnsureTranscriptionJob(ctx, jobName, cfgApp.BucketName, s3Key, cfgApp); err != nil {
		return fmt.Errorf("ensuring transcription job: %w", err)
	}
	log.Printf("Transcription job completed.")

	// By default, Transcribe names the output file "<jobName>.json" in the provided bucket.
	transcriptionKey := fmt.Sprintf("%s.json", jobName)
	log.Printf("Retrieving transcription result from S3: %s", transcriptionKey)
	transcript, err := s3Service.GetTranscriptFromS3(ctx, cfgApp.BucketName, transcriptionKey, cfgApp)
	if err != nil {
		return fmt.Errorf("retrieve transcription result: %w", err)
	}

	if err := os.WriteFile(cfgApp.OutputFilePath, []byte(transcript), 0o644); err != nil {
		return fmt.Errorf("write transcript to file: %w", err)
	}
	log.Printf("Transcript saved to %q", cfgApp.OutputFilePath)
	return nil
}
