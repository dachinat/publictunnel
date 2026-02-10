#!/usr/bin/env bash
set -e

# Output directory for builds
OUTPUT_DIR="build"
mkdir -p "$OUTPUT_DIR"

# Source file
SRC="cmd/client/main.go"
BINARY_NAME="publictunnel"

# Function to build and zip
build_and_zip() {
  local os=$1
  local arch=$2
  local ext=$3

  local out_name="${BINARY_NAME}_${os}_${arch}${ext}"
  echo "Building $out_name..."
  GOOS="$os" GOARCH="$arch" go build -o "$OUTPUT_DIR/$out_name" "$SRC"

  echo "Zipping $out_name..."
  (cd "$OUTPUT_DIR" && zip "${out_name}.zip" "$out_name")
  rm "$OUTPUT_DIR/$out_name" # Remove the unzipped binary if you only want the zip
}

# Mac OS
build_and_zip darwin amd64 ""
build_and_zip darwin arm64 ""

# Linux
build_and_zip linux amd64 ""

# Windows
build_and_zip windows amd64 ".exe"

echo "All builds and zips completed. Check the '$OUTPUT_DIR' directory."
