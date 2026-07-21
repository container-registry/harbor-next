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
	"crypto/tls"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goharbor/harbor/src/lib/log"
)

const (
	// Internal TLS ENV
	internalTLSEnable        = "INTERNAL_TLS_ENABLED"
	internalVerifyClientCert = "INTERNAL_VERIFY_CLIENT_CERT"
	internalTLSKeyPath       = "INTERNAL_TLS_KEY_PATH"
	internalTLSCertPath      = "INTERNAL_TLS_CERT_PATH"

	// customCACertDirEnv optionally overrides where operator-supplied custom
	// CA certificates are mounted. Defaults to "/harbor_cust_cert", Harbor's
	// long-standing Helm chart convention (see caBundleSecretName), if unset.
	customCACertDirEnv     = "CUSTOM_CA_CERT_DIR"
	defaultCustomCACertDir = "/harbor_cust_cert"
	// defaultSystemCABundle is the CA bundle baked into Harbor's scratch-based
	// service images at build time.
	defaultSystemCABundle = "/etc/ssl/certs/ca-certificates.crt"
	// combinedCABundlePath is where the merged (system + custom) bundle is
	// written at startup, and what SSL_CERT_FILE is pointed at afterward. It
	// lives in a directory owned by the non-root harbor user, kept separate
	// from the read-only, root-owned /etc/ssl/certs baked in at build time.
	combinedCABundlePath = "/etc/ssl/harbor-custom-ca/ca-certificates.crt"
)

// InternalTLSEnabled returns true if internal TLS enabled
func InternalTLSEnabled() bool {
	return strings.ToLower(os.Getenv(internalTLSEnable)) == "true"
}

// InternalEnableVerifyClientCert returns true if mTLS enabled
func InternalEnableVerifyClientCert() bool {
	return strings.ToLower(os.Getenv(internalVerifyClientCert)) == "true"
}

// GetInternalCertPair used to get internal cert and key pair from environment
func GetInternalCertPair() (tls.Certificate, error) {
	crtPath := os.Getenv(internalTLSCertPath)
	keyPath := os.Getenv(internalTLSKeyPath)
	return tls.LoadX509KeyPair(crtPath, keyPath)
}

// GetInternalTLSConfig return a tls.Config for internal https communicate
func GetInternalTLSConfig() (*tls.Config, error) {
	// generate key pair
	cert, err := GetInternalCertPair()
	if err != nil {
		return nil, fmt.Errorf("internal TLS enabled but can't get cert file %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
	}, nil
}

// NewServerTLSConfig returns a modern tls config,
// refer to https://blog.cloudflare.com/exposing-go-on-the-internet/
func NewServerTLSConfig() *tls.Config {
	return &tls.Config{
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.CurveP256,
			tls.X25519,
		},
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}
}

// LoadCustomCACertificates merges any operator-supplied CA certificates
// mounted at the directory named by CUSTOM_CA_CERT_DIR (default
// /harbor_cust_cert, Harbor's long-standing Helm chart convention) into the
// CA bundle baked into this image at build time, and points Go's TLS stack
// at the merged bundle via SSL_CERT_FILE.
//
// Harbor's scratch-based service images (core, jobservice, registryctl,
// exporter) have no shell or update-ca-certificates, so without this a
// mounted custom CA directory is inert: outbound TLS connections these
// services make back to Harbor's own externally-signed endpoints (e.g. the
// registry's token realm) fail with "certificate signed by unknown
// authority" even though the operator supplied the CA. This must be called
// before any TLS connection is attempted, ideally as the first thing in
// main().
//
// It is a deliberate no-op when the custom cert directory doesn't exist or
// is empty, which is the common case for deployments that don't use a
// private CA.
func LoadCustomCACertificates() {
	loadCustomCACertificates(resolveCustomCACertDir(), defaultSystemCABundle, combinedCABundlePath)
}

// resolveCustomCACertDir returns the CUSTOM_CA_CERT_DIR override if set,
// otherwise the default custom CA cert directory.
func resolveCustomCACertDir() string {
	if dir := os.Getenv(customCACertDirEnv); dir != "" {
		return dir
	}
	return defaultCustomCACertDir
}

// loadCustomCACertificates does the work for LoadCustomCACertificates, with
// paths as parameters so it can be exercised in tests without touching real
// filesystem locations.
func loadCustomCACertificates(certDir, defaultSystemBundlePath, combinedBundlePath string) {
	entries, err := os.ReadDir(certDir)
	if err != nil {
		return
	}

	var certFiles []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".crt", ".pem":
			certFiles = append(certFiles, filepath.Join(certDir, e.Name()))
		}
	}
	if len(certFiles) == 0 {
		return
	}

	combined, err := os.ReadFile(defaultSystemBundlePath)
	if err != nil {
		log.Errorf("custom CA certs found in %s but failed to read system CA bundle %s: %v", certDir, defaultSystemBundlePath, err)
		return
	}

	loaded := 0
	for _, f := range certFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			log.Errorf("failed to read custom CA certificate %s: %v", f, err)
			continue
		}
		combined = append(combined, '\n')
		combined = append(combined, data...)
		loaded++
	}
	if loaded == 0 {
		return
	}

	if err := os.WriteFile(combinedBundlePath, combined, 0o644); err != nil { //nolint:gosec // G703: combinedBundlePath is always the hardcoded combinedCABundlePath constant in production, never derived from external input; only varied by tests
		log.Errorf("failed to write combined CA bundle %s: %v", combinedBundlePath, err)
		return
	}

	os.Setenv("SSL_CERT_FILE", combinedBundlePath)
	log.Infof("loaded %d custom CA certificate(s) from %s into %s", loaded, certDir, combinedBundlePath)
}
