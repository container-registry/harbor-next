// Copyright Project Harbor Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	  http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build db

package token

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/docker/distribution/registry/auth/token"
	jwt "github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common/rbac"
	"github.com/goharbor/harbor/src/common/rbac/project"
	"github.com/goharbor/harbor/src/common/security"
	"github.com/goharbor/harbor/src/common/utils/test"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/orm"
	_ "github.com/goharbor/harbor/src/pkg/config/db"
	_ "github.com/goharbor/harbor/src/pkg/config/inmemory"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
)

func TestMain(m *testing.M) {
	test.InitDatabaseFromEnv()
	config.Init()
	InitCreators()
	result := m.Run()
	if result != 0 {
		os.Exit(result)
	}
}

func TestGetResourceActions(t *testing.T) {
	cases := map[string]*token.ResourceActions{
		"::": {
			Type:    "",
			Name:    "",
			Actions: []string{},
		},
		"repository": {
			Type:    "repository",
			Name:    "",
			Actions: []string{},
		},
		"repository:": {
			Type:    "repository",
			Name:    "",
			Actions: []string{},
		},
		"repository:library/hello-world": {
			Type:    "repository",
			Name:    "library/hello-world",
			Actions: []string{},
		},
		"repository:library/hello-world:": {
			Type:    "repository",
			Name:    "library/hello-world",
			Actions: []string{},
		},
		"repository:library/hello-world:pull,push": {
			Type:    "repository",
			Name:    "library/hello-world",
			Actions: []string{"pull", "push"},
		},
		"registry:catalog:*": {
			Type:    "registry",
			Name:    "catalog",
			Actions: []string{"*"},
		},
		"repository:192.168.0.1:443/library/hello-world:pull,push": {
			Type:    "repository",
			Name:    "192.168.0.1:443/library/hello-world",
			Actions: []string{"pull", "push"},
		},
	}

	for k, v := range cases {
		r := GetResourceActions([]string{k})[0]
		assert.EqualValues(t, v, r)
	}
}

func getKeyAndCertPath() (string, string) {
	_, f, _, ok := runtime.Caller(0)
	if !ok {
		panic("Failed to get current directory")
	}
	return path.Join(path.Dir(f), "test/private_key.pem"), path.Join(path.Dir(f), "test/root.crt")
}

func getPublicKey(crtPath string) (*rsa.PublicKey, error) {
	crt, err := os.ReadFile(crtPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(crt)
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	return cert.PublicKey.(*rsa.PublicKey), nil
}

// writeECDSAKeyAndCert generates a fresh P-256 key and a self-signed cert for
// it into t.TempDir(), instead of relying on a static key/cert checked into
// git (which trips secret/vuln scanners indefinitely, even for test fixtures).
func writeECDSAKeyAndCert(t *testing.T) (keyPath, crtPath string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("failed to generate cert serial number: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "harbor-test-ecdsa"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		IsCA:         true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create self-signed cert: %v", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal ECDSA private key: %v", err)
	}

	dir := t.TempDir()
	keyPath = filepath.Join(dir, "ecdsa_private_key.pem")
	crtPath = filepath.Join(dir, "ecdsa_root.crt")

	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("failed to write ECDSA private key: %v", err)
	}
	if err := os.WriteFile(crtPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("failed to write ECDSA cert: %v", err)
	}
	return keyPath, crtPath
}

func getECDSAPublicKey(crtPath string) (*ecdsa.PublicKey, error) {
	crt, err := os.ReadFile(crtPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(crt)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM from %s", crtPath)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	pubKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("certificate public key is not ECDSA, got %T", cert.PublicKey)
	}
	return pubKey, nil
}

type harborClaims struct {
	jwt.RegisteredClaims
	// Private claims
	Access []*token.ResourceActions `json:"access"`
}

func TestMakeToken(t *testing.T) {
	pk, crt := getKeyAndCertPath()
	// overwrite the config values for testing.
	privateKey = pk
	ra := []*token.ResourceActions{{
		Type:    "repository",
		Name:    "10.117.4.142/notary-test/hello-world-2",
		Actions: []string{"pull", "push"},
	}}
	svc := "harbor-registry"
	u := "tester"
	tokenJSON, err := MakeToken(orm.Context(), u, svc, ra)
	if err != nil {
		t.Errorf("Error while making token: %v", err)
	}
	tokenString := tokenJSON.Token
	pubKey, err := getPublicKey(crt)
	if err != nil {
		t.Errorf("Error while getting public key from cert: %s", crt)
	}
	tok, err := jwt.ParseWithClaims(tokenString, &harborClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	})
	t.Logf("Token validity: %v", tok.Valid)
	if err != nil {
		t.Errorf("Error while parsing the token: %v", err)
	}
	claims := tok.Claims.(*harborClaims)
	assert.Equal(t, *(claims.Access[0]), *(ra[0]), "Access mismatch")
	assert.Equal(t, claims.Audience, jwt.ClaimStrings([]string{svc}), "Audience mismatch")
}

func TestMakeTokenECDSA(t *testing.T) {
	pkECDSA, crtECDSA := writeECDSAKeyAndCert(t)

	// overwrite the config values for testing.
	oldPrivateKey := privateKey
	defer func() { privateKey = oldPrivateKey }()
	privateKey = pkECDSA

	ra := []*token.ResourceActions{{
		Type:    "repository",
		Name:    "10.117.4.142/notary-test/hello-world-2",
		Actions: []string{"pull", "push"},
	}}
	svc := "harbor-registry"
	u := "tester"
	tokenJSON, err := MakeToken(orm.Context(), u, svc, ra)
	if err != nil {
		t.Fatalf("Error while making token: %v", err)
	}
	tokenString := tokenJSON.Token
	pubKey, err := getECDSAPublicKey(crtECDSA)
	if err != nil {
		t.Fatalf("Error while getting public key from cert: %s", crtECDSA)
	}
	tok, err := jwt.ParseWithClaims(tokenString, &harborClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	})
	t.Logf("Token validity: %v", tok.Valid)
	if err != nil {
		t.Fatalf("Error while parsing the token: %v", err)
	}
	claims := tok.Claims.(*harborClaims)
	assert.Equal(t, *(claims.Access[0]), *(ra[0]), "Access mismatch")
	assert.Equal(t, claims.Audience, jwt.ClaimStrings([]string{svc}), "Audience mismatch")
}

func TestParseTokenECDSA(t *testing.T) {
	pkECDSA, crtECDSA := writeECDSAKeyAndCert(t)

	// overwrite the config values for testing.
	oldPrivateKey := privateKey
	defer func() { privateKey = oldPrivateKey }()
	privateKey = pkECDSA

	ra := []*token.ResourceActions{{
		Type:    "repository",
		Name:    "10.117.4.142/library/alpine",
		Actions: []string{"pull"},
	}}
	svc := "harbor-registry"
	u := "reader"

	// Make a token with ECDSA key
	tokenJSON, err := MakeToken(orm.Context(), u, svc, ra)
	if err != nil {
		t.Fatalf("Error while making token: %v", err)
	}

	// Parse the ECDSA token
	tokenString := tokenJSON.Token
	pubKey, err := getECDSAPublicKey(crtECDSA)
	if err != nil {
		t.Fatalf("Error while getting public key from cert: %s", crtECDSA)
	}

	tok, err := jwt.ParseWithClaims(tokenString, &harborClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return pubKey, nil
	})
	if err != nil {
		t.Fatalf("Error while parsing ECDSA token: %v", err)
	}

	if !tok.Valid {
		t.Fatal("ECDSA token is not valid")
	}

	claims := tok.Claims.(*harborClaims)
	assert.Equal(t, *(claims.Access[0]), *(ra[0]), "Access mismatch in ECDSA token")
	assert.Equal(t, claims.Audience, jwt.ClaimStrings([]string{svc}), "Audience mismatch in ECDSA token")
}

type parserTestRec struct {
	input       string
	expect      image
	expectError bool
}

func TestInit(t *testing.T) {
	InitCreators()
}

func TestBasicParser(t *testing.T) {
	testList := []parserTestRec{{"library/ubuntu:14.04", image{"library", "ubuntu", "14.04"}, false},
		{"test/hello", image{"test", "hello", ""}, false},
		{"myimage:14.04", image{}, true},
		{"org/team/img", image{"org", "team/img", ""}, false},
	}

	p := &basicParser{}
	for _, rec := range testList {
		r, err := p.parse(rec.input)
		if rec.expectError {
			assert.Error(t, err, fmt.Sprintf("Expected error for input: %s", rec.input))
		} else {
			assert.Nil(t, err, "Expected no error for input: %s", rec.input)
			assert.Equal(t, rec.expect, *r, "result mismatch for input: %s", rec.input)
		}
	}
}

func TestEndpointParser(t *testing.T) {
	p := &endpointParser{
		"10.117.4.142:5000",
	}
	testList := []parserTestRec{{"10.117.4.142:5000/library/ubuntu:14.04", image{"library", "ubuntu", "14.04"}, false},
		{"myimage:14.04", image{}, true},
		{"10.117.4.142:80/library/myimage:14.04", image{}, true},
		{"library/myimage:14.04", image{}, true},
		{"10.117.4.142:5000/myimage:14.04", image{}, true},
		{"10.117.4.142:5000/org/team/img", image{"org", "team/img", ""}, false},
	}
	for _, rec := range testList {
		r, err := p.parse(rec.input)
		if rec.expectError {
			assert.Error(t, err, fmt.Sprintf("Expected error for input: %s", rec.input))
		} else {
			assert.Nil(t, err, "Expected no error for input: %s", rec.input)
			assert.Equal(t, rec.expect, *r, "result mismatch for input: %s", rec.input)
		}
	}
}

type fakeSecurityContext struct {
	isAdmin   bool
	rcActions map[rbac.Resource][]rbac.Action
}

func (f *fakeSecurityContext) Name() string {
	return "fake"
}

func (f *fakeSecurityContext) IsAuthenticated() bool {
	return true
}

func (f *fakeSecurityContext) GetUsername() string {
	return "jack"
}

func (f *fakeSecurityContext) IsSysAdmin() bool {
	return f.isAdmin
}
func (f *fakeSecurityContext) IsSolutionUser() bool {
	return false
}
func (f *fakeSecurityContext) Can(ctx context.Context, action rbac.Action, resource rbac.Resource) bool {
	if actions, ok := f.rcActions[resource]; ok {
		if slices.Contains(actions, action) {
			return true
		}
	}
	return false
}

func (f *fakeSecurityContext) GetMyProjects() ([]*proModels.Project, error) {
	return nil, nil
}
func (f *fakeSecurityContext) GetProjectRoles(any) []int {
	return nil
}

func TestFilterAccess(t *testing.T) {
	// TODO put initial data in DB to verify repository filter.
	var err error
	s := []string{"registry:catalog:*"}
	a1 := GetResourceActions(s)
	a3 := GetResourceActions(s)

	ra1 := token.ResourceActions{
		Type:    "registry",
		Name:    "catalog",
		Actions: []string{"*"},
	}
	ra2 := token.ResourceActions{
		Type:    "registry",
		Name:    "catalog",
		Actions: []string{},
	}

	ctx := func(secCtx security.Context) context.Context {
		return security.NewContext(context.TODO(), secCtx)
	}

	err = filterAccess(ctx(&fakeSecurityContext{
		isAdmin: true,
	}), a1, nil, registryFilterMap)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, ra1, *a1[0], "Mismatch after registry filter Map")

	err = filterAccess(ctx(&fakeSecurityContext{
		isAdmin: false,
	}), a3, nil, registryFilterMap)
	assert.Nil(t, err, "Unexpected error: %v", err)
	assert.Equal(t, ra2, *a3[0], "Mismatch after registry filter Map")
}

func TestParseScopes(t *testing.T) {
	assert := assert.New(t)
	u1 := "/service/token?account=admin&scope=repository%3Alibrary%2Fregistry%3Apush%2Cpull&scope=repository%3Ahello-world%2Fregistry%3Apull&service=harbor-registry"
	r1, _ := url.Parse(u1)
	l1 := parseScopes(r1)
	assert.Equal([]string{"repository:library/registry:push,pull", "repository:hello-world/registry:pull"}, l1)
}

func TestResourceScopes(t *testing.T) {
	sctx := &fakeSecurityContext{
		isAdmin: false,
		rcActions: map[rbac.Resource][]rbac.Action{
			project.NewNamespace(1).Resource(rbac.ResourceRepository): {rbac.ActionPull, rbac.ActionScannerPull},
			project.NewNamespace(2).Resource(rbac.ResourceRepository): {rbac.ActionPull, rbac.ActionScannerPull, rbac.ActionPush},
			project.NewNamespace(3).Resource(rbac.ResourceRepository): {rbac.ActionPull, rbac.ActionScannerPull, rbac.ActionPush, rbac.ActionDelete},
			project.NewNamespace(4).Resource(rbac.ResourceRepository): {},
		},
	}
	ctx := security.NewContext(context.TODO(), sctx)
	cases := []struct {
		rc     rbac.Resource
		expect map[string]struct{}
	}{
		{
			rc: project.NewNamespace(1).Resource(rbac.ResourceRepository),
			expect: map[string]struct{}{
				"pull":         {},
				"scanner-pull": {},
			},
		},
		{
			rc: project.NewNamespace(2).Resource(rbac.ResourceRepository),
			expect: map[string]struct{}{
				"pull":         {},
				"scanner-pull": {},
				"push":         {},
			},
		},
		{
			rc: project.NewNamespace(3).Resource(rbac.ResourceRepository),
			expect: map[string]struct{}{
				"pull":         {},
				"scanner-pull": {},
				"push":         {},
				"delete":       {},
			},
		},
		{
			rc:     project.NewNamespace(4).Resource(rbac.ResourceRepository),
			expect: map[string]struct{}{},
		},
		{
			rc:     project.NewNamespace(5).Resource(rbac.ResourceRepository),
			expect: map[string]struct{}{},
		},
	}
	for _, c := range cases {
		assert.Equal(t, c.expect, resourceScopes(ctx, c.rc))
	}
}
