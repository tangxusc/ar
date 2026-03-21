# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**ar** (allrun) is a deployment automation tool for batch-executing commands on physical machines/VMs in offline/air-gapped environments. Primary use case: Kubernetes cluster deployment via DAG-based pipelines. It runs only on physical machines/VMs — containers (podman) are not supported as a host runtime.

- **Frontend**: Vue 3 + TypeScript + Vite (in `frontend/`)
- **Backend**: Go 1.24 with GraphQL API (gqlgen + Gin) and CLI (Cobra) (in `backend/`)
- **Pipelines**: Step definitions with shell scripts, built as OCI images (in `pipelines/`)
- **Design docs**: Chinese-language architecture documents (in `design/`)

## Build & Run Commands

```bash
# Build backend binary (produces "allrun", copies to /usr/bin/allrun)
make build                    # or: make -C backend build

# Run backend dev server (GraphQL at http://localhost:8080/graphql)
cd backend && make run        # or: cd backend && go run main.go server start

# Run Go tests
cd backend && go test ./pkg/pipeline/...

# Regenerate GraphQL code (after editing .graphqls schemas)
cd backend && go run github.com/99designs/gqlgen generate
```

## Architecture

### Backend Package Structure (`backend/pkg/`)

| Package | Role |
|---------|------|
| `command/` | Cobra CLI registration (root, version). Uses `RegisterCommand()` pattern for extensibility. |
| `config/` | Global flags: `OciRuntimeRoot`, `PipelinesDir`, `ImagesStoreDir`, `LoadTmpRoot`, `NodesDir` (defaults under `/var/lib/ar/`) |
| `container/` | OCI runtime abstraction via libcontainer/runc. Platform-specific: `oci_linux.go` + `oci_stub.go` |
| `pipeline/` | Core engine — template loading, DAG parsing, step execution, build, image management. Largest package (~4K LOC) |
| `graph/` | gqlgen-generated GraphQL server. `generated.go` is auto-generated — do not edit manually |
| `graph/model/` | Auto-generated GraphQL models (`models_gen.go`) — do not edit manually |
| `graph/resolver/` | Hand-written GraphQL resolvers (follow-schema layout, one file per schema) |
| `schema/` | GraphQL schema definitions (`.graphqls` files): pipeline, node, image, version |
| `web/` | Gin HTTP server setup, mounts GraphQL handler on `:8080` |

### Pipeline Execution Flow

1. Load pipeline: unpack OCI image → extract `*.template.json` + step images → store in `/var/lib/ar/pipelines/`
2. Run pipeline: render template with node inventory → parse DAG → execute steps in topological order
3. Each step runs as an OCI container (via runc/libcontainer, NOT podman) with:
   - Entrypoint: `bash -lcx /scripts/remote/pipeline-step.sh`
   - Env vars: `ACTION` (step action), `NODES_MATRIX` (node inventory JSON)
   - Mounts: `/tasks` (shared task dir), `/current-task` (step-specific dir)
4. Task state persisted to `/var/lib/ar/tasks/<pipelineName>/<taskId>/pipeline.json`

### CLI Command Tree

```
ar server start              # Start GraphQL API server
ar pipeline load -i <archive>  # Load pipeline from OCI tar archive
ar pipeline run -p <name> -n <nodes.json>  # Execute pipeline
ar pipeline task list        # List running tasks
ar pipeline task stop -t <id>  # Stop a task
ar pipeline task resume -t <id>  # Resume a stopped task
ar pipeline task log -t <id> [-c container] [-f]  # View logs
ar pipeline build -p <template> -t <tag> -i <images>  # Build pipeline image
ar image pull/push/list/rm/tag/prune/login  # OCI image management
```

### GraphQL API

Schema files in `backend/pkg/schema/`. Key operations:
- Queries: `pipelines`, `pipeline(name)`
- Mutations: `runPipeline`, `stopPipeline`, `resumePipeline`

Config in `backend/gqlgen.yml`. Resolver layout: `follow-schema` (one resolver file per schema).

### Pipeline Definition Format

Pipeline templates are `*.template.json` files defining a DAG of steps. Each step specifies an OCI image, action script, and dependency edges. Templates are rendered with Go text/template using node inventory data.

## Key Conventions

- All code comments and logs are in Chinese
- The project uses Go module path `github.com/tangxusc/ar`
- GraphQL models and `generated.go` are auto-generated — edit `.graphqls` schemas and resolvers only
- Pipeline step images are Alpine-based with bash, jq, yq, skopeo, sshpass, curl
- Container IDs follow pattern: `ar_<pipeline>_<step>_<index>`
- The binary is named `allrun` but the CLI command is `ar`
