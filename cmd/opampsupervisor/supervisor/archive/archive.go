// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package archive installs collector executable updates from downloaded
// packages, dispatching on the archive format.
package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
)

// maxAgentBytes is the maximum size of an agent binary the supervisor will write
// to disk during an install. It guards against unbounded writes.
const maxAgentBytes = 1 << 30 // 1 GiB

// Format is the format of the package downloaded by the supervisor.
type Format string

const (
	// FormatNone treats the downloaded package as a raw collector binary with no
	// archive wrapping.
	FormatNone Format = ""
	// FormatTarGzip treats the downloaded package as a gzipped tarball containing
	// the collector binary.
	FormatTarGzip Format = "tar.gz"
)

// Installer installs the agent binary from a downloaded package.
type Installer interface {
	// Install writes the agent binary contained in pkg to destination. binaryName
	// identifies which file inside the package is the agent binary for formats that
	// bundle multiple files; it is ignored for raw binaries.
	Install(ctx context.Context, pkg []byte, binaryName, destination string) error
}

// NewInstaller returns the Installer for the given archive format, or an error if
// the format is not supported.
func NewInstaller(format Format) (Installer, error) {
	switch format {
	case FormatNone:
		return rawInstaller{}, nil
	case FormatTarGzip:
		return tarGzipInstaller{}, nil
	default:
		return nil, fmt.Errorf("unsupported archive format: %q", string(format))
	}
}

var _ Installer = rawInstaller{}

// rawInstaller treats the package bytes as a raw agent binary and writes them
// directly to destination.
type rawInstaller struct{}

func (rawInstaller) Install(_ context.Context, pkg []byte, _, destination string) error {
	return writeBinaryToDestination(bytes.NewReader(pkg), destination)
}

var _ Installer = tarGzipInstaller{}

// tarGzipInstaller extracts the file named binaryName from a gzipped tarball and
// writes it to destination.
type tarGzipInstaller struct{}

func (tarGzipInstaller) Install(_ context.Context, pkg []byte, binaryName, destination string) error {
	if binaryName == "" {
		return errors.New("agent binary name is required for tar.gz archives")
	}

	gzipReader, err := gzip.NewReader(bytes.NewReader(pkg))
	if err != nil {
		return fmt.Errorf("create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err != nil {
			return fmt.Errorf("read tarball looking for %q: %w", binaryName, err)
		}
		if header.Name == binaryName {
			break
		}
	}

	if err := writeBinaryToDestination(tarReader, destination); err != nil {
		return fmt.Errorf("write binary to destination: %w", err)
	}

	return nil
}

// writeBinaryToDestination writes binary to destination, creating or truncating
// the file with executable permissions. At most maxAgentBytes are written.
func writeBinaryToDestination(binary io.Reader, destination string) error {
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return fmt.Errorf("open destination file: %w", err)
	}
	defer f.Close()

	if _, err := io.CopyN(f, binary, maxAgentBytes); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("write binary to destination: %w", err)
	}

	return nil
}
