package repository

import "context"

func riskEvidenceReferenceSQL(fileKey string) string {
	reference := "jsonb_build_array(jsonb_build_object('kind', 'file', 'ref', " + fileKey + "))"
	return `(s.evidence_refs @> ` + reference + `
		OR EXISTS (SELECT 1 FROM risk_remediation_reviews rev
			WHERE rev.submission_id=s.id AND rev.evidence_refs @> ` + reference + `))`
}

// hiddenRiskEvidenceSQL is the same current risk authorization for attachment
// metadata and content. Historical references still protect the same bytes.
// Where a file is reused, every risk that references it must be visible.
func hiddenRiskEvidenceSQL(fileKey, actor string) string {
	return `EXISTS (
		SELECT 1 FROM risk_remediation_submissions s
		JOIN risk_items ri ON ri.id=s.risk_id
		JOIN organizations go ON go.id=ri.org_id
		LEFT JOIN organizations ao ON ao.id=ri.affected_org_id
		WHERE ` + riskEvidenceReferenceSQL(fileKey) + `
		  AND NOT COALESCE((` + riskVisiblePredicate(actor) + `), false)
	)`
}

func (r *StorageRepo) RiskEvidenceSubjects(ctx context.Context, keys []string) (map[string][]int64, error) {
	subjects := make(map[string][]int64)
	if len(keys) == 0 {
		return subjects, nil
	}
	rows, err := IAMDB(ctx, r.db).Query(ctx, `
		SELECT file.object_key, array_agg(DISTINCT s.risk_id)
		FROM unnest($1::text[]) AS file(object_key)
		JOIN risk_remediation_submissions s ON `+riskEvidenceReferenceSQL("file.object_key")+`
		GROUP BY file.object_key`, keys)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		var ids []int64
		if err := rows.Scan(&key, &ids); err != nil {
			return nil, err
		}
		subjects[key] = ids
	}
	return subjects, rows.Err()
}

func (r *StorageRepo) IsHiddenRiskEvidence(ctx context.Context, objectKey string, actorID int64) (bool, error) {
	var hidden bool
	err := IAMDB(ctx, r.db).QueryRow(ctx, "SELECT "+hiddenRiskEvidenceSQL("$1::text", "$2"), objectKey, actorID).Scan(&hidden)
	return hidden, err
}
