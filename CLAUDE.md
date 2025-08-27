# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Kettle is a Kubernetes operator built using the Kubebuilder framework that provides IP Address Management (IPAM) functionality for pods. It manages network resources and allocates IP addresses to pods based on network annotations.

### Core Components

- **Network CRD** (`api/v1alpha1/network_types.go`): Defines Network custom resources with VLAN, CIDR, gateway configuration, and IP allocation tracking
- **Network Controller** (`internal/controller/network_controller.go`): Manages Network resource lifecycle and initialization
- **Pod Controller** (`internal/controller/pod_controller.go`): Handles pod creation/deletion, IP allocation/deallocation, and annotation management
- **Pod Cache** (`internal/cache/pod_cache.go`): Provides shared caching for pod metadata to support garbage collection

### Key Architecture Patterns

- Uses controller-runtime framework with custom predicates for efficient event filtering
- Network controller only reconciles on status changes to allocated IPs to minimize unnecessary reconciliations
- Pod controller filters events based on network annotations (`networking.kettle.io/networks`) and only processes pods with IPAM requirements
- IP allocation follows a first-available strategy from the FreeIPs list in Network status
- Implements proper garbage collection by deallocating IPs when pods are deleted

## Development Commands

### Build and Test
```sh
make build                    # Build manager binary
make test                     # Run unit tests (excludes e2e)
make test-e2e                 # Run e2e tests against Kind cluster
make lint                     # Run golangci-lint
make lint-fix                 # Run golangci-lint with fixes
```

### Code Generation
```sh
make generate                 # Generate DeepCopy methods
make manifests                # Generate CRDs, RBAC, webhooks
```

### Local Development
```sh
make run                      # Run controller locally against configured cluster
make install                  # Install CRDs into cluster
make uninstall                # Remove CRDs from cluster
```

### Docker and Deployment
```sh
make docker-build IMG=<registry>/kettle:tag
make docker-push IMG=<registry>/kettle:tag
make deploy IMG=<registry>/kettle:tag
make undeploy
```

### Dependencies
All development tools are managed via the Makefile and installed to `./bin/`:
- Controller-gen v0.14.0
- Kustomize v5.3.0
- Envtest (Kubebuilder assets) v1.29.0
- golangci-lint v1.57.2

## Testing

- Unit tests use Ginkgo v2 and Gomega
- E2e tests run against Kind clusters with `make test-e2e`
- Test files follow `*_test.go` naming convention
- Controller tests use envtest for Kubernetes API simulation

## Key Constants and Annotations

- `networking.kettle.io/networks`: Pod annotation specifying target network name
- `networking.kettle.io/status`: Pod annotation containing allocated network details (IP, VLAN, gateway, etc.)
- Network conditions: `Initialized`, `FreeIPsUpdated`, `ErrorNoFreeIPs`

## Go Module and Dependencies

- Go 1.21+
- Primary dependencies: controller-runtime v0.17.3, client-go v0.29.2
- Testing: Ginkgo v2.14.0, Gomega v1.30.0