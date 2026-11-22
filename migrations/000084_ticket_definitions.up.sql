-- Freeze the rules in effect at the v83 cutover for future Creates. The cutover
-- records no historical ticket rules and performs no ticket backfill.
CREATE TABLE IF NOT EXISTS ticket_type_definitions (
 type_code VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
 definition_version BIGINT NOT NULL CHECK (definition_version > 0),
 definition_hash TEXT NOT NULL,
 schema_format_version INT NOT NULL CHECK (schema_format_version > 0),
 snapshot JSONB NOT NULL CHECK (jsonb_typeof(snapshot) = 'object'),
 source_draft_revision BIGINT NOT NULL CHECK (source_draft_revision > 0),
 -- Preserve the publishing actor as historical identity independently of users.
 published_by BIGINT CHECK (published_by > 0),
 published_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 -- NULL identifies the v83 cutover baseline, which has no publishing actor.
 CHECK (published_by IS NOT NULL OR definition_version = 1),
 PRIMARY KEY (type_code, definition_version)
);
CREATE INDEX IF NOT EXISTS idx_ticket_definition_hash ON ticket_type_definitions(definition_hash);

-- Format 1 uses the same JSONB SHA-256 for cutover and explicit publication.
CREATE OR REPLACE FUNCTION ticket_definition_snapshot_hash(snapshot JSONB) RETURNS TEXT
 LANGUAGE SQL STRICT AS $$
 SELECT encode(sha256(convert_to(snapshot::text, 'UTF8')), 'hex');
 $$;

CREATE TABLE IF NOT EXISTS ticket_field_identities (
 type_code VARCHAR(50) NOT NULL REFERENCES ticket_types(code) ON DELETE CASCADE,
 field_id BIGSERIAL NOT NULL,
 field_key VARCHAR(50) NOT NULL,
 created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 retired_at TIMESTAMPTZ,
 PRIMARY KEY (type_code,field_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_field_identity_active
 ON ticket_field_identities(type_code,field_key) WHERE retired_at IS NULL;

DO $$ BEGIN
 -- Adding the pointer and freezing the pre-v84 cohort are atomic. Replaying
 -- this SQL must neither capture later drafts nor replace the frozen rules.
 IF EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='ticket_types'::regclass
           AND attname='current_published_version' AND NOT attisdropped) THEN
  RETURN;
 END IF;
 ALTER TABLE ticket_types ADD COLUMN current_published_version BIGINT;
 LOCK TABLE ticket_type_fields IN SHARE MODE;

 -- Map the fields visible at cutover to stable identities.
 INSERT INTO ticket_field_identities(type_code,field_key)
  SELECT type_code,field_key FROM ticket_type_fields;

 WITH field_snapshots AS (
  SELECT f.type_code, bool_and(f.required IS NOT NULL AND f.sort_order IS NOT NULL AND f.field_key <> ''
    AND f.field_type IN ('input','textarea','number','date','select','multi_select','tips','richtext','radio','rating','file')) AS readable,
   jsonb_agg(
   jsonb_build_object('id','0', 'field_id',i.field_id::text, 'type_code',f.type_code,
    'field_key',f.field_key, 'field_label',f.field_label, 'field_type',f.field_type,
    'required',f.required, 'sort_order',f.sort_order)
   || CASE WHEN f.field_options IS NOT NULL THEN jsonb_build_object('field_options',f.field_options) ELSE '{}'::jsonb END
   || CASE WHEN f.field_meta IS NOT NULL THEN jsonb_build_object('field_meta',f.field_meta) ELSE '{}'::jsonb END
   || CASE WHEN f.validate_regex <> '' THEN jsonb_build_object('validate_regex',f.validate_regex) ELSE '{}'::jsonb END
   ORDER BY f.sort_order, f.id) AS fields
  FROM ticket_type_fields f JOIN ticket_field_identities i USING(type_code,field_key)
  GROUP BY f.type_code
 ), type_graphs AS (
  -- Guard set-returning JSON functions against scalar and NULL inputs.
  SELECT t.*,
   CASE WHEN jsonb_typeof(t.states)='array' THEN t.states ELSE '[]'::jsonb END AS state_array,
   CASE WHEN jsonb_typeof(t.transitions)='object' THEN t.transitions ELSE '{}'::jsonb END AS transition_object,
   CASE WHEN jsonb_typeof(t.transition_meta)='object' THEN t.transition_meta ELSE '{}'::jsonb END AS meta_object
  FROM ticket_types t
 ), snapshots AS (
  SELECT t.code, t.version, jsonb_build_object('type',jsonb_build_object(
   'id','0', 'code',t.code, 'name',t.name, 'description',COALESCE(t.description,''),
   'states',t.states, 'transitions',t.transitions, 'transition_meta',t.transition_meta,
   'default_sla_hours',t.default_sla_hours, 'has_custom_fields',t.has_custom_fields,
   'is_active',false, 'version',0, 'created_at','0001-01-01T00:00:00Z',
   'current_published_version',NULL), 'fields',COALESCE(f.fields,'[]'::jsonb)) AS snapshot
  FROM type_graphs t LEFT JOIN field_snapshots f ON f.type_code=t.code
  -- Only freeze shapes the format-1 reader can use. Keep malformed types
  -- unbound until an explicit publication of a corrected draft.
  WHERE t.default_sla_hours IS NOT NULL AND t.has_custom_fields IS NOT NULL
    AND t.is_active IS NOT NULL AND t.created_at IS NOT NULL AND COALESCE(f.readable,true)
    AND t.states=t.state_array AND t.state_array ?& ARRAY['open','assigned','closed']
    AND jsonb_array_length(t.state_array)=(SELECT count(DISTINCT s.value) FROM jsonb_array_elements(t.state_array) s(value))
    AND NOT EXISTS (
     SELECT 1 FROM jsonb_array_elements(t.state_array) s(value)
     WHERE jsonb_typeof(s.value)<>'string' OR (s.value #>> '{}')=''
       OR (s.value #>> '{}')<>btrim(s.value #>> '{}',
          U&'\0009\000A\000B\000C\000D\0020\0085\00A0\1680\2000\2001\2002\2003\2004\2005\2006\2007\2008\2009\200A\2028\2029\202F\205F\3000')
    )
    AND t.transitions=t.transition_object
    AND NOT EXISTS (
     SELECT 1 FROM jsonb_each(t.transition_object) e(key,value)
     WHERE NOT (t.state_array ? e.key) OR jsonb_typeof(e.value)<>'array' OR EXISTS (
      SELECT 1 FROM jsonb_array_elements(
       CASE WHEN jsonb_typeof(e.value)='array' THEN e.value ELSE '[]'::jsonb END
      ) target(value)
      WHERE jsonb_typeof(target.value)<>'string' OR NOT (t.state_array ? (target.value #>> '{}'))
     )
    )
    AND (t.transition_meta='null'::jsonb OR t.transition_meta=t.meta_object)
    AND NOT EXISTS (
     SELECT 1 FROM jsonb_each(t.meta_object) m(key,value)
     WHERE jsonb_typeof(m.value)<>'object'
       OR (strpos(m.key,'>')=0 AND NOT (t.state_array ? m.key))
       OR (strpos(m.key,'>')>0 AND NOT EXISTS (
        SELECT 1 FROM jsonb_each(t.transition_object) e(key,value)
        CROSS JOIN LATERAL jsonb_array_elements_text(
         CASE WHEN jsonb_typeof(e.value)='array' THEN e.value ELSE '[]'::jsonb END
        ) target(state)
        WHERE m.key=e.key || '>' || target.state
       ))
    )
 ), baselines AS (
  INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision)
   SELECT code,1,ticket_definition_snapshot_hash(snapshot),1,snapshot,version FROM snapshots
  RETURNING type_code,definition_version
 )
 UPDATE ticket_types t SET current_published_version=b.definition_version
  FROM baselines b WHERE b.type_code=t.code;

 ALTER TABLE ticket_types ADD CONSTRAINT ticket_types_published_fk
  FOREIGN KEY (code,current_published_version) REFERENCES ticket_type_definitions(type_code,definition_version);
END $$;

ALTER TABLE tickets ADD COLUMN IF NOT EXISTS definition_version BIGINT;
DO $$ BEGIN
 IF NOT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='tickets'::regclass AND conname='tickets_definition_fk') THEN
  ALTER TABLE tickets ADD CONSTRAINT tickets_definition_fk
   FOREIGN KEY (type_code,definition_version) REFERENCES ticket_type_definitions(type_code,definition_version);
 END IF;
END $$;
CREATE INDEX IF NOT EXISTS idx_tickets_definition_status ON tickets(type_code,definition_version,status);

CREATE OR REPLACE FUNCTION reject_ticket_definition_mutation() RETURNS trigger
 LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'published ticket definitions are immutable' USING ERRCODE='23514';
 END $$;
CREATE OR REPLACE TRIGGER ticket_definition_immutable BEFORE UPDATE OR DELETE ON ticket_type_definitions
 FOR EACH ROW EXECUTE FUNCTION reject_ticket_definition_mutation();
