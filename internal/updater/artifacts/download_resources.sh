#!/bin/sh

# This file is part of arduino-flasher-cli.
#
# SPDX-FileCopyrightText: Arduino s.r.l. and/or its affiliated companies
# SPDX-License-Identifier: GPL-3.0-or-later

set -e

BASE_DIR="$(cd "$(dirname "$0")" && pwd)"

REPO="arduino/qdl-packing"
TAG="v2.4-26"

# Work with paths relative to this script's directory: on Windows an absolute
# path contains a drive letter (e.g. D:\a\...) and tar interprets everything
# before the colon as a remote host, failing with "Cannot connect to D".
cd "$BASE_DIR"

# Remove existing resource directories if they exist
rm -rf resources_*

# Download and extract Linux 64-bit
mkdir -p resources_linux_amd64
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_Linux_64bit.tar.gz" --dir resources_linux_amd64
tar -xzf resources_linux_amd64/qdl_*_Linux_64bit.tar.gz -C resources_linux_amd64 qdl

# Download and extract Linux arm64
mkdir -p resources_linux_arm64
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_Linux_ARM64.tar.gz" --dir resources_linux_arm64
tar -xzf resources_linux_arm64/qdl_*_Linux_ARM64.tar.gz -C resources_linux_arm64 qdl

# Download and extract Windows 32-bit
mkdir -p resources_windows_amd64
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_Windows_32bit.tar.gz" --dir resources_windows_amd64
tar -xzf resources_windows_amd64/qdl_*_Windows_32bit.tar.gz -C resources_windows_amd64 qdl.exe

# Download and extract macOS 64-bit
mkdir -p resources_darwin_amd64
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_macOS_64bit.tar.gz" --dir resources_darwin_amd64
tar -xzf resources_darwin_amd64/qdl_*_macOS_64bit.tar.gz -C resources_darwin_amd64 qdl

# Download and extract macOS arm64
mkdir -p resources_darwin_arm64
gh release download "$TAG" --repo "$REPO" --pattern "qdl_*_macOS_arm64.tar.gz" --dir resources_darwin_arm64
tar -xzf resources_darwin_arm64/qdl_*_macOS_arm64.tar.gz -C resources_darwin_arm64 qdl

# cleanup archive files
rm -rf resources_*/*.tar.gz

echo "All files downloaded, extracted, and organized successfully."
