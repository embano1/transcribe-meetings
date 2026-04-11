package config

import (
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/embano1/transcribe-meetings/internal/types"
)

// build info set by goreleaser
var (
	Version = "unknown"
	Commit  = "unknown"
)

// ParseResult contains the outcome of parsing command-line flags.
type ParseResult struct {
	Config      *types.AppConfig
	ShowVersion bool
}

// UsageError indicates CLI usage should be shown instead of treating the failure as a runtime error.
type UsageError struct {
	Err      error
	Usage    string
	ExitCode int
}

func (e *UsageError) Error() string {
	return e.Err.Error()
}

func (e *UsageError) Unwrap() error {
	return e.Err
}

// New parses flags and performs initial validation.
func New(args []string) (*ParseResult, error) {
	fs, opts := newFlagSet(io.Discard)

	if err := fs.Parse(args); err != nil {
		return nil, &UsageError{
			Err:      fmt.Errorf("parsing flags: %w", err),
			Usage:    usageText(),
			ExitCode: 2,
		}
	}

	if opts.version {
		return &ParseResult{ShowVersion: true}, nil
	}

	// fail fast
	if opts.inputFilePath == "" || opts.outputFilePath == "" || opts.bucketName == "" {
		return nil, &UsageError{
			Err:      fmt.Errorf("required flags -f, -o, and -b must be set"),
			Usage:    usageText(),
			ExitCode: 1,
		}
	}

	if !strings.HasSuffix(strings.ToLower(opts.inputFilePath), ".m4a") {
		return nil, fmt.Errorf("input file must be an m4a file")
	}

	isValid, err := validateBucketName(opts.bucketName)
	if err != nil {
		return nil, fmt.Errorf("invalid bucket name %q: %w", opts.bucketName, err)
	}

	if !isValid {
		return nil, fmt.Errorf("invalid bucket name %q", opts.bucketName)
	}

	return &ParseResult{
		Config: &types.AppConfig{
			InputFilePath:      opts.inputFilePath,
			OutputFilePath:     opts.outputFilePath,
			BucketName:         opts.bucketName,
			Region:             opts.region,
			LanguageCode:       opts.languageCode,
			SpeakerDiarization: opts.speakerDiarization,
			MaxSpeakers:        opts.maxSpeakers,
			Force:              opts.force,
		},
	}, nil
}

type flagOptions struct {
	inputFilePath      string
	outputFilePath     string
	bucketName         string
	region             string
	languageCode       string
	speakerDiarization bool
	maxSpeakers        int
	force              bool
	version            bool
}

func newFlagSet(output io.Writer) (*flag.FlagSet, *flagOptions) {
	fs := flag.NewFlagSet("", flag.ContinueOnError)
	fs.SetOutput(output)

	opts := &flagOptions{}
	fs.StringVar(&opts.inputFilePath, "f", "", "Path to input m4a audio file")
	fs.StringVar(&opts.outputFilePath, "o", "", "Path to output text file")
	fs.StringVar(&opts.bucketName, "b", "", "S3 bucket name")
	fs.StringVar(&opts.region, "r", "us-east-1", "AWS region")
	fs.StringVar(&opts.languageCode, "l", "en-US", "Language code for transcription")
	fs.BoolVar(&opts.speakerDiarization, "d", false, "Enable speaker diarization")
	fs.IntVar(&opts.maxSpeakers, "m", 10, "Maximum number of speakers for diarization")
	fs.BoolVar(&opts.force, "force", false, "Force re-transcription even if job already exists")
	fs.BoolVar(&opts.version, "v", false, "Print version and exit")

	return fs, opts
}

// VersionInfo returns the version output shown by the CLI.
func VersionInfo() string {
	var builder strings.Builder

	fmt.Fprintf(&builder, "Version: %s\n", Version)
	if len(Commit) >= 7 {
		fmt.Fprintf(&builder, "Commit: %s\n", Commit[:7])
	} else {
		fmt.Fprintf(&builder, "Commit: %s\n", Commit)
	}
	return builder.String()
}

func usageText() string {
	var builder strings.Builder

	fs, _ := newFlagSet(&builder)
	fs.Usage()
	return builder.String()
}

// validateBucketName validates an S3 bucket name
func validateBucketName(bucket string) (bool, error) {
	re, err := regexp.Compile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	if err != nil {
		return false, fmt.Errorf("compile regex: %w", err)
	}
	return re.MatchString(bucket), nil
}
