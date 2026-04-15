<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
SPDX-License-Identifier: Apache-2.0
-->

# Claude Code Pre-Push Validation

This script (`claude-pre-push-check.sh`) provides pre-push validation for Claude Code to ensure code quality before pushing to GitHub.

## What It Does

The script runs the following checks in sequence:

1. **Code Generation Check** (`make check-gen`)
   - Verifies generated CRDs, RBAC, and code are up-to-date
   - Ensures manifests match the current API definitions
   - Validates code formatting

2. **Linting** (`make lint`)
   - Runs golangci-lint to catch code quality issues
   - Enforces project coding standards

3. **License Headers** (`make check-license`)
   - Verifies all Go files have proper SPDX license headers
   - Ensures compliance with project licensing

4. **Kustomize Validation** (`./hack/validate-kustomize.sh`)
   - Validates all kustomization.yaml files
   - Ensures Kustomize configuration is correct

5. **Unit Tests** (`make test`)
   - Runs the full test suite
   - Ensures no regressions

## How It Works

The script is automatically triggered by Claude Code when it attempts to run `git push` commands. This is configured via a hook in `.claude/settings.json`:

```json
{
  "hooks": {
    "PreToolUse": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "./hack/claude-pre-push-check.sh",
            "if": "Bash(git push*)",
            "statusMessage": "Running pre-push validation checks..."
          }
        ]
      }
    ]
  }
}
```

## Manual Usage

You can run the validation script manually at any time:

```bash
./hack/claude-pre-push-check.sh
```

This is useful for:
- Pre-push validation before manual git push
- Verifying code quality during development
- Running all checks quickly before creating a PR

## Emergency Bypass

In rare situations where you need to bypass validation (not recommended):

```bash
CLAUDE_SKIP_PREPUSH=1 git push
```

**Warning:** This should only be used for emergency hotfixes. CI checks will still run and may fail if validation is skipped.

## Exit Codes

- `0` - All checks passed, safe to push
- `1` - One or more checks failed, push is blocked

## Performance

Expected runtime:
- `make check-gen`: ~10-20 seconds
- `make lint`: ~10-30 seconds
- `make check-license`: ~5 seconds
- `./hack/validate-kustomize.sh`: ~5 seconds
- `make test`: ~30-60 seconds

**Total: ~60-120 seconds**

## Disabling for Manual Work

If you prefer to run validation manually:

1. Edit `.claude/settings.json` (project-level) or `.claude/settings.local.json` (personal)
2. Remove the entire `PreToolUse` hook entry that contains `claude-pre-push-check.sh`
3. Remember to run `make check` before pushing

## Benefits

- **Early Error Detection**: Catches issues before CI runs
- **Faster Feedback**: No waiting for remote CI
- **Cost Savings**: Reduces failed CI runs
- **Quality Gate**: Ensures consistent code quality
- **Team Alignment**: Everyone runs the same checks

## Relation to CI/CD

This script mirrors the checks run in GitHub Actions:
- `lint.yml` - Runs golangci-lint
- `test.yml` - Runs unit tests
- `check-codegen.yml` - Verifies code generation
- `reuse.yml` - Checks license compliance
- `kustomize-validation.yml` - Validates Kustomize files

By running these checks locally, you catch issues before they reach CI.
