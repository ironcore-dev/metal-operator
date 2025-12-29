# Metal Operator - Agent Guidelines

This document provides comprehensive guidelines for agentic coding assistants working on the metal-operator codebase. Follow these conventions to maintain code quality, consistency, and proper testing practices.

## Build/Lint/Test Commands

### Core Build Commands
- **Full build**: `make build` - Builds the manager binary with manifests generation and formatting
- **Quick build**: `go build -o bin/manager cmd/manager/main.go` - Builds just the manager binary
- **Docker build**: `make docker-build` - Builds all Docker images (manager, metalprobe, bmctools)
- **Clean build**: `make all` - Equivalent to `make build`

### Testing Commands
- **Run all tests**: `make test` - Runs tests with manifests generation, formatting, vetting, and envtest setup
- **Run tests only**: `make test-only` - Runs tests without code generation (assumes envtest is set up)
- **Run single test**: `go test -run TestName ./path/to/package` - Run specific test function
- **Run package tests**: `go test ./internal/controller/` - Run all tests in a specific package
- **Run with coverage**: `go test -coverprofile=cover.out ./...` - Generate coverage report
- **Run e2e tests**: `make test-e2e` - Requires Kind cluster and proper setup

### Linting and Code Quality
- **Full lint**: `make lint` - Runs golangci-lint with all configured linters
- **Lint and fix**: `make lint-fix` - Runs golangci-lint and auto-fixes issues where possible
- **Format code**: `make fmt` - Runs goimports to format and organize imports
- **Vet code**: `make vet` - Runs go vet for static analysis
- **Check licenses**: `make check-license` - Verifies all Go files have proper license headers
- **Code generation check**: `make check-gen` - Validates generated code is up to date

### Development Setup
- **Start BMC emulator**: `make startbmc` - Starts Redfish mockup server for testing
- **Stop BMC emulator**: `make stopbmc` - Stops the Redfish mockup server
- **Start docs server**: `make startdocs` - Starts local documentation server
- **Run locally**: `make run` - Runs the controller from host (requires cluster access)

## Code Style Guidelines

### Go Formatting and Imports
- **Formatter**: Use `goimports` (not `gofmt`) - automatically organizes imports and formats code
- **Import organization**: Standard library imports first, then third-party, then internal packages
- **Import grouping**: Separate groups with blank lines
- **Unused imports**: Remove immediately - CI will fail on unused imports

### Naming Conventions
- **Packages**: Lowercase, single word when possible (e.g., `bmc`, `controller`, `api`)
- **Types**: PascalCase (e.g., `BIOSSettingsSpec`, `ServerReconciler`)
- **Functions/Methods**: PascalCase for exported, camelCase for unexported
- **Variables**: camelCase, descriptive names (e.g., `biosSettingsSet`, not `bss`)
- **Constants**: PascalCase for exported, camelCase for unexported
- **Struct fields**: PascalCase for exported JSON fields, camelCase for internal
- **Test functions**: `TestXxx` format with descriptive names

### File Structure and Organization
- **API types**: `api/v1alpha1/` - Kubernetes CRD definitions
- **Controllers**: `internal/controller/` - Reconciliation logic
- **BMC logic**: `bmc/` - Baseboard Management Controller interactions
- **Commands**: `cmd/` - CLI applications (manager, metalctl, etc.)
- **Internal packages**: `internal/` - Private application logic
- **Tests**: Colocated with implementation files (`*_test.go`)

### License Headers
All Go files must start with:
```go
// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and IronCore contributors
// SPDX-License-Identifier: Apache-2.0
```

### Error Handling
- **Return errors**: Always check and return errors from functions
- **Error wrapping**: Use `fmt.Errorf("context: %w", err)` for error context
- **Controller errors**: Use `ctrl.Result{}, err` for reconciliation errors
- **Ignore not found**: Use `client.IgnoreNotFound(err)` for Kubernetes API calls
- **Logging**: Use structured logging with `log := ctrl.LoggerFrom(ctx)`

### Controller Patterns
- **Reconciler structure**: Follow standard controller-runtime pattern
- **Finalizers**: Use descriptive finalizer names (e.g., `"metal.ironcore.dev/biossettingsset"`)
- **RBAC comments**: Include `// +kubebuilder:rbac:` comments above reconciler structs
- **Context usage**: Always pass context through function calls
- **Client usage**: Prefer `r.Client` over direct client access

### API Types and Validation
- **Kubebuilder annotations**: Use validation tags like `// +kubebuilder:validation:MinLength=1`
- **JSON tags**: Include for all exported fields
- **Optional fields**: Use `omitempty` and `// +optional` comments
- **Required fields**: Use `// +required` comments
- **Enums**: Define as constants with descriptive names

### Testing Patterns
- **Framework**: Use Ginkgo/Gomega for BDD-style tests
- **Test structure**: Use `Describe`/`Context`/`It` blocks
- **Setup**: Use `BeforeEach` for common test setup
- **Assertions**: Use `Expect(...).To(...)` and `Expect(...).To(Succeed())`
- **Mocking**: Use controller-runtime envtest for integration tests
- **Test naming**: Descriptive names explaining the behavior being tested

### Constants and Magic Numbers
- **Avoid magic numbers**: Define constants for any hardcoded values
- **Group constants**: Keep related constants together at package level
- **Naming**: Use descriptive names explaining the purpose

### Function Length and Complexity
- **Function length**: Keep functions focused and under 50 lines when possible
- **Complexity**: Avoid deeply nested logic - extract helper functions
- **Single responsibility**: Each function should do one thing well

### Logging and Debugging
- **Structured logging**: Use `log.Info()`, `log.Error()` with key-value pairs
- **Log levels**: Use appropriate levels (Info, Error, Debug)
- **Context logging**: Include relevant identifiers (names, namespaces, etc.)

### Security Considerations
- **Credentials**: Never log or store secrets in code
- **Input validation**: Validate all external inputs
- **RBAC**: Ensure proper RBAC permissions for all operations
- **Error messages**: Don't leak sensitive information in error messages

### Documentation
- **Code comments**: Document exported functions, types, and complex logic
- **Package comments**: Include package-level documentation
- **Examples**: Provide usage examples for complex APIs

### Commit Messages
- **Format**: Use imperative mood, present tense
- **Content**: Focus on what and why, not how
- **Examples**: "Add validation for BIOS settings", "Fix BMC connection timeout"

## Pre-commit Checklist
Before committing code:
1. Run `make check` to ensure all checks pass
2. Verify tests pass with `make test`
3. Check linting with `make lint`
4. Ensure code is formatted with `make fmt`
5. Verify license headers are present with `make check-license`
6. Run code generation check with `make check-gen`

## Development Workflow
1. Create feature branch from main
2. Make changes following these guidelines
3. Run full test suite and linting
4. Update documentation if needed
5. Create pull request with descriptive title and description
6. Address review feedback
7. Merge after approval

## Getting Help
- Run `make help` for available Make targets
- Check `README.md` for project overview
- Review existing code for patterns and conventions
- Check GitHub issues for known issues and discussions</content>
<parameter name="filePath">/home/tobi/code/metal-operator/AGENTS.md