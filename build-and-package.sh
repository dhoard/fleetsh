#!/bin/bash

#
# Copyright (c) 2026-present Douglas Hoard
#
# Permission is hereby granted, free of charge, to any person obtaining a copy
# of this software and associated documentation files (the "Software"), to deal
# in the Software without restriction, including without limitation the rights
# to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
# copies of the Software, and to permit persons to whom the Software is
# furnished to do so, subject to the following conditions:
#
# The above copyright notice and this permission notice shall be included in all
# copies or substantial portions of the Software.
#

set -euo pipefail

scriptdir="$(cd "$(dirname "$0")" && pwd)"
srcdir="${scriptdir}/src"

# --- check tools ---

min_go_version=$(sed -n 's/^go \([0-9.]*\)/\1/p' "${srcdir}/go.mod" | head -1)
if [ -z "${min_go_version}" ]; then
    echo "build: could not parse go version from src/go.mod"
    exit 1
fi

installed_go_ver=$(go env GOVERSION 2>/dev/null | sed 's/^go//' || echo "")
if [ -z "${installed_go_ver}" ]; then
    echo "build: go is required (not found)"
    exit 1
fi

lowest_go=$(printf "%s\n%s\n" "$installed_go_ver" "$min_go_version" | sort -V | head -1)
if [ "$lowest_go" != "$min_go_version" ]; then
    echo "build: go ${min_go_version} or newer is required (found: go${installed_go_ver})"
    exit 1
fi

if ! command -v goreleaser &>/dev/null; then
    echo "build: goreleaser is required (not found)"
    exit 1
fi

# --- version ---

version=$(grep -oP 'var version = "\K[^"]+' "${srcdir}/internal/cli/version.go")
if [ -z "${version}" ]; then
    echo "build: could not parse version from internal/cli/version.go"
    exit 1
fi
export FLEETSH_VERSION="${version}"

# --- test ---

echo "==> Testing..."
cd "${srcdir}"
go test ./...

# --- vet ---

echo "==> Vetting..."
go vet ./...

# --- build ---

cd "${scriptdir}"

echo "==> Building release (all targets)..."
goreleaser release --clean --snapshot --skip=publish

# --- package ---

rm -rf "${scriptdir}/package"
mkdir -p "${scriptdir}/package"

for archive in "${scriptdir}/dist"/*.tar.gz; do
    if [ -f "$archive" ]; then
        filename=$(basename "$archive")
        cp "$archive" "${scriptdir}/package/"
        (cd "${scriptdir}/package" && sha256sum "$filename" > "${filename}.sha256")
    fi
done

echo ""
echo "Packaged:"
ls -lah "${scriptdir}/package/"
