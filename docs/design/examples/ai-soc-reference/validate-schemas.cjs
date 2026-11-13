const fs = require("fs");
const path = require("path");
if (!process.argv[2]) throw new Error("Pass the existing UI Ajv module path; no dependency is installed by this command.");
const Ajv = require(path.resolve(process.argv[2]));
const ajv = new Ajv({allErrors: true, jsonPointers: true, schemaId: "auto"});
const here = __dirname;
const read = name => JSON.parse(fs.readFileSync(path.join(here, name), "utf8"));
const fixtures = read("fixtures.json");
const domain = read("../../domain-agent-reference-scenario.schema.json");
const skill = read("../../skill-reference-package.schema.json");
const soc = read("../../soc-collaboration-authorization.schema.json");
ajv.addSchema(domain);
const input = ajv.compile({$ref: domain.$id + "#/definitions/reference_run_input"});
const proposal = ajv.compile({$ref: domain.$id + "#/definitions/proposal"});
const approval = ajv.compile({$ref: domain.$id + "#/definitions/approval"});
const checkpoint = ajv.compile({$ref: domain.$id + "#/definitions/checkpoint"});
const receipt = ajv.compile({$ref: domain.$id + "#/definitions/receipt_read_result"});
const manifest = ajv.compile(skill);
const authorization = ajv.compile(soc);
let positives = 0, negatives = 0;
function valid(check, value, name) {
  if (!check(value)) throw new Error(name + ": " + JSON.stringify(check.errors));
  positives++;
}
function invalid(check, value, name) {
  if (check(value)) throw new Error("Accepted semantic schema negative: " + name);
  negatives++;
}
fixtures.agent_cases.forEach(c => valid(input, c.input, c.id));
fixtures.soc_cases.forEach(c => valid(authorization, c, c.id));
const samples = read("contract-examples.json");
valid(proposal, samples.proposal, "proposal");
valid(approval, samples.approval, "approval");
valid(checkpoint, samples.checkpoint, "checkpoint");
valid(receipt, samples.receipt_read_result, "minimum receipt");
const packageManifest = read("skills/risk-remediation-brief/manifest.json");
valid(manifest, packageManifest, "skill manifest");
invalid(input, {...fixtures.agent_cases[0].input, max_tools: 0}, "zero budget");
invalid(input, {...fixtures.agent_cases[0].input, secret: "should-not-be-a-field"}, "unknown secret field");
invalid(approval, {...samples.approval, plan_hash: "unbound"}, "bad plan hash");
invalid(manifest, {...packageManifest, scripts_allowed: true}, "script execution");
invalid(manifest, {...packageManifest, required_capabilities: ["admin.destroy"]}, "unknown capability");
invalid(authorization, {...fixtures.soc_cases[0], action: "arbitrary-soc-execute"}, "unknown incident operation");
invalid(proposal, {...samples.proposal, knowledge_refs: ["knowledge-public"]}, "id-only knowledge reference");
invalid(proposal, {...samples.proposal, knowledge_refs: [{...samples.proposal.knowledge_refs[0], digest: "not-a-content-digest"}]}, "unpinned knowledge content");
invalid(receipt, {...samples.receipt_read_result, plan_hash: samples.approval.plan_hash}, "receipt discloses sensitive plan binding");
console.log(JSON.stringify({schema_positive_samples: positives, schema_negative_samples: negatives, real_network_calls: 0}, null, 2));
