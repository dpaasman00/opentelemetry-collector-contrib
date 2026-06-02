// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewInstaller(t *testing.T) {
	testCases := []struct {
		name      string
		format    Format
		expectErr string
	}{
		{
			name:   "no archive returns raw installer",
			format: FormatNone,
		},
		{
			name:   "tar.gz returns tar.gz installer",
			format: FormatTarGzip,
		},
		{
			name:      "unsupported archive format",
			format:    Format("zip"),
			expectErr: "unsupported archive format",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			installer, err := NewInstaller(tc.format)
			if tc.expectErr != "" {
				require.ErrorContains(t, err, tc.expectErr)
				assert.Nil(t, installer)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, installer)
		})
	}
}

func TestRawInstaller_Install(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "otelcol-contrib")
	contents := []byte("raw collector binary")

	installer, err := NewInstaller(FormatNone)
	require.NoError(t, err)

	require.NoError(t, installer.Install(t.Context(), contents, "otelcol-contrib", destination))

	written, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, contents, written)
}

func TestTarGzipInstaller_Install(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "otelcol-contrib")
		contents := []byte("collector binary inside a tarball")
		archive := createTarGzArchive(t, map[string][]byte{
			"README.md":       []byte("not the binary"),
			"otelcol-contrib": contents,
		})

		installer, err := NewInstaller(FormatTarGzip)
		require.NoError(t, err)

		require.NoError(t, installer.Install(t.Context(), archive, "otelcol-contrib", destination))

		written, err := os.ReadFile(destination)
		require.NoError(t, err)
		assert.Equal(t, contents, written)
	})

	t.Run("missing binary name", func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "otelcol-contrib")
		archive := createTarGzArchive(t, map[string][]byte{"otelcol-contrib": []byte("binary")})

		err := tarGzipInstaller{}.Install(t.Context(), archive, "", destination)
		require.ErrorContains(t, err, "agent binary name is required")
	})

	t.Run("binary not in archive", func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "otelcol-contrib")
		archive := createTarGzArchive(t, map[string][]byte{"some-other-file": []byte("binary")})

		err := tarGzipInstaller{}.Install(t.Context(), archive, "otelcol-contrib", destination)
		require.ErrorContains(t, err, `read tarball looking for "otelcol-contrib"`)
	})

	t.Run("not a gzip archive", func(t *testing.T) {
		destination := filepath.Join(t.TempDir(), "otelcol-contrib")

		err := tarGzipInstaller{}.Install(t.Context(), []byte("not gzip data"), "otelcol-contrib", destination)
		require.ErrorContains(t, err, "create gzip reader")
	})
}

// createTarGzArchive builds an in-memory gzipped tarball containing the given
// files (name -> contents).
func createTarGzArchive(t *testing.T, files map[string][]byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)

	for name, contents := range files {
		require.NoError(t, tarWriter.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(contents)),
		}))
		_, err := tarWriter.Write(contents)
		require.NoError(t, err)
	}

	require.NoError(t, tarWriter.Close())
	require.NoError(t, gzipWriter.Close())

	return buf.Bytes()
}
