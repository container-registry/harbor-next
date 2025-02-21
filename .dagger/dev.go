package main

import (
	"context"
	"dagger/harbor/internal/dagger"
)

// Run Harbor inside Dagger
func (m *Harbor) RunDev(
	ctx context.Context,
	// +optional
	// +defaultPath="."
	source *dagger.Directory,
) (*dagger.Service, error) {
	golang := dag.Container().
		WithExposedPort(8080)

	return golang.AsService(), nil
}
