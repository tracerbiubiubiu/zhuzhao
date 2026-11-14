/* Read-only contract checks. No installation, fixture generation or product imports. */
const fs = require("node:fs");
const path = require("node:path");
const crypto = require("node:crypto");
const { spawnSync } = require("node:child_process");

const core = path.resolve(__dirname, "../../../..");
const schemaPath = path.join(core, "docs/design/capability-interconnect-detail.schema.json");
const before = fs.readFileSync(schemaPath);
const schema = JSON.parse(before.toString("utf8"));
const sha256 = value => crypto.createHash("sha256").update(value).digest("hex");
const fallbackAjv = path.resolve(core, "../zhuzhao-ui/node_modules/.pnpm/ajv@6.15.0/node_modules/ajv");
const moduleRef = process.env.AJV_MODULE || fallbackAjv;
let Ajv;
let entry;
try {
  entry = require.resolve(moduleRef, { paths: [process.cwd()] });
  Ajv = require(entry);
} catch (error) {
  console.error("Existing AJV 6.15.0 is required. Set AJV_MODULE to its module directory; no dependency is installed by this verifier.");
  process.exit(1);
}
let directory = path.dirname(entry);
let version;
while (directory !== path.dirname(directory)) {
  const candidate = path.join(directory, "package.json");
  if (fs.existsSync(candidate)) {
    const pkg = JSON.parse(fs.readFileSync(candidate, "utf8"));
    if (pkg.name === "ajv") { version = pkg.version; break; }
  }
  directory = path.dirname(directory);
}
if (version !== "6.15.0") throw new Error(`Expected existing AJV 6.15.0, found ${version || "unknown"}`);

function canonical(value) {
  if (value === null || typeof value === "boolean") return JSON.stringify(value);
  if (typeof value === "string") {
    for (let index = 0; index < value.length; index++) {
      const point = value.charCodeAt(index);
      if (point >= 0xD800 && point <= 0xDBFF) {
        const next = value.charCodeAt(++index);
        if (!(next >= 0xDC00 && next <= 0xDFFF)) throw new Error("lone surrogate");
      } else if (point >= 0xDC00 && point <= 0xDFFF) throw new Error("lone surrogate");
    }
    return JSON.stringify(value);
  }
  if (typeof value === "number") {
    if (!Number.isSafeInteger(value)) throw new Error("unsafe or non-integer number");
    return JSON.stringify(value);
  }
  if (Array.isArray(value)) return "[" + value.map(canonical).join(",") + "]";
  if (typeof value !== "object") throw new Error("not JSON");
  return "{" + Object.keys(value).sort().map(key => canonical(key) + ":" + canonical(value[key])).join(",") + "}";
}
function projection(command) {
  const result = {
    capability_id: command.capability_id, contract_version: command.contract_version,
    target: command.target, params: command.params,
    subject: { subject_type: command.principal.subject_type, subject_id: command.principal.subject_id },
  };
  if (command.principal.delegation_ref) result.subject.delegation_ref = command.principal.delegation_ref;
  if (command.expected_definition_hash) result.expected_definition_hash = command.expected_definition_hash;
  return result;
}
const inputHash = command => sha256(canonical(projection(command)));
const ajv = new Ajv({ allErrors: true, jsonPointers: true, schemaId: "auto" });
const validate = ajv.compile(schema);
const rows = schema["x-validation-cases"].map(test => {
  const valid = validate(test.value);
  const errors = valid ? [] : JSON.parse(JSON.stringify(validate.errors));
  const matchedKeyword = !test.expected_error_keyword || errors.some(error =>
    error.keyword === test.expected_error_keyword && error.dataPath === test.expected_error_data_path);
  return { id: test.id, pass: valid === test.expected_valid && matchedKeyword };
});
const al = schema["x-validation-cases"].find(test => test.id === "command_al").value;
const alHash = inputHash(al);
let rejectedUnsafe = false;
try { canonical({ unsafe: 9007199254740992 }); } catch { rejectedUnsafe = true; }
const canonicalCases = [
  { id: "entry_does_not_change_input_identity", pass: inputHash({ ...al, entrypoint: "internal" }) === alHash },
  { id: "subject_changes_input_identity", pass: inputHash({ ...al, principal: { ...al.principal, subject_id: "3" } }) !== alHash },
  { id: "patch_changes_input_identity", pass: inputHash({ ...al, params: { ...al.params, patch: { status: "unhealthy" } } }) !== alHash },
  { id: "key_order_does_not_change_input_identity", pass: canonical({ b: 2, a: 1 }) === canonical({ a: 1, b: 2 }) },
  { id: "unsafe_numeric_value_rejected", pass: rejectedUnsafe },
  { id: "no_unicode_normalization", pass: canonical("\u00e9") !== canonical("e\u0301") },
];
const hashChecks = schema["x-validation-cases"].filter(test => test.expected_valid).flatMap(test => {
  const command = test.value.kind === "command" ? test.value
    : test.value.kind === "al_execution_request" ? test.value.command : null;
  return command && command.principal ? [{ id: test.id, pass: inputHash(command) === command.canonical_input_hash }] : [];
});
const semantic = spawnSync(process.env.PYTHON || "python3", [path.join(__dirname, "verify_semantics.py")], { encoding: "utf8" });
let semanticSummary;
try { semanticSummary = JSON.parse(semantic.stdout); }
catch { throw new Error(`Python verifier did not return a report: ${semantic.error || semantic.stderr}`); }
const unchanged = fs.readFileSync(schemaPath).equals(before);
const failures = [...rows, ...canonicalCases, ...hashChecks].filter(test => !test.pass);
console.log(JSON.stringify({
  schema_cases: rows.length, schema_passed: rows.filter(test => test.pass).length,
  schema_positive: schema["x-validation-cases"].filter(test => test.expected_valid).length,
  schema_negative: schema["x-validation-cases"].filter(test => !test.expected_valid).length,
  canonical_cases: canonicalCases.length, canonical_passed: canonicalCases.filter(test => test.pass).length,
  frozen_command_hash_checks: hashChecks, semantic: semanticSummary,
  schema_unchanged: unchanged, schema_sha256: sha256(before), failures,
  proof_scope: "Read-only selected candidate contracts, not business implementation, PG, provider or multi-instance acceptance.",
}, null, 2));
if (failures.length || !unchanged || semantic.status !== 0) process.exit(1);
