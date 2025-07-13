package config

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/embano1/transcribe-meetings/internal/types"
)

// build info set by goreleaser
var (
	Version = "unknown"
	Commit  = "unknown"
)

// New parses flags and performs initial validation.
func New(args []string) (*types.AppConfig, error) {
	fs := flag.NewFlagSet("", flag.ExitOnError)

	inputFilePath := fs.String("f", "", "Path to input m4a audio file")
	outputFilePath := fs.String("o", "", "Path to output text file")
	bucketName := fs.String("b", "", "S3 bucket name")
	region := fs.String("r", "us-east-1", "AWS region")
	languageCode := fs.String("l", "en-US", "Language code for transcription")
	speakerDiarization := fs.Bool("d", false, "Enable speaker diarization")
	maxSpeakers := fs.Int("m", 10, "Maximum number of speakers for diarization")
	version := fs.Bool("v", false, "Print version and exit")

	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parsing flags: %w", err)
	}

	if *version {
		PrintVersion()
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

	return &types.AppConfig{
		InputFilePath:      *inputFilePath,
		OutputFilePath:     *outputFilePath,
		BucketName:         *bucketName,
		Region:             *region,
		LanguageCode:       *languageCode,
		SpeakerDiarization: *speakerDiarization,
		MaxSpeakers:        *maxSpeakers,
	}, nil
}

// PrintVersion prints version information and exits
func PrintVersion() {
	fmt.Printf("Version: %s\n", Version)
	if len(Commit) >= 7 {
		fmt.Printf("Commit: %s\n", Commit[:7])
	} else {
		fmt.Printf("Commit: %s\n", Commit)
	}
	os.Exit(0)
}

// validateBucketName validates an S3 bucket name
func validateBucketName(bucket string) (bool, error) {
	re, err := regexp.Compile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	if err != nil {
		return false, fmt.Errorf("compile regex: %w", err)
	}
	return re.MatchString(bucket), nil
}
