---
paths:
  - "internal/orchestration/**"
  - "internal/ssh/**"
  - "api/process/service/launch.go"
---

# Orchestration, Slurm and SSH

- Workflow functions (`handleBatchJobSubmission`, `handleBatchJobCompletion`) are replayed by go-workflows.
  Put new side effects — SSH, SCP, DB writes, `time.Now()`, `uuid.New` — in an activity registered in
  `StartEngine`, not in the workflow body. Existing workflows already read the DB inline; don't extend that to writes.
- Activities receive an `*ExecutionContext` rebuilt from JSON: use `ctx.set(...)`, never assume `Data` is non-nil,
  and only store JSON-serialisable values in it.
- Remote commands: quote every interpolated value with `ssh.ShellQuote` (see `batch.go` submit command).
  Never build a command with `fmt.Sprintf("... %s", userValue)` unquoted.
- Slurm script values: the pongo2 template renders with autoescape **off**. New `#SBATCH` values must be added to the
  line-break check list in `buildSlurmScript`; values used in the script body need a `_quoted` variant.
- Job state arrives via Slurm notification emails parsed by `parseBatchJobSubject` (`monitoring.go`).
  `HandleBatchJobEmailResponse` returns `nil` on bad input on purpose — an error would leave the email unread forever.
- `ExecutionEngine.orchestrator` is nil in tests and when no backend is configured; guard with `ErrNoWorkflowBackend`.
- Unit-test parsing/rendering without SSH, as `slurm_test.go`, `parsing_test.go` and `monitoring_test.go` do.
  There is no fake Slurm cluster; don't add tests that dial a real host.
