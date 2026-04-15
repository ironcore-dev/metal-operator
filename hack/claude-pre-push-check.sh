#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and IronCore contributors
# SPDX-License-Identifier: Apache-2.0

# Pre-push validation script for Claude Code
# This script runs comprehensive checks before allowing git push operations.
# It mirrors CI/CD validation to catch issues early.

set -e  # Exit on first failure
set -o pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Allow emergency bypass
if [[ "${CLAUDE_SKIP_PREPUSH:-}" == "1" ]]; then
  echo -e "${YELLOW}⚠️  CLAUDE_SKIP_PREPUSH set - skipping validation${NC}"
  echo "WARNING: Pushing without validation. CI checks may fail."
  exit 0
fi

echo -e "${BLUE}🔍 Running pre-push validation...${NC}"
echo ""

# Track overall status
FAILED=0

# 1. Code generation check (fast, catches common issue)
echo -e "${BLUE}→ Checking generated code is up-to-date...${NC}"
if ! make check-gen; then
  echo -e "${RED}❌ Code generation check failed.${NC}"
  echo "   Fix by running: make check-gen"
  echo "   This ensures CRDs, RBAC, and generated code are current."
  exit 1
fi
echo -e "${GREEN}✓ Code generation check passed${NC}"
echo ""

# 2. Linting (moderate speed)
echo -e "${BLUE}→ Running linter...${NC}"
if ! make lint; then
  echo -e "${RED}❌ Linting failed.${NC}"
  echo "   Fix by running: make lint-fix"
  echo "   This will auto-fix most linting issues."
  exit 1
fi
echo -e "${GREEN}✓ Linting passed${NC}"
echo ""

# 3. License headers (fast)
echo -e "${BLUE}→ Checking license headers...${NC}"
if ! make check-license; then
  echo -e "${RED}❌ License check failed.${NC}"
  echo "   Fix by running: make add-license"
  echo "   This ensures all Go files have proper SPDX headers."
  exit 1
fi
echo -e "${GREEN}✓ License headers valid${NC}"
echo ""

# 4. Kustomize validation (fast)
echo -e "${BLUE}→ Validating kustomize files...${NC}"
if ! ./hack/validate-kustomize.sh; then
  echo -e "${RED}❌ Kustomize validation failed.${NC}"
  echo "   Check kustomization.yaml files in config/ directory."
  exit 1
fi
echo -e "${GREEN}✓ Kustomize validation passed${NC}"
echo ""

# 5. Unit tests (slowest, but critical)
echo -e "${BLUE}→ Running unit tests...${NC}"
if ! make test; then
  echo -e "${RED}❌ Tests failed.${NC}"
  echo "   Fix failing tests before pushing."
  echo "   Run 'make test' to see detailed output."
  exit 1
fi
echo -e "${GREEN}✓ All tests passed${NC}"
echo ""

echo -e "${GREEN}✅ All pre-push checks passed!${NC}"
echo "Safe to push to GitHub."
exit 0
