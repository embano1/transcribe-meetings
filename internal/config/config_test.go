package config

import (
	"errors"
	"strings"
	"testing"
)

func TestNewParsesConfigWithDefaults(t *testing.T) {
	t.Parallel()

	result, err := New([]string{"-f", "meeting.m4a", "-o", "transcript.txt", "-b", "demo-bucket"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if result.ShowVersion {
		t.Fatalf("expected config result, got version result")
	}

	got := result.Config
	if got.InputFilePath != "meeting.m4a" {
		t.Fatalf("InputFilePath = %q", got.InputFilePath)
	}
	if got.OutputFilePath != "transcript.txt" {
		t.Fatalf("OutputFilePath = %q", got.OutputFilePath)
	}
	if got.BucketName != "demo-bucket" {
		t.Fatalf("BucketName = %q", got.BucketName)
	}
	if got.Region != "us-east-1" {
		t.Fatalf("Region = %q", got.Region)
	}
	if got.LanguageCode != "en-US" {
		t.Fatalf("LanguageCode = %q", got.LanguageCode)
	}
	if got.SpeakerDiarization {
		t.Fatalf("SpeakerDiarization = true")
	}
	if got.MaxSpeakers != 10 {
		t.Fatalf("MaxSpeakers = %d", got.MaxSpeakers)
	}
	if got.Force {
		t.Fatalf("Force = true")
	}
}

func TestNewParsesOptionalFlags(t *testing.T) {
	t.Parallel()

	result, err := New([]string{
		"-f", "meeting.m4a",
		"-o", "transcript.txt",
		"-b", "demo-bucket",
		"-r", "eu-central-1",
		"-l", "de-DE",
		"-d",
		"-m", "5",
		"-force",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	got := result.Config
	if got.Region != "eu-central-1" {
		t.Fatalf("Region = %q", got.Region)
	}
	if got.LanguageCode != "de-DE" {
		t.Fatalf("LanguageCode = %q", got.LanguageCode)
	}
	if !got.SpeakerDiarization {
		t.Fatalf("SpeakerDiarization = false")
	}
	if got.MaxSpeakers != 5 {
		t.Fatalf("MaxSpeakers = %d", got.MaxSpeakers)
	}
	if !got.Force {
		t.Fatalf("Force = false")
	}
}

func TestNewReturnsUsageErrorWhenRequiredFlagsMissing(t *testing.T) {
	t.Parallel()

	_, err := New([]string{"-f", "meeting.m4a"})
	if err == nil {
		t.Fatal("expected error")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T", err)
	}
	if usageErr.ExitCode != 1 {
		t.Fatalf("ExitCode = %d", usageErr.ExitCode)
	}
	if !strings.Contains(usageErr.Usage, "-f") || !strings.Contains(usageErr.Usage, "-o") || !strings.Contains(usageErr.Usage, "-b") {
		t.Fatalf("usage text missing expected flags: %q", usageErr.Usage)
	}
}

func TestNewReturnsUsageErrorForParseFailure(t *testing.T) {
	t.Parallel()

	_, err := New([]string{"-unknown"})
	if err == nil {
		t.Fatal("expected error")
	}

	var usageErr *UsageError
	if !errors.As(err, &usageErr) {
		t.Fatalf("expected UsageError, got %T", err)
	}
	if usageErr.ExitCode != 2 {
		t.Fatalf("ExitCode = %d", usageErr.ExitCode)
	}
	if !strings.Contains(usageErr.Error(), "parsing flags") {
		t.Fatalf("unexpected error: %v", usageErr)
	}
}

func TestNewRejectsInvalidInputExtension(t *testing.T) {
	t.Parallel()

	_, err := New([]string{"-f", "meeting.mp3", "-o", "transcript.txt", "-b", "demo-bucket"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "input file must be an m4a file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRejectsInvalidBucketName(t *testing.T) {
	t.Parallel()

	_, err := New([]string{"-f", "meeting.m4a", "-o", "transcript.txt", "-b", "Invalid_Bucket"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid bucket name") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewReturnsVersionMode(t *testing.T) {
	t.Parallel()

	result, err := New([]string{"-v"})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if !result.ShowVersion {
		t.Fatal("expected ShowVersion to be true")
	}
	if result.Config != nil {
		t.Fatalf("expected Config to be nil, got %+v", result.Config)
	}
}

func TestVersionInfo(t *testing.T) {
	t.Parallel()

	originalVersion := Version
	originalCommit := Commit
	Version = "1.2.3"
	Commit = "abcdef123456"
	t.Cleanup(func() {
		Version = originalVersion
		Commit = originalCommit
	})

	got := VersionInfo()
	want := "Version: 1.2.3\nCommit: abcdef1\n"
	if got != want {
		t.Fatalf("VersionInfo() = %q, want %q", got, want)
	}
}
