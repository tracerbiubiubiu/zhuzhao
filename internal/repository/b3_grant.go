package repository

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3al"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func (s *B3Store) Grant(ctx context.Context, ref string) (b3al.Grant, error) {
	var g b3al.Grant
	g.AuthorizationRef = ref
	err := IAMDB(ctx, s.db).QueryRow(ctx, `
		SELECT revision::text, subject_type, subject_id, type_name, generation::text,
		       COALESCE(document_id::text,''), read_fields, write_fields
		FROM b3_grants WHERE grant_ref=$1 AND enabled
	`, ref).Scan(&g.AuthorizationRevision, &g.SubjectType, &g.SubjectID, &g.TypeName,
		&g.Generation, &g.DocumentID, &g.ReadFields, &g.WriteFields)
	if errors.Is(err, pgx.ErrNoRows) {
		return g, errcode.ErrForbidden
	}
	return g, err
}

func (s *B3Store) AuthorizeDocumentUpdate(ctx context.Context, actor int64, update b3protocol.DocumentUpdate) error {
	g, err := s.Grant(ctx, update.GrantRef)
	if err != nil {
		return err
	}
	err = b3al.Authorize(b3al.Command{
		SubjectType: "human", SubjectID: strconv.FormatInt(actor, 10),
		AuthorizationRef: update.GrantRef, AuthorizationRevision: update.GrantRevision,
		TypeName: update.TypeName, DocumentID: update.DocumentID, Generation: update.Generation, Patch: update.Patch,
	}, g, true)
	if err != nil {
		return errcode.ErrForbidden
	}
	return nil
}
