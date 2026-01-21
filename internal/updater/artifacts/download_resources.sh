#!/bin/sh

set -e

BASE_DIR="$(cd "$(dirname "$0")" && pwd)"

REPO="arduino/qdl-packing"
TAG="v2.4-26"

# Remove existing resource directories if they exist
rm -rf $BASE_DIR/resources_*

# Download and extract Linux 64-bit
mkdir -p "$BASE_DIR/resources_linux_amd64"
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_Linux_64bit.tar.gz" --dir "$BASE_DIR/resources_linux_amd64"
tar -xzf $BASE_DIR/resources_linux_amd64/qdl_*_Linux_64bit.tar.gz -C "$BASE_DIR/resources_linux_amd64" qdl

# Download and extract Linux arm64
mkdir -p "$BASE_DIR/resources_linux_arm64"
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_Linux_ARM64.tar.gz" --dir "$BASE_DIR/resources_linux_arm64"
tar -xzf $BASE_DIR/resources_linux_arm64/qdl_*_Linux_ARM64.tar.gz -C "$BASE_DIR/resources_linux_arm64" qdl

# Download and extract Windows 32-bit
mkdir -p "$BASE_DIR/resources_windows_amd64"
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_Windows_32bit.tar.gz" --dir "$BASE_DIR/resources_windows_amd64"
tar -xzf $BASE_DIR/resources_windows_amd64/qdl_*_Windows_32bit.tar.gz -C "$BASE_DIR/resources_windows_amd64" qdl.exe

# Download and extract macOS 64-bit
mkdir -p "$BASE_DIR/resources_darwin_amd64"
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_macOS_64bit.tar.gz" --dir "$BASE_DIR/resources_darwin_amd64"
tar -xzf $BASE_DIR/resources_darwin_amd64/qdl_*_macOS_64bit.tar.gz -C "$BASE_DIR/resources_darwin_amd64" qdl

# Download and extract macOS arm64
mkdir -p "$BASE_DIR/resources_darwin_arm64"
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_macOS_arm64.tar.gz" --dir "$BASE_DIR/resources_darwin_arm64"
tar -xzf $BASE_DIR/resources_darwin_arm64/qdl_*_macOS_arm64.tar.gz -C "$BASE_DIR/resources_darwin_arm64" qdl

# cleanup archive files
rm -rf $BASE_DIR/resources_*/*.tar.gz

echo "All files downloaded, extracted, and organized successfully."
