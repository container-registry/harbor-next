//go:build e2e

// Package imagebuilder wraps go-containerregistry (crane + random) for pushing
// and pulling synthetic test images, plus docker-buildx shellout for multi-arch
// scenarios.
package imagebuilder

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/random"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

// Creds is the Basic-auth credential pair for a registry.
type Creds struct {
	Username string
	Password string
}

func (c Creds) auth() authn.Authenticator {
	return &authn.Basic{Username: c.Username, Password: c.Password}
}

// Options contains flags common to all operations.
type Options struct {
	Insecure bool // use plaintext HTTP
	Creds    Creds
}

var syntheticImageNonce atomic.Int64

func (o Options) craneOpts() []crane.Option {
	opts := []crane.Option{crane.WithAuth(o.Creds.auth())}
	if o.Insecure {
		opts = append(opts, crane.Insecure)
	}
	return opts
}

func (o Options) remoteOpts() []remote.Option {
	opts := []remote.Option{remote.WithAuth(o.Creds.auth())}
	if o.Insecure {
		opts = append(opts, remote.WithTransport(&http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		}))
	}
	return opts
}

// PushSynthetic generates a random single-arch image of approximately sizeBytes
// and pushes it to ref. Returns the pushed manifest digest as a string.
func PushSynthetic(ref string, sizeBytes int64, opts Options) (string, error) {
	sizeBytes += syntheticImageNonce.Add(1)
	img, err := random.Image(sizeBytes, 1)
	if err != nil {
		return "", fmt.Errorf("build random image: %w", err)
	}
	if err := crane.Push(img, ref, opts.craneOpts()...); err != nil {
		return "", err
	}
	d, err := img.Digest()
	if err != nil {
		return "", err
	}
	return d.String(), nil
}

// Pull fetches ref and returns the manifest digest actually served.
func Pull(ref string, opts Options) (string, error) {
	img, err := crane.Pull(ref, opts.craneOpts()...)
	if err != nil {
		return "", err
	}
	d, err := img.Digest()
	if err != nil {
		return "", err
	}
	return d.String(), nil
}

// Copy an image under one reference to a second tag — used for "tag copy" tests.
func Copy(srcRef, dstRef string, opts Options) error {
	return crane.Copy(srcRef, dstRef, opts.craneOpts()...)
}

// Head performs a HEAD against the manifest; returns the underlying status via
// the typed transport error when absent.
func Head(ref string, opts Options) (*v1.Descriptor, int, error) {
	r, err := name.ParseReference(ref)
	if err != nil {
		return nil, 0, err
	}
	desc, err := remote.Head(r, opts.remoteOpts()...)
	if err != nil {
		if te, ok := err.(*transportError); ok {
			return nil, te.status, nil
		}
		status := extractStatus(err)
		return nil, status, err
	}
	return desc, http.StatusOK, nil
}

// Index returns the manifest of ref. When ref is a multi-platform index, the
// returned descriptor lists child manifests.
func Index(ref string, opts Options) (v1.ImageIndex, error) {
	r, err := name.ParseReference(ref)
	if err != nil {
		return nil, err
	}
	desc, err := remote.Get(r, opts.remoteOpts()...)
	if err != nil {
		return nil, err
	}
	if desc.MediaType != types.OCIImageIndex && desc.MediaType != types.DockerManifestList {
		return nil, fmt.Errorf("%s is not a manifest index (mediaType=%s)", ref, desc.MediaType)
	}
	return desc.ImageIndex()
}

// RawManifest returns the manifest body and media type for an arbitrary ref.
func RawManifest(ref string, opts Options) ([]byte, string, error) {
	r, err := name.ParseReference(ref)
	if err != nil {
		return nil, "", err
	}
	desc, err := remote.Get(r, opts.remoteOpts()...)
	if err != nil {
		return nil, "", err
	}
	return desc.Manifest, string(desc.MediaType), nil
}

// transportError stands in for go-containerregistry's TransportError when
// extracting the status code across package boundaries.
type transportError struct {
	status int
	msg    string
}

func (e *transportError) Error() string { return e.msg }

// extractStatus tries to recover an HTTP status from a go-containerregistry error.
func extractStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}
	// go-containerregistry errors stringify as "… Unauthorized: ..." / "… denied: ..."
	// We accept a small heuristic since the typed transport error isn't exported.
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "401"):
		return http.StatusUnauthorized
	case strings.Contains(msg, "denied"), strings.Contains(msg, "forbidden"), strings.Contains(msg, "403"):
		return http.StatusForbidden
	case strings.Contains(msg, "not found"), strings.Contains(msg, "404"):
		return http.StatusNotFound
	case strings.Contains(msg, "denied_by_immutable"), strings.Contains(msg, "immutable"):
		return http.StatusPreconditionFailed
	case strings.Contains(msg, "projectquotaerror"), strings.Contains(msg, "quota exceeded"):
		return http.StatusForbidden
	default:
		return 0
	}
}

// ClassifyPushError maps an error message back to a stable status kind.
func ClassifyPushError(err error) string {
	if err == nil {
		return "ok"
	}
	msg := strings.ToLower(err.Error())
	switch {
	// Harbor quota messages vary by version:
	//   v2.x: "DENIED: adding N KiB … will exceed the configured upper limit of N MiB."
	//   older: contains "projectquotaerror" or "quota"
	// Check quota before the generic "denied" catch-all.
	case strings.Contains(msg, "projectquotaerror"),
		strings.Contains(msg, "quota"),
		strings.Contains(msg, "upper limit"):
		return "quota_exceeded"
	case strings.Contains(msg, "immutable"), strings.Contains(msg, "denied_by_immutable"):
		return "immutable"
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "401"):
		return "unauthorized"
	case strings.Contains(msg, "denied"), strings.Contains(msg, "forbidden"), strings.Contains(msg, "403"):
		return "forbidden"
	case strings.Contains(msg, "not found"), strings.Contains(msg, "404"):
		return "not_found"
	default:
		return "error"
	}
}
