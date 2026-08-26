//go:build e2e

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

package e2e

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type env struct {
	coreURL  string
	user     string
	password string
	dbDSN    string
	http     *http.Client
}

func newEnv() *env {
	get := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	return &env{
		coreURL:  get("E2E_CORE_URL", "http://localhost:8080"),
		user:     get("E2E_ADMIN_USER", "admin"),
		password: get("E2E_ADMIN_PASSWORD", "Harbor12345"),
		dbDSN:    get("E2E_DB_DSN", "postgres://postgres:root123@localhost:5432/registry?sslmode=disable"),
		http:     &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *env) do(method, path string, body []byte, contentType string) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, e.coreURL+path, rd)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(e.user, e.password)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return e.http.Do(req)
}

// --- Harbor API ---

func (e *env) createProject(name string, storageLimit int64) error {
	b, _ := json.Marshal(map[string]any{
		"project_name":  name,
		"storage_limit": storageLimit,
		"public":        false,
	})
	resp, err := e.do(http.MethodPost, "/api/v2.0/projects", b, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create project %s: %d %s", name, resp.StatusCode, body)
	}
	return nil
}

func (e *env) projectUsedStorage(name string) (int64, error) {
	resp, err := e.do(http.MethodGet, "/api/v2.0/projects/"+name+"/summary", nil, "")
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("summary %s: %d", name, resp.StatusCode)
	}
	var out struct {
		Quota struct {
			Used struct {
				Storage int64 `json:"storage"`
			} `json:"used"`
		} `json:"quota"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return 0, err
	}
	return out.Quota.Used.Storage, nil
}

// --- minimal OCI push (monolithic blob upload + manifest PUT) ---

type blob struct {
	data   []byte
	digest string
}

func randomBlob(size int) blob {
	data := make([]byte, size)
	_, _ = rand.Read(data)
	sum := sha256.Sum256(data)
	return blob{data: data, digest: "sha256:" + hex.EncodeToString(sum[:])}
}

func (e *env) headBlob(repo string, b blob) (int, error) {
	resp, err := e.do(http.MethodHead, "/v2/"+repo+"/blobs/"+b.digest, nil, "")
	if err != nil {
		return 0, err
	}
	resp.Body.Close()
	return resp.StatusCode, nil
}

func (e *env) pushBlob(repo string, b blob) error {
	resp, err := e.do(http.MethodPost, "/v2/"+repo+"/blobs/uploads/", nil, "")
	if err != nil {
		return err
	}
	loc := resp.Header.Get("Location")
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("initiate upload %s: %d", repo, resp.StatusCode)
	}
	if loc == "" {
		return fmt.Errorf("initiate upload %s: no Location header in %d response", repo, resp.StatusCode)
	}

	// the Location header may be absolute (scheme://host/...) or a path
	uploadURL := loc
	if !strings.HasPrefix(uploadURL, "http://") && !strings.HasPrefix(uploadURL, "https://") {
		uploadURL = e.coreURL + uploadURL
	}
	sep := "?"
	if strings.Contains(uploadURL, "?") {
		sep = "&"
	}
	req, err := http.NewRequest(http.MethodPut, uploadURL+sep+"digest="+b.digest, bytes.NewReader(b.data))
	if err != nil {
		return err
	}
	req.SetBasicAuth(e.user, e.password)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(b.data))
	resp2, err := e.http.Do(req)
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusCreated {
		return fmt.Errorf("put blob %s: %d %s", b.digest, resp2.StatusCode, body)
	}
	return nil
}

func (e *env) pushManifest(repo, tag string, cfg blob, layers []blob) error {
	type desc struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
		Size      int    `json:"size"`
	}
	m := map[string]any{
		"schemaVersion": 2,
		"mediaType":     "application/vnd.oci.image.manifest.v1+json",
		"config": desc{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    cfg.digest, Size: len(cfg.data),
		},
	}
	ls := make([]desc, len(layers))
	for i, l := range layers {
		ls[i] = desc{MediaType: "application/vnd.oci.image.layer.v1.tar", Digest: l.digest, Size: len(l.data)}
	}
	m["layers"] = ls
	mb, _ := json.Marshal(m)

	resp, err := e.do(http.MethodPut, "/v2/"+repo+"/manifests/"+tag, mb, "application/vnd.oci.image.manifest.v1+json")
	if err != nil {
		return err
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("put manifest %s:%s: %d %s", repo, tag, resp.StatusCode, body)
	}
	return nil
}

// randomConfigBlob returns a valid JSON config blob (Harbor's artifact
// processor parses the config, so it must be JSON, not random bytes).
func randomConfigBlob() blob {
	seed := randomBlob(16)
	data, _ := json.Marshal(map[string]string{"author": "e2e", "nonce": seed.digest})
	sum := sha256.Sum256(data)
	return blob{data: data, digest: "sha256:" + hex.EncodeToString(sum[:])}
}

// pushImage pushes config+layers+manifest; layers may be shared across images.
func (e *env) pushImage(repo, tag string, layers []blob) error {
	cfg := randomConfigBlob()
	if err := e.pushBlob(repo, cfg); err != nil {
		return err
	}
	for _, l := range layers {
		// mirror real clients: probe first, upload when absent
		code, err := e.headBlob(repo, l)
		if err != nil {
			return err
		}
		if code != http.StatusOK {
			if err := e.pushBlob(repo, l); err != nil {
				return err
			}
		}
	}
	return e.pushManifest(repo, tag, cfg, layers)
}
