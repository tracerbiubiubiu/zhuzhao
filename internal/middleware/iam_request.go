package middleware

import "context"

type iamRequestKey struct{}

// IAMRequestPurpose is minted only by successful CasbinAuth. The HTTP body
// cannot supply it. The configured model definition is reused at commit time.
type IAMRequestPurpose struct {
	UserID              int64
	Path, Method, Model string
	SelfService         bool
}

func IAMRequestFromContext(ctx context.Context) (IAMRequestPurpose, bool) {
	purpose, ok := ctx.Value(iamRequestKey{}).(IAMRequestPurpose)
	return purpose, ok
}
func withIAMRequest(ctx context.Context, purpose IAMRequestPurpose) context.Context {
	return context.WithValue(ctx, iamRequestKey{}, purpose)
}
