package quay

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/goharbor/harbor/src/common/utils/test"
	adp "github.com/goharbor/harbor/src/pkg/reg/adapter"
	"github.com/goharbor/harbor/src/pkg/reg/model"
)

func getMockAdapter(t *testing.T) (*adapter, *httptest.Server) {
	server := test.NewServer(
		&test.RequestHandlerMapping{
			Method:  http.MethodGet,
			Pattern: "/v2/quay/busybox/manifests/latest",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Println(r.Method, r.URL)
				// sample test data
				data := `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.image.config.v1+json","digest":"sha256:ee780d08a5b4de5192a526d422987f451d9a065e6da42aefe8c3b20023a250c7","size":1457},"layers":[{"mediaType":"application/vnd.oci.image.layer.v1.tar+gzip","digest":"sha256:9c075fe2c773108d2fe2c18ea170548b0ee30ef4e5e072d746e3f934e788b734","size":760854}]}`
				w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(data))
			},
		},
		&test.RequestHandlerMapping{
			Method:  http.MethodGet,
			Pattern: "/v2/",
			Handler: func(w http.ResponseWriter, r *http.Request) {
				fmt.Println(r.Method, r.URL)
				fmt.Println(555)
				w.WriteHeader(http.StatusOK)
			},
		},
	)

	factory, _ := adp.GetFactory(model.RegistryTypeQuay)
	ad, err := factory.Create(&model.Registry{
		Type: model.RegistryTypeQuay,
		URL:  server.URL,
	})
	assert.Nil(t, err)
	a := ad.(*adapter)
	return a, server
}

func TestAdapter_NewAdapter(t *testing.T) {
	factory, err := adp.GetFactory("BadName")
	assert.Nil(t, factory)
	assert.NotNil(t, err)

	factory, err = adp.GetFactory(model.RegistryTypeQuay)
	assert.Nil(t, err)
	assert.NotNil(t, factory)
}

func TestAdapter_HealthCheck(t *testing.T) {
	a, s := getMockAdapter(t)
	defer s.Close()

	health, err := a.HealthCheck()
	assert.Nil(t, err)
	assert.Equal(t, string(health), model.Healthy)
}

func TestAdapter_Info(t *testing.T) {
	a, s := getMockAdapter(t)
	defer s.Close()

	info, err := a.Info()
	assert.Nil(t, err)
	t.Log(info)
}

func TestAdapter_PullManifests(t *testing.T) {
	a, s := getMockAdapter(t)
	defer s.Close()

	registry, _, err := a.PullManifest("quay/busybox", "latest")

	assert.Nil(t, err)
	assert.NotNil(t, registry)
}
