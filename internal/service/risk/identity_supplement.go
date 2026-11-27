package risk

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

const assessIdentityCommand = "assess_identity"

func validIdentitySupplement(supplement *model.RiskIdentitySupplement) bool {
	if !validSupplementKey(supplement.FindingKey, 200) ||
		!validSupplementKey(supplement.AssetRef.Provider, 100) ||
		!validSupplementKey(supplement.AssetRef.ExternalID, 200) ||
		len(supplement.EvidenceRefs) == 0 || len(supplement.EvidenceRefs) > 20 {
		return false
	}
	for _, ref := range supplement.EvidenceRefs {
		if !validSupplementKey(ref.Provider, 100) || !validSupplementKey(ref.ExternalID, 200) ||
			(ref.Version != "" && !validSupplementKey(ref.Version, 200)) {
			return false
		}
	}
	return true
}

// A submitted value must be non-blank and usable as an identity. The repository
// applies the dispatch basis's missing-field rule to previously stored values.
func validSupplementKey(value string, maxRunes int) bool {
	return value != "" && strings.TrimSpace(value) == value &&
		utf8.ValidString(value) && utf8.RuneCountInString(value) <= maxRunes &&
		!strings.ContainsFunc(value, unicode.IsControl)
}

// A new version of the same evidence reference is still the same evidence source.
func hasIndependentSupplementEvidence(submitted []model.RiskEvidenceRef, sourceEvidence ...[]byte) bool {
	type identity struct{ provider, externalID string }
	seen := make(map[identity]bool)
	for _, raw := range sourceEvidence {
		var refs []model.RiskEvidenceRef
		if len(raw) != 0 && json.Unmarshal(raw, &refs) != nil {
			return false
		}
		for _, ref := range refs {
			seen[identity{ref.Provider, ref.ExternalID}] = true
		}
	}
	for _, ref := range submitted {
		if !seen[identity{ref.Provider, ref.ExternalID}] {
			return true
		}
	}
	return false
}
