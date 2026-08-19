package authctx

import "context"

type workloadKey struct{}

func WithWorkloadSubject(ctx context.Context, subject string) context.Context {
	return context.WithValue(ctx, workloadKey{}, subject)
}

func WorkloadSubject(ctx context.Context) string {
	value, _ := ctx.Value(workloadKey{}).(string)
	return value
}
