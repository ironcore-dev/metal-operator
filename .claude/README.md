<!--
SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
SPDX-License-Identifier: Apache-2.0
-->

# Claude Code Configuration

This directory contains configuration for Claude Code.

## Pre-Push Validation (Opt-In)

### Enabling Automatic Validation

To enable automatic pre-push validation when using Claude Code:

1. Copy the example configuration:
   ```bash
   cp .claude/settings.local.json.example .claude/settings.local.json
   ```

2. The hook will now automatically run validation before Claude Code pushes to GitHub

### What Gets Validated

When enabled, these checks run automatically before push:
- Code generation (`make check-gen`)
- Linting (`make lint`)
- License headers (`make check-license`)
- REUSE compliance (`reuse lint`)
- Kustomize validation
- Unit tests (`make test`)

### Manual Validation

You can always run validation manually without enabling the hook:

```bash
./hack/claude-pre-push-check.sh
```

### More Information

See `hack/README-claude-pre-push.md` for detailed documentation.
