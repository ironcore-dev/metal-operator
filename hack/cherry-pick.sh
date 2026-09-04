#!/usr/bin/env bash

set -euo pipefail

usage() {
    echo "Usage: $0 <PR-number> <release-branch>"
    echo ""
    echo "Cherry-pick a merged PR into a release branch and open a new PR."
    echo ""
    echo "Examples:"
    echo "  $0 123 release-v0.1"
    echo "  $0 456 release-v0.2"
    echo ""
    echo "Prerequisites:"
    echo "  - gh CLI installed and authenticated"
    echo "  - Clean working tree"
    echo "  - 'origin' remote pointing to your fork"
    echo "  - 'upstream' remote pointing to the upstream repo"
    exit 1
}

if [[ $# -ne 2 ]]; then
    usage
fi

PR_NUMBER="$1"
RELEASE_BRANCH="$2"

# Verify gh CLI is available
if ! command -v gh &>/dev/null; then
    echo "Error: gh CLI is not installed. See https://cli.github.com/"
    exit 1
fi

# Verify origin (fork) and upstream remotes exist
if ! git remote get-url origin &>/dev/null; then
    echo "Error: 'origin' remote not found."
    echo "Set up your fork as origin:"
    echo "  git remote add origin https://github.com/<your-fork>/<repo>.git"
    exit 1
fi
if ! git remote get-url upstream &>/dev/null; then
    echo "Error: 'upstream' remote not found."
    echo "Set up the upstream repo:"
    echo "  git remote add upstream https://github.com/<org>/<repo>.git"
    exit 1
fi

# Verify clean working tree
if [[ -n "$(git status --porcelain)" ]]; then
    echo "Error: working tree is not clean. Please commit or stash your changes."
    exit 1
fi

# Verify the release branch exists on upstream
if ! git ls-remote --exit-code --heads upstream "${RELEASE_BRANCH}" &>/dev/null; then
    echo "Error: branch '${RELEASE_BRANCH}' does not exist on upstream."
    exit 1
fi

UPSTREAM_URL=$(git remote get-url upstream)
UPSTREAM_REPO=$(gh repo view "${UPSTREAM_URL}" --json nameWithOwner -q '.nameWithOwner')
if [[ -z "${UPSTREAM_REPO}" ]]; then
    echo "Error: could not determine upstream repository from '${UPSTREAM_URL}'."
    exit 1
fi

ORIGIN_URL=$(git remote get-url origin)
FORK_OWNER=$(gh repo view "${ORIGIN_URL}" --json owner -q '.owner.login')
if [[ -z "${FORK_OWNER}" ]]; then
    echo "Error: could not determine fork owner from '${ORIGIN_URL}'."
    exit 1
fi

# Get the merge commit SHA for the PR
MERGE_COMMIT=$(gh pr view "${PR_NUMBER}" --repo "${UPSTREAM_REPO}" --json mergeCommit --jq '.mergeCommit.oid')
if [[ -z "${MERGE_COMMIT}" ]]; then
    echo "Error: PR #${PR_NUMBER} has no merge commit. Is it merged?"
    exit 1
fi

PR_TITLE=$(gh pr view "${PR_NUMBER}" --repo "${UPSTREAM_REPO}" --json title --jq '.title')
CHERRY_PICK_BRANCH="cherry-pick-${PR_NUMBER}-into-${RELEASE_BRANCH}"

echo "Cherry-picking PR #${PR_NUMBER} (\"${PR_TITLE}\") into ${RELEASE_BRANCH}"
echo "  Merge commit: ${MERGE_COMMIT}"
echo "  Branch: ${CHERRY_PICK_BRANCH}"
echo ""

# Fetch latest remote state from upstream
git fetch upstream "${RELEASE_BRANCH}"

# Create cherry-pick branch from the release branch
git switch -c "${CHERRY_PICK_BRANCH}" "upstream/${RELEASE_BRANCH}"

# Cherry-pick the merge commit using the first parent
if ! git cherry-pick -x -m1 "${MERGE_COMMIT}"; then
    echo ""
    echo "Cherry-pick has conflicts. Please resolve them, then run:"
    echo "  git cherry-pick --continue"
    echo "  git push -u origin ${CHERRY_PICK_BRANCH}"
    echo "  gh pr create --repo ${UPSTREAM_REPO} --base ${RELEASE_BRANCH} --head ${FORK_OWNER}:${CHERRY_PICK_BRANCH} --title \"🍒 [${RELEASE_BRANCH}] ${PR_TITLE}\" --body \"Cherry-pick of #${PR_NUMBER} into \`${RELEASE_BRANCH}\`.\""
    exit 1
fi

# Push to fork and open PR against upstream
git push -u origin "${CHERRY_PICK_BRANCH}"
gh pr create \
    --repo "${UPSTREAM_REPO}" \
    --base "${RELEASE_BRANCH}" \
    --head "${FORK_OWNER}:${CHERRY_PICK_BRANCH}" \
    --title "🍒 [${RELEASE_BRANCH}] ${PR_TITLE}" \
    --body "Cherry-pick of #${PR_NUMBER} into \`${RELEASE_BRANCH}\`."

echo ""
echo "Done."
