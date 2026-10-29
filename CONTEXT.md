# Domain glossary

Business terms and the names used for them in code. Read before naming a new type, route or column,
or when a request uses a term that maps to more than one thing here.

## Identity and access

| Term | In code | Meaning |
|---|---|---|
| Principal | `auth.Principal` (`Name`, `Authorities`) | The authenticated caller. `Name` equals `iam/model.User.ID`. |
| User | `iam/model.User`, table `users` | A registered person. Id is `cilogon:<sub>` or `root`; never generated. Owning anything requires a row. |
| Role / authority | `role.Role`: `SUPER_ADMIN`, `ADMIN`, `USER`; table `user_roles` | Platform-wide privilege, read from the DB, never from the token. No row = `USER`. |
| Root account | `auth.RootTokenProvider` | Bootstrap `SUPER_ADMIN` whose token is printed at startup. |
| Group | `iam/model.Group`, `GroupMember` | Set of users that a share can name. Member role/status gate what a member may do. |
| Owner | `OwnerID` field, column `user_id` | The user who registered a record. Not transferable via the API. |
| Share / grant | `sharing/model.Sharing`, table `resource_sharings` | `(resourceType, resourceId) → (principalType USER/GROUP, principalId)` with `READ` or `WRITE`. Strongest grant wins. |
| Resource guard | `sharing/service.ResourceGuard` | Per-vertical adapter telling the shared sharing service how to load a record and who controls it. |

## Compute

| Term | In code | Meaning |
|---|---|---|
| Slurm cluster | `compute/model.SlurmCluster` | The machine: head node, optional data host, partitions. Holds no credentials. Admin-written, publicly readable. |
| Partition | `ClusterPartition` | A Slurm queue on a cluster. Sub-resource; reached only through its cluster's path. |
| Cluster config | `SlurmClusterConfig` | *How one person logs in* to a cluster: login user, work root, SSH credential. Owned and shareable. |
| SSH credential | `credentials/model.SSHCredential` | A keypair (+ passphrase). Private to its owner — admins included, never shareable. |

## Applications

| Term | In code | Meaning |
|---|---|---|
| Template | `application/model.Template` | An application's declared inputs and outputs (`TemplateInput`/`TemplateOutput`). |
| Batch deployment | `BatchDeployment` | A template installed somewhere runnable, with a default partition and `BatchJobConfig`. |
| Batch job config | `BatchJobConfig` | Slurm resource request: wall time, allocation/account, nodes, CPUs, GPUs, memory… |

## Data

| Term | In code | Meaning |
|---|---|---|
| SCP data storage | `data/model.SCPDataStorage` | A host+path reached over SCP with a credential. Owned and shareable. |
| Data product | `DataProduct` | A registered file/dir on a storage. Owned and shareable; content can be uploaded/downloaded. |
| Virtual data directory / file | `VirtualDataDirectory`, `VirtualDataFile` | A tree of references to data products (a "virtual dataset"). Shares inherit down the tree. |
| Data transfer | `data/service.DataTransferService` | Moving product content between storages/client over SCP. |

## Runs

| Term | In code | Meaning |
|---|---|---|
| Process | `process/model.Process` | One run. Every other run-related row hangs off it. `ProcessType` is `BATCH_JOB` (or `CLOUD_JOB`). |
| Batch section | `BatchJobProcess` (`Process.BatchProcess`) | What a `BATCH_JOB` run adds: deployment, cluster config, job config, input/output mappings. No route of its own. |
| Process status | `ProcessStatus` (`CREATED`, `SUBMITTED`, `RUNNING`, `COMPLETED`, `FAILED`) | Append-only status history; `Process.LastStatusID` points at the newest (no FK, deliberately). |
| Batch job status | `BatchJobStatus` | What Slurm reported (`BEGIN`, `END`, `FAIL`, `REQUEUE`…), parsed from Slurm notification emails. |
| Tasks | `DataStagingTask`, `JobSubmissionTask`, `JobMonitoringTask`, `InteractiveCommandTask` | Ordered steps of a process (`taskOrder`). |
| Launch | `process/service.LaunchService` | Turns a created process into tasks and starts the submission workflow. |
| Execution engine | `orchestration.ExecutionEngine` | go-workflows worker: workflows `handleBatchJobSubmission`/`handleBatchJobCompletion` drive activities. |
