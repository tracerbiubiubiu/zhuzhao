package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
)

type iamRequestKey struct{}

// IAMRequestPurpose is minted only by successful CasbinAuth. The HTTP body
// cannot supply it. The configured model definition is reused at commit time.
type IAMRequestPurpose struct {
	UserID              int64
	Path, Method, Model string
	SelfService         bool
	// Exactly one credential fact is copied from successful authentication.
	// A nil epoch is distinct from a presented legacy epoch zero.
	SessionEpoch *int64
	PATID        int64
}

func IAMRequestFromContext(ctx context.Context) (IAMRequestPurpose, bool) {
	purpose, ok := ctx.Value(iamRequestKey{}).(IAMRequestPurpose)
	return purpose, ok
}
func withIAMRequest(ctx context.Context, purpose IAMRequestPurpose) context.Context {
	return context.WithValue(ctx, iamRequestKey{}, purpose)
}

// mintIAMRequest copies typed authentication facts before handlers run. Missing,
// malformed or ambiguous facts remain unusable by HTTP business commands.
func mintIAMRequest(c *gin.Context, model string, selfService bool) IAMRequestPurpose {
	p := IAMRequestPurpose{UserID: c.GetInt64("userID"), Path: c.Request.URL.Path, Method: c.Request.Method, Model: model, SelfService: selfService}
	epochValue, hasEpoch := c.Get("session_epoch")
	patValue, hasPAT := c.Get("pat_id")
	if hasEpoch && !hasPAT {
		if epoch, ok := epochValue.(int64); ok && epoch >= 0 {
			p.SessionEpoch = &epoch
		}
	}
	if hasPAT && !hasEpoch {
		if id, ok := patValue.(int64); ok && id > 0 {
			p.PATID = id
		}
	}
	return p
}
