package repository

import "context"

// MatchesLiveOrgPaths verifies a path candidate against actual organization IDs.
// Historical archive access is handled independently by its original business,
// CC and BK20 delegation branches.
func (r *TicketRepo) MatchesLiveOrgPaths(ctx context.Context, orgID int64, paths []string) (bool, error) {
	if len(paths) == 0 {
		return false, nil
	}
	var allowed bool
	err := IAMDB(ctx, r.db).QueryRow(ctx, `SELECT `+LiveOrgPathAnchorSQL("$1", "$2"), orgID, paths).Scan(&allowed)
	return allowed, err
}
