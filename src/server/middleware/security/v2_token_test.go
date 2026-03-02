package security

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	registry_token "github.com/docker/distribution/registry/auth/token"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	project_ctl "github.com/goharbor/harbor/src/controller/project"
	"github.com/goharbor/harbor/src/core/service/token"
	"github.com/goharbor/harbor/src/lib/config"
	"github.com/goharbor/harbor/src/lib/log"
	"github.com/goharbor/harbor/src/lib/orm"
	proModels "github.com/goharbor/harbor/src/pkg/project/models"
	v2 "github.com/goharbor/harbor/src/pkg/token/claims/v2"
	projecttesting "github.com/goharbor/harbor/src/testing/controller/project"
	"github.com/goharbor/harbor/src/testing/mock"
)

func TestGenerate(t *testing.T) {
	config.Init()
	vt := &v2Token{}
	req1, _ := http.NewRequest(http.MethodHead, "/api/2.0/", nil)
	ctx := orm.Context()
	assert.Nil(t, vt.Generate(req1))
	req2, _ := http.NewRequest(http.MethodGet, "/v2/library/ubuntu/manifests/v1.0", nil)
	req2.Header.Set("Authorization", "Bearer 123")
	assert.Nil(t, vt.Generate(req2))
	mt, err := token.MakeToken(ctx, "admin", "none", []*registry_token.ResourceActions{})
	require.Nil(t, err)
	req3 := req2.Clone(req2.Context())
	req3.Header.Set("Authorization", fmt.Sprintf("Bearer %s", mt.Token))
	assert.Nil(t, vt.Generate(req3))
	req4 := req3.Clone(req3.Context())
	mt2, err2 := token.MakeToken(ctx, "admin", token.Registry, []*registry_token.ResourceActions{})
	require.Nil(t, err2)
	req4.Header.Set("Authorization", fmt.Sprintf("Bearer %s", mt2.Token))
	assert.NotNil(t, vt.Generate(req4))
}

func makeClaims(iat *time.Time, access []*registry_token.ResourceActions) *v2TokenClaims {
	rc := jwt.RegisteredClaims{}
	if iat != nil {
		rc.IssuedAt = jwt.NewNumericDate(*iat)
	}
	return &v2TokenClaims{
		Claims: v2.Claims{RegisteredClaims: rc},
		Access: access,
	}
}

func TestTokenIssuedAfterProjectCreation(t *testing.T) {
	logger := log.DefaultLogger()
	projectCreated := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	after := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	repoAccess := []*registry_token.ResourceActions{{Type: "repository", Name: "myproject/myimage"}}
	catalogAccess := []*registry_token.ResourceActions{{Type: "registry", Name: "catalog"}}
	bareRepoAccess := []*registry_token.ResourceActions{{Type: "repository", Name: "ubuntu"}}
	proj := &proModels.Project{Name: "myproject", CreationTime: projectCreated}

	tests := []struct {
		name    string
		claims  *v2TokenClaims
		project *proModels.Project
		projErr error
		allowed bool
	}{
		{"after creation - allowed", makeClaims(&after, repoAccess), proj, nil, true},
		{"before creation - rejected", makeClaims(&before, repoAccess), proj, nil, false},
		{"exact creation time - allowed", makeClaims(&projectCreated, repoAccess), proj, nil, true},
		{"non-repo access - skipped", makeClaims(&before, catalogAccess), nil, nil, true},
		{"empty access - allowed", makeClaims(&before, nil), nil, nil, true},
		{"project lookup error - rejected", makeClaims(&after, repoAccess), nil, fmt.Errorf("not found"), false},
		{"bare repo name (no project) - rejected", makeClaims(&after, bareRepoAccess), nil, nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origCtl := project_ctl.Ctl
			defer func() { project_ctl.Ctl = origCtl }()

			mockCtl := &projecttesting.Controller{}
			project_ctl.Ctl = mockCtl
			if tt.project != nil || tt.projErr != nil {
				mock.OnAnything(mockCtl, "GetByName").Return(tt.project, tt.projErr)
			}

			result := tokenIssuedAfterProjectCreation(context.Background(), logger, tt.claims)
			assert.Equal(t, tt.allowed, result)
		})
	}
}
