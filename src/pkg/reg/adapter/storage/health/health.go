package health

import "context"

type Checker interface {
	Health(ctx context.Context) error
}
