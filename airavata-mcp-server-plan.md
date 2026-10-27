# Airavata MCP Server — Implementation Plan

## 1. Purpose

Researchers and gateway operators currently onboard to Airavata by learning its resource model (clusters, partitions, cluster configs, templates, deployments, data storages, data products, processes) and then hand-writing JSON against the HTTP API. An MCP (Model Context Protocol) server lets an AI assistant do that translation instead: the user states what they want ("run my PyTorch inference script on Expanse and Anvil with these checkpoints") and the assistant turns it into correct, validated Airavata API calls.

The server is built against [`docs/openapi.yaml`](docs/openapi.yaml) (OpenAPI 3.1, ~110 operations across 12 tags) and targets five use cases:

| # | Use case | What the user gets |
|---|----------|--------------------|
| UC1 | Automated application + deployment registration | Describe an app once; templates, deployments and partition defaults are created and validated |
| UC2 | On-demand application generation for dynamic workflows (e.g. federated inferencing) | Ephemeral apps generated, fanned out across clusters, launched and tracked |
| UC3 | SLURM script → Airavata model conversion | Paste an `sbatch` script, get a template + deployment that renders back to an equivalent script |
| UC4 | Registering and discovering data sources | Storages, data products and virtual datasets registered and searchable |
| UC5 | Application dependency graphs and workflow optimization | A graph of apps, data and runs, plus resource and placement recommendations |

### Non-goals

- Replacing the Airavata API or duplicating its authorization. The MCP server is a client; every call runs as the end user's token.
- Executing jobs or moving data itself. It only calls the API; `launchProcess` and the orchestration engine do the work.
- Holding long-lived privileged credentials.

---

## 2. What the API gives us (and what it doesn't)

### Resource model (from the spec)

```
SlurmCluster ──< ClusterPartition
     │
     ├── SlurmClusterConfig (loginUser, workRoot, sshKeyId)  ← owned/shared, per user
     │
ApplicationTemplate (inputs[], outputs[])
     └──< BatchDeployment (templateId, slurmClusterId, slurmRunSection, defaultBatchJobConfig, defaultPartition)
                 └──< Process (BATCH_JOB → batchProcess: deploymentId, slurmClusterConfigId,
                               batchJobConfig, inputMappings[], outputMappings[])
                          ├── statuses[]
                          └── tasks: data-staging / job-submission / job-monitoring / interactive-command

SSHKey ─┬─ SCPDataStorage ──< DataProduct ──── VirtualDataDirectory ──< VirtualDataFile
        └─ SlurmClusterConfig
Sharing: DATA_PRODUCT | SCP_DATA_STORAGE | VIRTUAL_DATA_DIRECTORY | SLURM_CLUSTER_CONFIG → user | group
```

Facts that shape the design:

- **Auth**: CILogon bearer token. Catalogue reads (clusters, partitions, templates, deployments) are anonymous-OK; catalogue writes need `ADMIN`/`SUPER_ADMIN`. Per-user resources go through ownership and sharing. Ownership always comes from the token and never from a body.
- **Validation**: errors use one envelope with `fieldErrors[]` that carry JSON paths (`inputs[0].inputType`). Passed back verbatim, they let the model correct itself.
- **Run sections are Jinja (pongo2)**: `internal/orchestration/slurm.go` renders `slurmRunSection` against a context of `inputs` (`map[name][]string`), `outputs`, `work_dir`, `process_id`, and the resource fields. Airavata owns the `#SBATCH` header (job name, chdir, output/error, time, mail, account, partition, nodes/ntasks/cpus/mem/gres/gpus/constraint). That is the target UC3 converts into.
- **Launch is async**: `POST /processes/{id}/launch` returns `202`. Progress is read through `statuses` and `batchProcess.batchJobStatuses` (Slurm mail states such as `TIME_LIMIT_90` and `END`).
- **Secrets are write-only**: SSH private keys and passphrases are accepted on create and never returned.

### Spec gaps to close first (Phase 0)

| Gap | Impact | Fix |
|-----|--------|-----|
| `virtual-data-directories` and `virtual-data-files` CRUD, `/contents`, `/files` are routed in `api/data/controller/virtual.go` but missing from `openapi.yaml` (only their `/shares` are documented) | UC4 can't register or discover virtual datasets from the spec | Add the paths and the `VirtualDataDirectory*`/`VirtualDataFile*` schemas |
| `TemplateInput.defaultValue` exists in `api/application/dto/application.go` but not in the spec | UC1/UC3 can't set defaults, so every run must map every input | Add it to `TemplateInput` |
| The run-section template context (`inputs.<name>`, `outputs.<name>`, `work_dir`, …) is undocumented | The model can't write correct run sections | Document it in `docs/api.md` and on `BatchDeploymentRequest.slurmRunSection` |
| No search, filter or pagination beyond `templateId`/`deploymentId` | Discovery (UC4/UC5) must scan lists client-side | Acceptable for v1; propose `?q=`/`?limit=` later |
| No labels or metadata on templates/deployments | Can't mark ephemeral (UC2) apps or record provenance | v1: naming convention (`eph-<hash>-…`) plus description tag; propose a `labels` map |
| No workflow/DAG entity | UC5 graphs must be inferred and can't be persisted server-side | v1: client-side; propose a Workflow API (§9) |
| No spec↔router drift check | Gaps like the first row recur | Add a Go test that walks `mux` routes and asserts each one appears in `openapi.yaml` |
| The 16 sharing operations have no `operationId` | The L1 generator has nothing to name those tools after | Add `list…Shares`/`share…`/`update…Share`/`revoke…Share` IDs |

**Phase 0 status:** all rows above are done on branch `mcp-phase0-spec-readiness`, except search/pagination, labels and the workflow entity, which stay as API proposals (§9). The drift test is `internal/server/openapi_test.go`.

**Found during Phase 0, not yet fixed:** at launch, a `FILE` input's mapping value is read as a data product id and staged to `<work_dir>/<inputName>`. The renderer instead gives `{{ inputs.<name>[0] }}` as `<work_dir>/<dataProductId>`, and `internal/orchestration/slurm_test.go` asserts that path. Run sections that use the variable therefore point at a file that is never staged. `FILE_LIST` and `DIRECTORY` inputs are not staged at all. UC1–UC3 depend on this being fixed in the server; until then, generated run sections must use `{{ work_dir }}/<inputName>` for `FILE` inputs.

---

## 3. Architecture

```
┌──────────────────────┐   stdio / Streamable HTTP   ┌────────────────────────────────────────┐   HTTPS + Bearer   ┌────────────────┐
│ MCP host             │ ──────────────────────────► │ airavata-mcp                           │ ─────────────────► │ airavata-server│
│ (Claude Code/Desktop,│                             │  ├─ tools/   L1 generated  (per op)     │                    │  /api/v1/...   │
│  IDE, gateway agent) │ ◄────────────────────────── │  ├─ tools/   L2 composite  (per UC)     │ ◄───────────────── │                │
└──────────────────────┘                             │  ├─ convert/ SLURM parser + renderer    │                    └────────────────┘
                                                     │  ├─ graph/   dependency graph builder   │
                                                     │  ├─ resources/ + prompts/               │
                                                     │  └─ client/  oapi-codegen from spec     │
                                                     └────────────────────────────────────────┘
```

### Decisions

| Concern | Choice | Why |
|---------|--------|-----|
| Language | **Go**, in this repo | Same toolchain as the server (Go 1.25). The UC3 renderer reuses `internal/orchestration` so the preview matches what the server will submit, byte for byte |
| MCP SDK | Official `github.com/modelcontextprotocol/go-sdk` | Maintained reference SDK; supports stdio, Streamable HTTP, tool annotations, structured output, OAuth resource-server metadata |
| API client | Generated with `oapi-codegen` from `docs/openapi.yaml` into `internal/mcp/client` | The spec is the contract. Regenerating on spec change keeps tools in sync; a `go generate` + CI diff check enforces it |
| Layout | `cmd/airavata-mcp/main.go`, `internal/mcp/{server,client,tools,convert,graph,resources,prompts}` | Matches the existing `cmd/airavata-server` + `internal/*` layout |
| Transports | stdio (local, single user) and Streamable HTTP (hosted, multi-user) | stdio for Claude Code/Desktop onboarding; HTTP for gateways and shared deployments |
| Auth (stdio) | Token from `AIRAVATA_TOKEN` or a token file / `airavata-mcp login` (CILogon device flow) | No secrets in MCP config files where avoidable |
| Auth (HTTP) | MCP authorization spec: the server is an OAuth **resource server** advertising CILogon as authorization server; the user's bearer token is forwarded unchanged to Airavata | Airavata keeps sole authority over identity and roles; MCP server is stateless and holds no credentials |
| Config | `AIRAVATA_API_URL` (default `http://localhost:9095`), `--toolsets`, `--read-only`, `--allow-destructive` | Least-privilege by default |

### Reusing server code

`buildSlurmScript` and `renderJinja` in `internal/orchestration/slurm.go` are unexported. Phase 3 adds a small exported, side-effect-free entry point (e.g. `orchestration.RenderPreview(template, deployment, jobConfig, inputs)`) so the MCP server can render "what Airavata would submit" without a database or a running process.

---

## 4. MCP surface

### 4.1 Layer 1 — generated operation tools

One tool per `operationId`, generated from the spec. JSON Schema input comes from path params + request body schema, output from the response schema, and the description from `summary` + `description`.

~110 tools is too many to load into a model's context at once, so they are grouped into **toolsets** by OpenAPI tag and are off by default except for reads:

| Toolset | Tags | Default |
|---------|------|---------|
| `catalog` | Slurm Clusters, Application Templates, Batch Deployments | read-only on |
| `compute_access` | Slurm Cluster Configs, SSH Keys | off |
| `data` | SCP Data Storages, Data Products, (Virtual Data, after Phase 0) | read-only on |
| `processes` | Processes + task sub-resources | read-only on |
| `iam` | Users, Groups | off |
| `sharing` | All `/shares` endpoints | off |

Each generated tool carries MCP annotations derived from the HTTP method: `GET` → `readOnlyHint`; `PUT` → `idempotentHint`; `DELETE` → `destructiveHint`. Destructive tools are registered only with `--allow-destructive` and additionally require `confirm: true` in the arguments.

L1 tools are the escape hatch. Composite tools (L2) are what the model should reach for first.

### 4.2 Layer 2 — composite intent tools (the onboarding value)

Every L2 tool that writes follows **plan → apply**: it takes `dry_run` (default `true`) and returns the exact ordered list of API calls it would make, with bodies, alongside diffs against existing records. The model shows the plan; the user approves; the model calls again with `dry_run: false` and a `plan_hash` so the applied plan is the approved one.

| Tool | UC | Calls | Notes |
|------|----|-------|-------|
| `describe_platform` | all | `listSlurmClusters`, `listClusterPartitions`, `listMySlurmClusterConfigs`, `listSlurmClusterConfigsSharedWithMe`, `listApplicationTemplates`, `listBatchDeployments` | One-shot orientation: what clusters, partitions, and access the caller has |
| `register_application` | UC1 | `listApplicationTemplates` (dedupe by name) → `createApplicationTemplate`/`updateApplicationTemplate` → per cluster: `getSlurmCluster` (validate partition, limits) → `createBatchDeployment`/`updateBatchDeployment` | Idempotent upsert keyed on `templateName` + `slurmClusterId`. Validates `defaultBatchJobConfig` against partition `maxRunTime`/`maxNodes`/`maxMemory`/`gres` before sending. Checks every `{{ inputs.X[0] }}` in the run section names a declared input |
| `validate_application` | UC1 | reads only | Lints a template/deployment pair: unused inputs, undeclared references, Jinja syntax, partition fit |
| `slurm_to_airavata` | UC3 | none (pure) | Parses an sbatch script into a draft `TemplateRequest` + `BatchDeploymentRequest` + warnings + a rendered preview and a diff against the original (§5) |
| `generate_application` | UC2 | none (pure) | From a structured intent (container image or module + entrypoint, model/data inputs, resource class) produce a draft template + run section. Output feeds `register_application` |
| `run_application` | UC2 | `listBatchDeployments` → `getApplicationTemplate` (resolve input names → `templateInputId`) → `createProcess` → `launchProcess` | Takes inputs **by name**, not by UUID, and wraps values as `{"value": …}`/`{"values": […]}`. Uses deployment defaults unless overridden. Requires a `slurmClusterConfigId`, or picks the caller's only config for that cluster |
| `plan_federated_run` / `run_federated` | UC2 | as above, N times | Fan the same template out across deployments on several clusters, splitting a list input (e.g. data shards) across them with a strategy (`even`, `by_gpu_capacity`, `data_locality`). Returns one process per shard plus a group id the server keeps in a description tag |
| `get_run_status` | UC2/UC5 | `getProcess`, `listProcessStatuses`, task lists | Normalized view: process status, latest Slurm state, job id, staging task outcomes, last log line |
| `gc_ephemeral_applications` | UC2 | `listApplicationTemplates`, `listBatchDeployments`, `listProcesses?deploymentId=`, deletes | Removes generated apps older than a TTL with no active runs. Destructive, so gated |
| `register_data_source` | UC4 | `listSSHKeys` → `createSCPDataStorage` → optional `createDataProduct`(s) → optional virtual directory tree | Never accepts private key text in hosted mode (§7). Locally it may read `private_key_path` from disk and call `createSSHKey` |
| `register_dataset` | UC4 | `createDataProduct`, virtual directories/files | Bulk-register many paths on one storage in one call; virtual datasets group them |
| `discover_data` | UC4 | `listMyDataProducts`, `listDataProductsSharedWithMe`, `listMySCPDataStorages`, `listSCPDataStoragesSharedWithMe`, virtual directory listings | Client-side search over name, description, path, storage host, provision status and permission. Results are cached briefly per token |
| `share_resource` | UC4 | `/shares` endpoints | One tool for all four shareable kinds, mirroring the API's single share shape |
| `build_dependency_graph` | UC5 | catalogue + data + process reads | Returns graph JSON + Mermaid (§6) |
| `analyze_workflow` / `optimize_workflow` | UC5 | process history reads | Recommendations; never applies changes on its own. Suggested changes come back as `register_application` plans |

### 4.3 Resources

| URI | Content |
|-----|---------|
| `airavata://openapi` | The spec itself, so the model can look up a schema on demand |
| `airavata://guide/resource-model` | The diagram and facts in §2, as model-facing prose |
| `airavata://guide/run-section` | Run-section Jinja context reference with examples (Phase 0 deliverable) |
| `airavata://clusters/{id}` | Cluster + partitions, with limits |
| `airavata://templates/{id}`, `airavata://deployments/{id}` | Catalogue records |
| `airavata://data-products/{id}`, `airavata://virtual-data-directories/{id}` | Data records, with contents for directories |
| `airavata://processes/{id}` | Process with status history, subscribable for status updates (the server polls internally, since the API has no push) |

### 4.4 Prompts

Guided flows that a host can surface as slash commands:

- `onboard_application` — interview the user (code location, inputs/outputs, target clusters), then `register_application`.
- `convert_slurm_script` — paste a script, review the draft, register.
- `register_my_data` — storage → datasets → sharing.
- `federated_inference` — model + shards + clusters → `plan_federated_run`.
- `optimize_my_workflow` — pick apps/runs, `analyze_workflow`, review recommendations.

---

## 5. UC3 in depth: SLURM script → Airavata model

The conversion is split into a **deterministic** part in Go code and a **semantic** part the model does with the tool's help.

### Deterministic parse (`internal/mcp/convert`)

1. Tokenize `#SBATCH` lines, handling both `--flag=value` and `--flag value`, short flags (`-N`, `-n`, `-c`, `-t`, `-p`, `-A`, `-G`), and continuation/comment edge cases.
2. Map each directive to its target:

| `#SBATCH` | Target |
|-----------|--------|
| `--time`, `-t` (all Slurm time formats) | `defaultBatchJobConfig.wallTimeMinutes` |
| `--account`, `-A` | `allocation` |
| `--partition`, `-p` | `defaultPartition` (checked against the cluster's partitions) |
| `--nodes`, `-N` / `--ntasks`, `-n` / `--ntasks-per-node` / `--cpus-per-task`, `-c` | `nodes` / `ntasks` / `ntasksPerNode` / `cpusPerTask` |
| `--mem` / `--mem-per-cpu` / `--mem-per-gpu` | `mem` / `memPerCpu` / `memPerGpu` |
| `--gres` / `--gpus`, `-G` / `--gpus-per-node` / `--cpus-per-gpu` | `gres` / `gpus` / `gpusPerNode` / `cpusPerGpu` |
| `--constraint`, `-C` | `constraints` |
| `--job-name`, `--output`, `--error`, `--chdir`, `--mail-*` | **Dropped with a note**: Airavata owns them |
| `--array`, `--dependency`, `--qos`, `--exclusive`, `--reservation`, `--export`, others | **Unsupported warning**. `--array` and `--dependency` are flagged as UC5 workflow candidates |

3. The body (everything after the directives) becomes the starting `slurmRunSection`.

### Semantic lifting (model-assisted)

The tool returns **lift candidates** it detects heuristically: positional args (`$1`), `${VAR:-default}` env reads, absolute input-looking paths (`/scratch/.../*.h5`), output redirections, `module load` lines, and container invocations. The model (with the user) decides which become template inputs or outputs. The tool then rewrites the body, replacing each lifted literal with `{{ inputs.<name>[0] }}` (or a loop for `FILE_LIST`), and infers `TemplateInputType` (`FILE`, `FILE_LIST`, `DIRECTORY`, `INTEGER`, …).

### Round-trip verification

The draft is rendered with the exported orchestration preview using the lifted defaults as input values. The tool returns a normalized diff against the original script (ignoring Airavata-owned directives and its preamble and epilogue). An empty semantic diff is the acceptance criterion; anything else is surfaced to the user before `register_application`.

### Test corpus

Golden tests over a corpus of real-world scripts (ACCESS centres' examples, MPI, GPU/PyTorch, Singularity/Apptainer, array jobs) with expected drafts and warnings.

---

## 6. UC5 in depth: dependency graphs and optimization

### Graph model

| Node | Source |
|------|--------|
| `template` | `listApplicationTemplates` |
| `deployment` | `listBatchDeployments` |
| `cluster` / `partition` | `listSlurmClusters` |
| `data_product` / `virtual_dir` / `storage` | data listings |
| `process` (optional, for history) | `listProcesses?deploymentId=` |

| Edge | Inference |
|------|-----------|
| `deployment → template`, `deployment → cluster` | Direct foreign keys |
| `template.output → template.input` ("can feed") | Type compatibility (`FILE`→`FILE`, `FILE_LIST`/`DIRECTORY`→`FILE_LIST`/`DIRECTORY`) plus name/description similarity. Marked *inferred* with a confidence |
| `process.output → process.input` ("did feed") | An output mapping value of one run that matches an input mapping value or staging `sourcePath` of a later run. Marked *observed* |
| `data_product → process` | Input mapping values and staging tasks that reference the product's path or storage |
| `storage ↔ cluster` locality | Storage `hostName` equal to the cluster `dataHost`/`headnodeHost` |

Output: JSON (nodes, edges, provenance per edge) plus Mermaid for display. A user-supplied workflow spec (YAML DAG of template steps) can be overlaid and validated against the inferred edges.

### Optimizations (`analyze_workflow`)

Each recommendation carries its evidence and comes back as a dry-run plan:

- **Walltime right-sizing**: from status timestamps (`SUBMITTED`→`BEGIN`→`END`). Runs that hit `TIME_LIMIT_*`/`FAIL` after a timeout get a raised walltime; runs that consistently finish under 50% of their request get a lowered one, which helps backfill scheduling.
- **Partition and resource fit**: requests that exceed partition limits, GPU requests on non-GPU partitions, and `mem`/`memPerCpu` conflicts.
- **Data locality**: steps whose inputs live on a storage that is not co-located with the target cluster. Suggest placing that step on a co-located cluster or adding a staging step early.
- **Parallelism**: independent branches of the DAG that can launch concurrently; list inputs that can be sharded (feeds `plan_federated_run`).
- **Deduplication**: near-identical templates and deployments (same run section modulo whitespace) that can be merged.

Executing a workflow end to end needs either a persistent orchestrator or a server-side workflow entity; see §9.

---

## 7. Safety, security, and governance

- **Identity passthrough only**: every API call carries the end user's token; the MCP server never escalates. Admin-only composite steps (catalogue writes) fail fast with a clear "requires ADMIN" message after a pre-check.
- **Secrets**:
  - Tool outputs never include private keys or passphrases (the API can't return them anyway).
  - In HTTP mode, `createSSHKey` is not exposed as a tool and `register_data_source` takes only an existing `sshKeyId`, so key material never enters a model context or a remote transcript.
  - In stdio mode, keys are read from a local `private_key_path` by the server process, never passed as a string argument.
  - Tokens are never logged.
- **Least privilege**: read-only toolsets by default; writes per toolset; destructive tools need `--allow-destructive` plus `confirm: true`.
- **Plan → apply** for every composite write, with `plan_hash` binding so that what runs is what was approved.
- **Injection hygiene**: descriptions, paths and names read from Airavata are returned as data in structured content, never interpolated into tool descriptions or prompts. Generated run sections are linted for obviously dangerous constructs (`rm -rf /`, `curl | sh`) and flagged for review rather than silently registered.
- **Validation passthrough**: API `fieldErrors` are returned as structured tool errors (`isError: true`) with the JSON path, so the model can correct the field and retry.
- **Audit**: structured log per tool call (user id, tool, API operations, status codes, plan hash) to stdout; optional OpenTelemetry traces.
- **Rate limiting**: per-token limits on fan-out tools (`run_federated`, `discover_data`) so one prompt can't flood the API.

---

## 8. Delivery phases

| Phase | Scope | Exit criteria |
|-------|-------|---------------|
| **0 — Spec readiness** | Close the §2 gaps: virtual data paths/schemas, `defaultValue`, run-section docs; add the route↔spec drift test | `openapi.yaml` covers every registered route; drift test green in CI |
| **1 — Skeleton + L1** | `cmd/airavata-mcp`, Go MCP SDK, stdio transport, token config, oapi-codegen client with `go generate`, L1 tool generator with toolsets, annotations, read-only default, error mapping, `airavata://openapi` resource | MCP Inspector lists tools; every read tool works against `dev-tools/compose`; unit tests with an `httptest` fake |
| **2 — UC1 + UC4** | `describe_platform`, `register_application`, `validate_application`, `register_data_source`, `register_dataset`, `discover_data`, `share_resource`; plan→apply framework; resources and prompts for these flows | A new user registers an app on a local compose cluster and a dataset from a natural-language request in one session, with no hand-written JSON |
| **3 — UC3** | Export the orchestration render preview; SBATCH parser; lift candidates; round-trip diff; golden corpus | ≥ 90% of the corpus converts with an empty semantic diff; the rest produce explicit warnings |
| **4 — UC2** | `generate_application`, `run_application`, `get_run_status`, `plan_federated_run`/`run_federated`, `gc_ephemeral_applications`, process resource subscriptions | A federated inference demo: one model, N shards, two clusters, launched and tracked from a single prompt |
| **5 — UC5** | `build_dependency_graph`, `analyze_workflow`/`optimize_workflow`, workflow spec overlay | Graph and recommendations for a seeded multi-step workflow; every recommendation is backed by cited runs |
| **6 — Hosted mode** | Streamable HTTP, OAuth resource-server metadata with CILogon, per-token rate limits, audit logging, container image, Helm/compose service | Deployed next to `airavata-server` in compose; works from a remote MCP client with CILogon login |

Phases 2 and 3 can run in parallel after Phase 1. Phase 4 depends on Phase 2's plan→apply framework. Phase 5 depends on Phase 4's status normalization.

---

## 9. Follow-up API proposals (out of scope for v1)

These improve the MCP server but belong in `airavata-server`:

1. **Labels/metadata** on templates, deployments and processes (`labels: map[string]string`) for ephemeral apps, provenance ("generated-from: slurm-script sha"), and federated run grouping.
2. **Search and pagination** on list endpoints (`?q=`, `?limit=`, `?cursor=`), so discovery doesn't need full scans.
3. **Workflow entity**: a persisted DAG of deployment steps with edges binding outputs to inputs, executed by the orchestration engine. UC5's graph and the MCP workflow spec would map onto it directly.
4. **Process events** (SSE or webhook) to replace polling for status subscriptions.
5. **Render preview endpoint** (`POST /slurm-deployments/{id}/preview`) so remote clients get the exact submission script without linking server code.

---

## 10. Testing strategy

- **Unit**: tool input→request mapping and error mapping against an `httptest` fake built from the spec's examples.
- **Contract**: a CI check that regenerates the client from `docs/openapi.yaml` and fails on diff; an L1 generator test that asserts one tool per `operationId`.
- **Integration**: against `dev-tools/compose` (server + DB + a test Slurm). Each composite tool runs its plan→apply path end to end.
- **Golden**: the SLURM conversion corpus (Phase 3) and graph snapshots (Phase 5).
- **Model-in-the-loop evals**: scripted scenarios per use case run through Claude with the server attached. Scoring covers task success, number of tool calls, wrong-field errors and unsafe actions attempted. Tracked per release to catch description and schema regressions.
- **Security**: tests asserting no tool output contains key material or tokens; destructive tools unreachable without flags; hosted mode rejects private key arguments.

---

## 11. Distribution and usage

- Binary `airavata-mcp` built alongside `airavata-server`; container image for hosted mode.
- Claude Code (local):

  ```bash
  claude mcp add airavata -e AIRAVATA_API_URL=http://localhost:9095 -e AIRAVATA_TOKEN=... -- airavata-mcp --toolsets=catalog,data,processes
  ```

- Project-scoped `.mcp.json` example and a `docs/mcp.md` user guide with the five prompts as walkthroughs.

---

## 12. Open questions

1. **Where should ephemeral (UC2) apps live?** In the shared catalogue (needs ADMIN, visible to all), or should Airavata support user-owned templates? Today templates are admin-written catalogue entries, which blocks non-admin on-demand generation.
2. **Federated inferencing scope**: is "federated" only multi-cluster fan-out with result aggregation, or does it include cross-site model/weight exchange (federated learning style)? The latter needs a staging pattern between runs that the API doesn't model yet.
3. **Workflow execution**: should v1 include a client-side DAG runner in the MCP server (stateful, fragile across sessions), or wait for the server-side Workflow entity (§9.3)?
4. **Hosted deployment owner**: will the MCP server be operated per gateway, or centrally next to each `airavata-server`?
5. **Slurm-only for now?** `ProcessType` has `CLOUD_JOB`, but the API has no cloud deployment resources yet. Confirm UC2/UC3 target Slurm only.
