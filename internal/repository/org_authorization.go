package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// ActiveReferenceMemberships returns the direct organizations available to the
// synthetic reference profile. Ancestors affect validity; they do not add scope.
func (r *OrgRepo) ActiveReferenceMemberships(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := IAMDB(ctx, r.db).Query(ctx, `SELECT o.id FROM user_orgs uo
		JOIN organizations o ON o.id = uo.org_id
		WHERE uo.user_id = $1 AND (uo.expires_at IS NULL OR uo.expires_at > NOW())
		AND (`+OrgSourceActiveSQL("o")+`) ORDER BY o.id LIMIT 257`, userID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[int64])
}

// OrgSourceActiveSQL is the live authorization predicate for a trusted SQL
// organization alias. An inactive ancestor suspends the whole source before
// anchors or scope are derived. Callers pass only source-code alias literals.
func OrgSourceActiveSQL(alias string) string {
	return orgSourceEnabledSQL(alias, true)
}

// OrgDelegationActiveSQL preserves the existing BK-20 archive delegation of
// soft-deleted organizations. Status suspension still applies to every ancestor;
// this exception is not used for L1 grants or new membership qualifications.
func OrgDelegationActiveSQL(alias string) string {
	return orgSourceEnabledSQL(alias, false)
}

func orgSourceEnabledSQL(alias string, live bool) string {
	invalid := `auth_ancestor.status <> 1`
	if live {
		invalid += ` OR auth_ancestor.deleted_at IS NOT NULL`
	}
	// Paths are reusable after soft deletion; only parent IDs identify this
	// source's ancestors. UNION terminates corrupt cycles. A missing/cyclic chain
	// without an actual root is unverifiable and contributes no authorization.
	return `EXISTS (
 WITH RECURSIVE auth_org_chain AS (
  SELECT id,parent_id,status,deleted_at FROM organizations WHERE id=` + alias + `.id
  UNION
  SELECT parent.id,parent.parent_id,parent.status,parent.deleted_at
  FROM organizations parent JOIN auth_org_chain child ON child.parent_id=parent.id
 )
 SELECT 1 WHERE EXISTS(SELECT 1 FROM auth_org_chain WHERE parent_id IS NULL)
 AND NOT EXISTS(SELECT 1 FROM auth_org_chain auth_ancestor WHERE ` + invalid + `)
 )`
}

// EffectiveRoleCTE is the single source for live direct/org/parent projections.
// $1 is the target user. Disabled/deleted parents terminate their own path, while
// independently seeded roles continue to contribute through UNION semantics.
func EffectiveRoleCTE() string {
	return `WITH RECURSIVE seeds AS (
  SELECT ur.role_id AS id FROM user_roles ur
  JOIN roles direct_role ON direct_role.id=ur.role_id AND direct_role.deleted_at IS NULL AND direct_role.status=1
  WHERE ur.user_id=$1
  UNION
  SELECT grants.role_id FROM org_roles grants
  JOIN user_orgs membership ON membership.org_id=grants.org_id
   AND (membership.expires_at IS NULL OR membership.expires_at>NOW())
  JOIN organizations source_org ON source_org.id=membership.org_id
  JOIN roles org_role ON org_role.id=grants.role_id AND org_role.deleted_at IS NULL AND org_role.status=1
  WHERE membership.user_id=$1 AND ` + OrgSourceActiveSQL("source_org") + `
 ), expanded AS (
  SELECT id FROM seeds
  UNION
  SELECT parent.id FROM expanded e
  JOIN roles child ON child.id=e.id AND child.deleted_at IS NULL AND child.status=1
  JOIN roles parent ON parent.id=child.parent_id AND parent.deleted_at IS NULL AND parent.status=1
 ) `
}

// OrgAncestorChainSQL identifies real ancestors and their graph distance.
// Alias arguments are trusted source literals. The visited IDs terminate cycles;
// authorization callers additionally require the appropriate live/BK20 predicate.
func OrgAncestorChainSQL(alias string) string {
	return `WITH RECURSIVE auth_lineage(id,parent_id,distance,seen) AS (
 SELECT id,parent_id,0,ARRAY[id] FROM organizations WHERE id=` + alias + `.id
 UNION ALL
 SELECT parent.id,parent.parent_id,child.distance+1,child.seen||parent.id
 FROM organizations parent JOIN auth_lineage child ON child.parent_id=parent.id
 WHERE NOT parent.id=ANY(child.seen)
 ) SELECT id,parent_id,distance FROM auth_lineage`
}
func OrgAncestorIDsSQL(alias string) string {
	return `SELECT id FROM (` + OrgAncestorChainSQL(alias) + `) auth_lineage_ids`
}

// LiveOrgPathAnchorSQL keeps ltree paths as candidates and proves the selected
// live anchor belongs to the ticket's actual organization identity chain.
func LiveOrgPathAnchorSQL(orgID, paths string) string {
	return `EXISTS(SELECT 1 FROM organizations auth_target_org WHERE auth_target_org.id=` + orgID + `
 AND EXISTS(SELECT 1 FROM organizations auth_live_anchor
 WHERE auth_live_anchor.id IN(` + OrgAncestorIDsSQL("auth_target_org") + `)
 AND (` + OrgSourceActiveSQL("auth_live_anchor") + `)
 AND auth_live_anchor.path=ANY(` + paths + `::ltree[])))`
}
