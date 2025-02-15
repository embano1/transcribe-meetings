#!/bin/bash

set -o pipefail

# Check if git working directory is clean
if ! git diff --quiet; then
    echo "Error: Git working directory is not clean"
    exit 1
fi

# export KO_DOCKER_REPO=ghcr.io/gh-username
if [ -z "${KO_DOCKER_REPO}" ]; then
    echo "Error: KO_DOCKER_REPO environment variable is not set"
    exit 1
fi

if [ -z "${GITHUB_TOKEN}" ]; then
    echo "Error: GITHUB_TOKEN environment variable is not set"
    exit 1
fi

# check if GITHUB_TOKEN starts with op://
if ! [[ $GITHUB_TOKEN == op://* ]]; then
    echo "Error: GITHUB_TOKEN environment variable must point to valid 1Password secrets reference"
    exit 1
fi

# uses 1Password to inject passwords
if ! command -v op &>/dev/null; then
    echo "Error: 1Password CLI (op) is not installed"
    exit 1
fi

if ! command -v goreleaser &>/dev/null; then
    echo "Error: goreleaser is not installed"
    exit 1
fi

if ! command -v ko &>/dev/null; then
    echo "Error: ko is not installed"
    exit 1
fi

if [ -z "${RELEASE}" ]; then
    echo "Error: RELEASE environment variable is not set"
    exit 1
fi

if [ -z "${RELEASE}" ]; then
    echo "Error: RELEASE environment variable is not set"
    exit 1
fi

git tag -a -s -m "Release ${RELEASE}" ${RELEASE}

# export GIT_COMMIT=$(git rev-parse HEAD)
if [ -z "${GIT_COMMIT}" ]; then
    echo "Error: GIT_COMMIT environment variable is not set"
    exit 1
fi

# verify GIT_COMMIT points to RELEASE
if [ "$(git rev-list -n 1 "$RELEASE" 2>/dev/null)" != "${GIT_COMMIT}" ]; then
    echo "Error: GIT_COMMIT ${GIT_COMMIT} does not match commit hash of tag ${RELEASE}"
    exit 1
fi

echo "Creating Github release with the following settings"
echo "TAG: ${RELEASE}"
echo "COMMIT: ${GIT_COMMIT}"
echo "Docker Repo: ${KO_DOCKER_REPO}"
echo "Waiting 10s before continuing..."
sleep 10

# note: goreleaser expects the commit and tag to be pushed
git push upstream main --tags

# build artifacts and publish release
op run -- goreleaser release --clean

# build and publish container image
op run -- ko build --platform=all -B --tags $RELEASE .