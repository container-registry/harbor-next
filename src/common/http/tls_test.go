// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package http

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withCleanSSLCertFileEnv(t *testing.T) {
	t.Helper()
	original, had := os.LookupEnv("SSL_CERT_FILE")
	t.Cleanup(func() {
		if had {
			os.Setenv("SSL_CERT_FILE", original)
		} else {
			os.Unsetenv("SSL_CERT_FILE")
		}
	})
	os.Unsetenv("SSL_CERT_FILE")
}

func TestLoadCustomCACertificatesNoDir(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	loadCustomCACertificates(filepath.Join(t.TempDir(), "does-not-exist"), "/does/not/matter", "/does/not/matter")

	_, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.False(t, ok, "SSL_CERT_FILE should be untouched when the custom cert dir doesn't exist")
}

func TestLoadCustomCACertificatesEmptyDir(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	certDir := t.TempDir()
	loadCustomCACertificates(certDir, "/does/not/matter", "/does/not/matter")

	_, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.False(t, ok, "SSL_CERT_FILE should be untouched when the custom cert dir is empty")
}

func TestLoadCustomCACertificatesIgnoresUnrelatedFiles(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "readme.txt"), []byte("not a cert"), 0o644))
	require.NoError(t, os.Mkdir(filepath.Join(certDir, "subdir.crt"), 0o755)) // a directory, not a file

	systemBundle := filepath.Join(t.TempDir(), "ca-certificates.crt")
	require.NoError(t, os.WriteFile(systemBundle, []byte("system bundle"), 0o644))

	combined := filepath.Join(t.TempDir(), "combined.crt")
	loadCustomCACertificates(certDir, systemBundle, combined)

	_, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.False(t, ok, "SSL_CERT_FILE should be untouched when no .crt/.pem files are present")
	_, err := os.Stat(combined)
	assert.True(t, os.IsNotExist(err), "combined bundle should not be written")
}

func TestLoadCustomCACertificatesMerges(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	customCert := generateSelfSignedCert(t)
	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "internal-ca.crt"), []byte(customCert), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "internal-ca-2.pem"), []byte(customCert), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "ignored.txt"), []byte("not a cert"), 0o644))

	systemBundle := filepath.Join(t.TempDir(), "ca-certificates.crt")
	require.NoError(t, os.WriteFile(systemBundle, []byte("-----BEGIN SYSTEM BUNDLE-----\n"), 0o644))

	combinedDir := t.TempDir()
	combined := filepath.Join(combinedDir, "harbor-custom-ca-certificates.crt")

	loadCustomCACertificates(certDir, systemBundle, combined)

	assert.Equal(t, combined, os.Getenv("SSL_CERT_FILE"))

	got, err := os.ReadFile(combined)
	require.NoError(t, err)
	assert.Contains(t, string(got), "-----BEGIN SYSTEM BUNDLE-----")
	assert.Contains(t, string(got), customCert)
}

func TestLoadCustomCACertificatesRespectsExistingSSLCertFile(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	customCert := generateSelfSignedCert(t)
	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "internal-ca.crt"), []byte(customCert), 0o644))

	overrideBundle := filepath.Join(t.TempDir(), "override-bundle.crt")
	require.NoError(t, os.WriteFile(overrideBundle, []byte("-----BEGIN OVERRIDE BUNDLE-----\n"), 0o644))
	os.Setenv("SSL_CERT_FILE", overrideBundle)

	combined := filepath.Join(t.TempDir(), "combined.crt")
	loadCustomCACertificates(certDir, "/should/not/be/used", combined)

	got, err := os.ReadFile(combined)
	require.NoError(t, err)
	assert.Contains(t, string(got), "-----BEGIN OVERRIDE BUNDLE-----")
}

func TestLoadCustomCACertificatesMissingSystemBundle(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	customCert := generateSelfSignedCert(t)
	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "internal-ca.crt"), []byte(customCert), 0o644))

	combined := filepath.Join(t.TempDir(), "combined.crt")
	loadCustomCACertificates(certDir, filepath.Join(t.TempDir(), "missing-system-bundle.crt"), combined)

	_, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.False(t, ok, "SSL_CERT_FILE should be untouched when the system bundle can't be read")
	_, err := os.Stat(combined)
	assert.True(t, os.IsNotExist(err), "combined bundle should not be written when the system bundle can't be read")
}

func TestLoadCustomCACertificatesSkipsUnreadableFile(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	customCert := generateSelfSignedCert(t)
	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "internal-ca.crt"), []byte(customCert), 0o644))

	unreadable := filepath.Join(certDir, "unreadable.crt")
	require.NoError(t, os.WriteFile(unreadable, []byte(customCert), 0o644))
	require.NoError(t, os.Chmod(unreadable, 0o000))

	systemBundle := filepath.Join(t.TempDir(), "ca-certificates.crt")
	require.NoError(t, os.WriteFile(systemBundle, []byte("-----BEGIN SYSTEM BUNDLE-----\n"), 0o644))

	combined := filepath.Join(t.TempDir(), "combined.crt")
	loadCustomCACertificates(certDir, systemBundle, combined)

	assert.Equal(t, combined, os.Getenv("SSL_CERT_FILE"), "the readable cert should still be loaded")
	got, err := os.ReadFile(combined)
	require.NoError(t, err)
	assert.Contains(t, string(got), customCert)
}

func TestLoadCustomCACertificatesWriteFailure(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	customCert := generateSelfSignedCert(t)
	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "internal-ca.crt"), []byte(customCert), 0o644))

	systemBundle := filepath.Join(t.TempDir(), "ca-certificates.crt")
	require.NoError(t, os.WriteFile(systemBundle, []byte("-----BEGIN SYSTEM BUNDLE-----\n"), 0o644))

	// Parent directory doesn't exist, so the write fails regardless of
	// permissions or whether the test runs as root.
	combined := filepath.Join(t.TempDir(), "does-not-exist", "combined.crt")
	loadCustomCACertificates(certDir, systemBundle, combined)

	_, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.False(t, ok, "SSL_CERT_FILE should be untouched when the combined bundle can't be written")
}

func TestLoadCustomCACertificatesPublicEntrypointIsNoOpByDefault(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	// /harbor_cust_cert won't exist in the test environment, so the public
	// entrypoint should be a no-op -- this just guards against a panic.
	LoadCustomCACertificates()

	_, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.False(t, ok)
}

func withCleanCustomCACertDirEnv(t *testing.T) {
	t.Helper()
	original, had := os.LookupEnv(customCACertDirEnv)
	t.Cleanup(func() {
		if had {
			os.Setenv(customCACertDirEnv, original)
		} else {
			os.Unsetenv(customCACertDirEnv)
		}
	})
	os.Unsetenv(customCACertDirEnv)
}

func TestResolveCustomCACertDirDefault(t *testing.T) {
	withCleanCustomCACertDirEnv(t)
	assert.Equal(t, defaultCustomCACertDir, resolveCustomCACertDir())
}

func TestResolveCustomCACertDirOverride(t *testing.T) {
	withCleanCustomCACertDirEnv(t)
	os.Setenv(customCACertDirEnv, "/tls")
	assert.Equal(t, "/tls", resolveCustomCACertDir())
}

func TestLoadCustomCACertificatesEmptyCertFile(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "empty.crt"), []byte(""), 0o644))

	systemBundle := filepath.Join(t.TempDir(), "ca-certificates.crt")
	require.NoError(t, os.WriteFile(systemBundle, []byte("-----BEGIN SYSTEM BUNDLE-----\n"), 0o644))

	combined := filepath.Join(t.TempDir(), "combined.crt")
	loadCustomCACertificates(certDir, systemBundle, combined)

	_, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.True(t, ok, "SSL_CERT_FILE should be set even with empty cert file")
	got, err := os.ReadFile(combined)
	require.NoError(t, err)
	assert.NotContains(t, string(got), "empty.crt", "empty file content should not appear in combined bundle")
}

func TestLoadCustomCACertificatesSSLCertFileWithTrailingSlash(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	customCert := generateSelfSignedCert(t)
	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "internal-ca.crt"), []byte(customCert), 0o644))

	sslCertDir := t.TempDir()
	systemBundle := filepath.Join(sslCertDir, "ca-certificates.crt")
	require.NoError(t, os.WriteFile(systemBundle, []byte("-----BEGIN SYSTEM BUNDLE-----\n"), 0o644))
	expected := sslCertDir + "/"
	os.Setenv("SSL_CERT_FILE", expected)

	combined := filepath.Join(t.TempDir(), "combined.crt")
	loadCustomCACertificates(certDir, "/should/not/be/used", combined)

	val, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.True(t, ok, "SSL_CERT_FILE should remain set")
	assert.Equal(t, expected, val, "SSL_CERT_FILE should be untouched when it points to directory")
}

func TestLoadCustomCACertificatesAllCustomCertsUnreadable(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	certDir := t.TempDir()
	unreadable := filepath.Join(certDir, "unreadable.crt")
	require.NoError(t, os.WriteFile(unreadable, []byte("cert"), 0o644))
	require.NoError(t, os.Chmod(unreadable, 0o000))

	systemBundle := filepath.Join(t.TempDir(), "ca-certificates.crt")
	require.NoError(t, os.WriteFile(systemBundle, []byte("-----BEGIN SYSTEM BUNDLE-----\n"), 0o644))

	combined := filepath.Join(t.TempDir(), "combined.crt")
	loadCustomCACertificates(certDir, systemBundle, combined)

	_, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.False(t, ok, "SSL_CERT_FILE should be untouched when all custom certs are unreadable")
	_, err := os.Stat(combined)
	assert.True(t, os.IsNotExist(err), "combined bundle should not be written when no custom certs loaded")
}

func TestLoadCustomCACertificatesExistingSSLCertFileMissing(t *testing.T) {
	withCleanSSLCertFileEnv(t)

	customCert := generateSelfSignedCert(t)
	certDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(certDir, "internal-ca.crt"), []byte(customCert), 0o644))

	missingDir := t.TempDir()
	expected := filepath.Join(missingDir, "nonexistent-bundle.crt")
	os.Setenv("SSL_CERT_FILE", expected)

	combined := filepath.Join(t.TempDir(), "combined.crt")
	loadCustomCACertificates(certDir, "/should/not/be/used", combined)

	val, ok := os.LookupEnv("SSL_CERT_FILE")
	assert.True(t, ok, "SSL_CERT_FILE should remain set")
	assert.Equal(t, expected, val, "SSL_CERT_FILE should be untouched when it points to missing file")
	_, err := os.Stat(combined)
	assert.True(t, os.IsNotExist(err), "combined bundle should not be written when SSL_CERT_FILE is missing")
}
