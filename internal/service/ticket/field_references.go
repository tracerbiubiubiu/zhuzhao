package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// fieldReferences consumes the same schema snapshot and post-G2/pruned JSON as
// validation. Fields omitted by visibility/patch deletion produce no reference.
func fieldReferences(fields []*model.TicketTypeField, raw json.RawMessage) ([]repository.StorageFieldReference, error) {
	if len(fields) == 0 || len(raw) == 0 {
		return nil, nil
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(raw, &data) != nil {
		return nil, errcode.ErrInvalidParams
	}
	var refs []repository.StorageFieldReference
	for _, field := range fields {
		value, exists := data[field.FieldKey]
		if !exists || string(value) == "null" {
			continue
		}
		switch field.FieldType {
		case "file":
			var keys []string
			if json.Unmarshal(value, &keys) != nil {
				return nil, errcode.ErrInvalidParams
			}
			for _, key := range keys {
				refs = append(refs, repository.StorageFieldReference{ObjectKey: key, Purpose: "ticket_attachment"})
			}
		case "richtext":
			var markdown string
			if json.Unmarshal(value, &markdown) != nil {
				return nil, errcode.ErrInvalidParams
			}
			for _, key := range ExtractFileRefs(markdown) {
				refs = append(refs, repository.StorageFieldReference{ObjectKey: key, Purpose: "ticket_richtext"})
			}
		}
	}
	return refs, nil
}
func (s *Service) bindUpdateReferencesTx(ctx context.Context, tx pgx.Tx, ticketID, actorID int64, explicit []string, refs []repository.StorageFieldReference) error {
	if len(refs)+len(explicit) == 0 {
		return nil
	}
	if s.storageCfg == nil || s.storageRepo == nil {
		return errcode.ErrServiceUnavailable
	}
	cfg := s.storageCfg.Load()
	err := s.storageRepo.BindUpdateAttachments(ctx, tx, cfg.Bucket, actorID, ticketID, explicit, refs, cfg.MaxAttachmentsPerTicket)
	if errors.Is(err, repository.ErrStorageObjectNotFound) || errors.Is(err, repository.ErrStorageAlreadyBound) {
		return errcode.New(errcode.ErrInvalidParams.Code, "custom_data 文件引用无效（不存在/他人新文件/未确认/已删/归属或purpose不符）")
	}
	if errors.Is(err, repository.ErrStorageAttachmentLimit) {
		return errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("工单附件数超过上限（%d）", cfg.MaxAttachmentsPerTicket))
	}
	return err
}
