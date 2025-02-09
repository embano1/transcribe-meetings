# Transcribe Meetings

[![Go Version](https://img.shields.io/badge/go-1.23%2B-blue)](https://golang.org/) [![License:
MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Transcribe Meetings is a Go command-line application that transcribes meeting recordings using Amazon S3 and Amazon
Transcribe. It uploads your audio file (in m4a format) to S3, triggers an Amazon Transcribe job (if one isn’t already
running), and retrieves the transcription result—saving it locally.

## Features

- **Idempotent Uploads:**  
  Computes a file hash to generate a unique S3 key and transcription job name, ensuring that the same file isn’t
  uploaded or re-transcribed more than once.

- **Amazon Transcribe Integration:**  
  Automatically starts (or reuses) a transcription job and polls for job completion.

- **Result Retrieval:**  
  Once the job completes, retrieves the transcript JSON from S3 and extracts the transcript text to a local file.

## Prerequisites

**AWS Credentials:**  
**Ensure** that your AWS credentials are configured. You can use environment variables, an AWS credentials file, or
another supported authentication method.

**AWS Resources:**  
  - An S3 bucket where the meeting files and transcription results will be stored.  
  - Amazon Transcribe must be enabled in your AWS account.

## Installation

### Releases

Grab the latest [release](https://github.com/embano1/transcribe-meetings/releases) or use the GHCR container
[image](https://github.com/users/embano1/packages/container/package/transcribe-meetings):

```bash
docker pull ghcr.io/embano1/transcribe-meetings
```

### Build From Source

Requires Go toolchain, tested with Go `1.23`.

```bash
git clone https://github.com/embano1/transcribe-meetings.git
cd transcribe-meetings
go build -o transcribe-meetings main.go
```

## Usage

> [!NOTE] Before running the application, make sure you have configured your AWS credentials accordingly to use Amazon
> S3 and Transcribe.

Once installed, you can run the application with the following flags:

- `-f` – Path to the input `.m4a` audio file  
- `-o` – Path for the output text file containing the transcript  
- `-b` – S3 bucket name (must exist)
- `-r` – (Optional) AWS region (defaults to `us-east-1`)
- `-v` - Print version information

Example:

```bash
./transcribe-meetings -f meeting.m4a -o transcript.txt -b your-existing-s3-bucket -r eu-central-1
```

Example Docker:

```bash
docker run -e AWS_SECRET_ACCESS_KEY=${AWS_SECRET_ACCESS_KEY} -e AWS_ACCESS_KEY_ID=${AWS_ACCESS_KEY_ID} -v $PWD:/app \
ghcr.io/embano1/transcribe-meetings  -b <bucket name> -f /app/<location of m4a file> -o /app/output.txt
```

> [!NOTE] 
> Make sure the container has access to the local filesystem (mount) and replace the file and folder names based on your environment

### How It Works

1. **File Hashing:**  
   The application computes a SHA-256 hash of your input file and uses a prefix of that hash along with the filename to
   generate a unique S3 key and transcription job name.

2. **S3 Upload:**  
   If the file isn’t already in your specified S3 bucket, the app uploads it.

3. **Transcription Job:**  
   It checks if a transcription job for that file already exists; if not, it starts a new job using Amazon Transcribe
   and polls until the job is complete.

4. **Result Retrieval:**  
   The transcript (stored as a JSON file in S3) is downloaded, parsed, and saved to your local output file.

## AWS SDK Logging Warning

If you see a warning like:

```
SDK 2025/02/07 11:49:20 WARN Response has no supported checksum. Not validating response payload.
```

this is being intentionally by the AWS SDK used. No action is required.

## Contributing

Contributions are welcome! Please feel free to open issues or submit pull requests if you have improvements, bug fixes,
or new features.

## License

This project is licensed under the [MIT License](LICENSE).
