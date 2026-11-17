package service

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"strings"
)

// objectFields validates the complete JSON before any raw-field merge. This
// prevents duplicate action/params keys being interpreted differently by peers.
func objectFields(body json.RawMessage) (map[string]json.RawMessage, error) {
	if len(body) > 128<<10 {
		return nil, errcode.ErrInvalidParams
	}
	if _, err := jobs.EqualParams(body, body); err != nil {
		return nil, errcode.ErrInvalidParams
	}
	if len(bytes.TrimSpace(body)) == 0 || bytes.TrimSpace(body)[0] != '{' {
		return nil, errcode.ErrInvalidParams
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil || fields == nil {
		return nil, errcode.ErrInvalidParams
	}
	if err := canonicalTaskFields(fields, "job_id", "action_id", "trigger_type", "cron_spec", "callback_url", "params", "dept", "enabled", "timeout_secs", "description", "created_by", "owner_service"); err != nil {
		return nil, err
	}
	return fields, nil
}

// Go's struct decoder folds field names while map keys and remarshal ordering
// do not. Reject alternate spellings before any typed authorization decision so
// the exact same definition is checked and forwarded (including Unicode folds).
func canonicalTaskFields(fields map[string]json.RawMessage, canonical ...string) error {
	for key := range fields {
		for _, name := range canonical {
			if key != name && strings.EqualFold(key, name) {
				return errcode.ErrInvalidParams
			}
		}
	}
	return nil
}
func (s *TaskrunnerService) createJobChecked(ctx context.Context, body json.RawMessage) (json.RawMessage, error) {
	fields, err := objectFields(body)
	if err != nil {
		return nil, err
	}
	var definition taskDefinition
	if json.Unmarshal(body, &definition) != nil {
		return nil, errcode.ErrInvalidParams
	}
	// TR defaults enabled to true when absent.
	if value, ok := fields["enabled"]; !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		definition.Enabled = true
	}
	definition.CreatedBy = taskCreator(ctx)
	// Create allocates a new job ID in TR. A caller-supplied job_id cannot
	// borrow an existing fixed-job approval. Create disabled, review its actual
	// identity/definition, then enable through the checked update path.
	if definition.ActionID == maintenanceAction && definition.TriggerType == "cron" && definition.Enabled {
		return nil, errcode.ErrForbidden
	}
	if err := s.policy.authorizeDefinition(ctx, definition); err != nil {
		return nil, err
	}
	fields["created_by"], _ = json.Marshal(definition.CreatedBy)
	out, err := json.Marshal(fields)
	if err != nil {
		return nil, errcode.ErrInvalidParams
	}
	return s.client.CreateJob(ctx, out, definition.CreatedBy)
}
func (s *TaskrunnerService) updateJobChecked(ctx context.Context, id string, body json.RawMessage) (json.RawMessage, error) {
	fields, err := objectFields(body)
	if err != nil {
		return nil, err
	}
	if raw, ok := fields["job_id"]; ok {
		var supplied string
		if json.Unmarshal(raw, &supplied) != nil || supplied != id {
			return nil, errcode.ErrInvalidParams
		}
	}
	original, err := s.client.GetJob(ctx, id)
	if err != nil {
		return nil, err
	}
	definition, err := readTaskDefinition(original, id)
	if err != nil {
		return nil, err
	}
	// Only TR's existing JobPatch fields participate. Immutable action, trigger
	// type, creator, owner and forged extra fields never replace the original.
	merged, _ := json.Marshal(definition)
	var values map[string]json.RawMessage
	_ = json.Unmarshal(merged, &values)
	patch := map[string]json.RawMessage{"job_id": json.RawMessage(strQuote(id))}
	for _, key := range []string{"callback_url", "cron_spec", "params", "dept", "enabled", "timeout_secs", "description"} {
		if value, ok := fields[key]; ok {
			if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
				if key == "params" {
					return nil, errcode.ErrInvalidParams
				}
				continue
			}
			values[key] = value
			patch[key] = value
		}
	}
	merged, _ = json.Marshal(values)
	if json.Unmarshal(merged, &definition) != nil {
		return nil, errcode.ErrInvalidParams
	}
	if err := s.policy.authorizeDefinition(ctx, definition); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(patch)
	return s.client.UpdateJob(ctx, id, raw, taskCreator(ctx))
}
func strQuote(value string) []byte { raw, _ := json.Marshal(value); return raw }
func (s *TaskrunnerService) authorizeExistingTask(ctx context.Context, id string, retry bool) error {
	raw, err := s.client.GetTask(ctx, id)
	if err != nil {
		return err
	}
	var task struct {
		TaskID string `json:"task_id"`
		Action string `json:"action"`
		Params string `json:"params"`
	}
	if json.Unmarshal(raw, &task) != nil || task.TaskID != id || task.Action == "" {
		return errcode.ErrServiceUnavailable
	}
	if task.Action != maintenanceAction {
		return nil
	}
	if err := s.policy.authorizeAction(ctx, task.Action, json.RawMessage(task.Params)); err != nil {
		return err
	}
	if retry {
		_, err = s.policy.AuthorizeCallback(ctx, id, task.Action, json.RawMessage(task.Params))
		return err
	}
	return nil
}
