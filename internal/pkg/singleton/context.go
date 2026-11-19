package singleton

import "context"

type ownerContextKey struct{}

// WithOwner attaches the instance ownership context to ctx. It must remain live
// during graceful shutdown and be canceled when the instance loses ownership.
func WithOwner(ctx, ownerCtx context.Context) context.Context {
	return context.WithValue(ctx, ownerContextKey{}, ownerCtx)
}

// Detach keeps request values and gives detached work the lifetime of the
// instance owner. Callers can layer their own operation timeout over it.
func Detach(ctx context.Context) context.Context {
	values := context.WithoutCancel(ctx)
	owner, ok := ctx.Value(ownerContextKey{}).(context.Context)
	if !ok {
		return values
	}
	return detachedContext{Context: owner, values: values}
}

type detachedContext struct {
	context.Context
	values context.Context
}

func (c detachedContext) Value(key any) any {
	if value := c.values.Value(key); value != nil {
		return value
	}
	// Forward owner cancellation metadata as well as its Done channel.
	return c.Context.Value(key)
}
