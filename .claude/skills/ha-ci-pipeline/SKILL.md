---
name: ha-ci-pipeline
description: Diagnose and fix GitHub Actions CI/CD pipeline issues for the ShobuPrime/home-assistant-apps repository. Use this skill when PRs are not auto-merging after validation passes, when CI checks are running unnecessarily for unrelated apps, when GitHub Actions workflows need to be scoped to specific app paths, when the user reports issues with validation-passed labels not triggering merges, or when debugging any workflow interaction between pr-validate.yml, builder.yml, and the update workflows. Also use when adding new workflows or modifying existing CI/CD behavior.
---

# Home Assistant CI Pipeline Skill

This skill helps diagnose and fix CI/CD pipeline issues in the `ShobuPrime/home-assistant-apps` repository. It covers the three interconnected workflows and the auto-merge system.

## Workflow Architecture Overview

The repository has three core workflows that interact:

```
Update workflow (e.g., update-arcane.yml)
  → Creates PR with `automated` label
  → Fires `repository_dispatch` event
       ↓
  ┌────────────────────┐    ┌──────────────┐
  │  pr-validate.yml   │    │  builder.yml  │
  │  (validates files) │    │ (test builds) │
  └────────┬───────────┘    └──────┬───────┘
           │                       │
  Adds `validation-passed`         │
  label if all pass                │
           │                       │
           ├───────────────────────┘
           ↓
  ┌────────────────────────────┐
  │  Auto-merge (inside        │
  │  pr-validate.yml)          │
  │  Polls for Builder, then   │
  │  squash merges if eligible │
  └────────────┬───────────────┘
               │ merge uses GITHUB_TOKEN, so NO push event fires
               ↓
  ┌────────────────────────────┐
  │  repository_dispatch       │
  │  `master-post-merge`       │
  │  → builder.yml validates   │
  │    what just landed        │
  └────────────────────────────┘
```

The last step exists because a `GITHUB_TOKEN` merge raises no push event — without it, every auto-merged commit lands on master unbuilt. See "GITHUB_TOKEN Does Not Trigger Workflows" below.

### Workflow Files

| File | Purpose | Triggers |
|------|---------|----------|
| `.github/workflows/pr-validate.yml` | Structure, changelog, YAML validation + primary auto-merge | `pull_request`, `repository_dispatch` |
| `.github/workflows/builder.yml` | Test-build + smoke-test changed apps | `push`, `pull_request`, `repository_dispatch` (`automated-pr-created`, `master-post-merge`) |
| `.github/workflows/update-*.yml` | Check for upstream updates, create PRs | Schedule (daily), manual |

## How Validation Scoping Works

All validation jobs in `pr-validate.yml` are scoped to only check apps/files that changed in the PR. This prevents unrelated app issues from blocking PRs.

### Current Implementation

| Validation | How it's scoped | Key detail |
|-----------|----------------|------------|
| Structure validation | `git diff` detects changed app dirs, only validates those | Requires `fetch-depth: 0` for diff |
| Changelog validation | `git diff` detects changed app dirs, checks their CHANGELOGs | Already had `fetch-depth: 0` |
| YAML lint | `git diff` detects changed `.yaml`/`.yml` files, only lints those | Skips Python/yamllint install if no YAML changed |
| Builder | `tj-actions/changed-files` detects changed apps, matrix builds only those | Auto-discovers apps from `*/config.yaml` |

### If a validation fails for an unrelated app

This shouldn't happen with scoped validation. If it does, check:

1. Is the failing app actually modified in the PR? (`git diff` against base branch)
2. Did the `fetch-depth: 0` get removed from the checkout step? (shallow clones can't diff)
3. Is the `changed_apps` output empty? (workflow-only changes correctly skip app validation)

### Workflow-only changes (no app files)

When a PR only modifies `.github/` files (workflows, scripts) and no app directories, all scoped validations skip gracefully with messages like "No app-specific changes detected" and "No YAML files changed." This is correct behavior — the checks show as passed (not failed or skipped), so auto-merge still works.

### New app's first PR

When adding a brand new app, all of its files appear as "new" in the diff. The scoping logic correctly detects the new app directory as changed and validates only that app. Existing apps are not affected.

## How Auto-Merge Works

Auto-merge runs as the final job in `pr-validate.yml` after all validations pass:

1. Checks PR is by `github-actions[bot]` with `automated` label, no blocking labels
2. Polls for Builder for up to **30 minutes** (60 attempts * 30 seconds)
3. Verifies Builder passed
4. Checks GitHub mergeability state
5. Merges with squash
6. Fires a `master-post-merge` `repository_dispatch` so Builder validates master (see below)

### The two paths check Builder differently

This asymmetry matters and has caused a real incident:

| Trigger | How Builder is checked |
|---------|------------------------|
| `repository_dispatch` | the whole Builder **workflow run's** conclusion — covers every job automatically |
| `pull_request` | individual **check runs**, matched by an allow-list of job names |

The `pull_request` path's allow-list is `Build ` / `Smoke test ` prefixes plus `Initialize build`. Until #198 it omitted `Smoke test `, so a build that compiled but failed its smoke test could auto-merge: PR #190 merged at 14:57:08 on the strength of `Build lemonade app` alone, while `Smoke test lemonade` was still running — it failed 40 seconds later, on an already-merged PR.

**A new job added to `builder.yml` is not gated until its name matches that allow-list.** If you add one, update the filter in `pr-validate.yml`'s auto-merge job. The `repository_dispatch` path needs no change.

`skipped` counts as a pass there: a matrix job with nothing to build reports `skipped` while keeping its un-expanded `Build ${{ matrix.app }} app` name, which means the PR changed no app rather than that a check failed.

### Label Requirements

| Required labels | Blocking labels |
|----------------|-----------------|
| `automated` | `do-not-merge`, `needs-review`, `on-hold` |

## Troubleshooting: PR Not Auto-Merging

When an automated PR has `validation-passed` but isn't merging, check these causes in order:

### 1. Builder checks not finishing in time

The merge path polls for 15 minutes. If Builder is queued or slow, it gives up.

**Check:** Look at the `pr-validate.yml` "Auto-merge if eligible" step logs for skip reasons.

### 2. Check run name mismatch

On the `pull_request` path the auto-merge filters for check run names:
- `Build *`, `Smoke test *`, or `Initialize build` (Builder workflow)

If names change (e.g., matrix strategy changes) the filter silently stops covering them — it does not error, it just stops gating. That is exactly how the smoke test went ungated until #198.

**Check:** List actual check run names for a PR and compare against the filter patterns:

```bash
# With gh CLI
gh pr checks <PR-NUMBER> --json name --jq '.[].name'

# Without gh CLI (using curl + GitHub API)
curl -s -H "Authorization: token $(git config github.token 2>/dev/null || echo $GITHUB_TOKEN)" \
  "https://api.github.com/repos/ShobuPrime/home-assistant-apps/commits/<HEAD-SHA>/check-runs" \
  | jq -r '.check_runs[].name'

# Or just check locally what names the workflows define
grep -E "^    name:" .github/workflows/pr-validate.yml .github/workflows/builder.yml
grep "name: Build" .github/workflows/builder.yml
```

The auto-merge code filters for these exact patterns:
- Starts with `Build ` (note trailing space) — from builder.yml matrix jobs
- Starts with `Smoke test ` — from builder.yml smoke-test jobs (added #198)
- Equals `Initialize build` — from builder.yml init job

### 3. Missing `validation-passed` label

If the summary job failed to add the label (API rate limit, permissions), check PR labels and the `summary` job logs.

### 4. Mergeability state

GitHub takes time to calculate. Both paths poll but may time out if there are conflicts.

### Quick fix

```bash
# Merge directly
gh pr merge <PR-NUMBER> --squash
```

## GITHUB_TOKEN Does Not Trigger Workflows

This single rule explains most of the repo's odd CI behaviour. **Events caused by `GITHUB_TOKEN` do not start new workflow runs** — with the exception of `repository_dispatch` and `workflow_dispatch`, which are delivered normally. Three consequences, all of which have bitten:

### 1. Bot PRs get no `pull_request` runs

A PR opened by `github-actions[bot]` with `GITHUB_TOKEN` does not trigger `pull_request` workflows. They are recorded as `completed / action_required` — queued awaiting approval, never executed. `gh pr checks <n>` reports "no checks reported on the branch".

That is *why* every `update-*.yml` fires a `repository_dispatch` after creating its PR. It is not redundancy.

**If someone approves those blocked runs**, they execute against the commit the run was *created* with — which may be long stale. Approving #190's blocked run replayed a commit from seven hours earlier, so it ran a `smoke-test.sh` from before the fix that had since landed on master.

### 2. Auto-merged commits get no `push` run

The auto-merge job merges with `GITHUB_TOKEN`, so `builder.yml`'s `push: master` does not fire. Master lands unbuilt and un-smoke-tested. Human merges through the UI are unaffected, which is why this stayed invisible.

Fixed by dispatching `master-post-merge` after a successful merge, with `app: changed` so Builder diffs the merge commit against its parent and builds exactly what landed. To confirm master is being validated:

```bash
# every master commit and whether a Builder run exists for it
for sha in $(git log --format='%h' origin/master -10); do
  full=$(git rev-parse $sha)
  n=$(gh run list --limit 60 --json headSha,workflowName \
        --jq "[.[] | select(.headSha==\"$full\" and .workflowName==\"Builder\")] | length")
  echo "$sha  builder-runs=$n  $(git log -1 --format=%s $sha | cut -c1-50)"
done
```

### 3. Squash merges break ancestry checks

Merged branches are **not** ancestors of master, so `git log master..branch` still lists commits and `git branch --merged` will not show them. Verify a merge by the PR's state and merge commit, or by content — never by ancestry.

## Adding a New Update Workflow

When creating update workflows for new apps:

### Cron Schedule (avoid conflicts)

Existing schedule:
- 1:00 AM UTC - Base image updates
- 2:00 AM UTC - Portainer LTS + STS
- 3:00 AM UTC - Arcane + Dockhand
- 3:30 AM UTC - Huly

New apps should use unoccupied slots: 4:00, 4:30, 5:00 AM UTC, etc.

### Required conventions

1. Apply the `automated` label for auto-merge eligibility
2. Use `sign-commits: true` (repo enforces signed commits)
3. Fire `repository_dispatch` after PR creation — GitHub doesn't trigger `pull_request` events for PRs created by `GITHUB_TOKEN`:

```yaml
- name: Trigger downstream workflows
  run: |
    curl -X POST \
      -H "Authorization: token ${{ secrets.GITHUB_TOKEN }}" \
      -H "Accept: application/vnd.github.v3+json" \
      https://api.github.com/repos/${{ github.repository }}/dispatches \
      -d '{
        "event_type": "automated-pr-created",
        "client_payload": {
          "pull_request_number": "${{ steps.create-pr.outputs.pull-request-number }}",
          "head_sha": "${{ steps.create-pr.outputs.pull-request-head-sha }}",
          "branch": "update-<app>-${{ version }}",
          "app": "<app-slug>"
        }
      }'
```

## Debugging Checklist

When a PR isn't merging or validations are failing unexpectedly:

1. **Check PR labels**: Has `automated`? Has `validation-passed`? Any blocking labels?
2. **Check workflow runs**: Did `pr-validate.yml` and `builder.yml` both complete and succeed?
3. **Check auto-merge logs**: The `Auto-merge if eligible` job in pr-validate.yml
4. **Check scoping**: Is validation failing for an unrelated app? Check if `fetch-depth: 0` is present and `git diff` is detecting changed apps correctly
5. **Check run names**: Do they match the filter patterns in the auto-merge code?
6. **Check for `action_required` runs**: bot PRs get blocked `pull_request` runs that never executed. Approving them replays a possibly-stale commit — prefer re-firing the `repository_dispatch`, or update the branch and let a fresh dispatch run.
7. **Manual intervention**: `gh pr merge <number> --squash`

## Managing Auto-Merge

Use the helper script to control auto-merge behavior on specific PRs:

```bash
# Check status
.github/scripts/manage-automerge.sh <pr-number> status

# Block auto-merge
.github/scripts/manage-automerge.sh <pr-number> block

# Unblock
.github/scripts/manage-automerge.sh <pr-number> unblock
```

## File Locations

- Validation workflow: `.github/workflows/pr-validate.yml`
- Builder workflow: `.github/workflows/builder.yml`
- Auto-merge helper: `.github/scripts/manage-automerge.sh`
- Update workflows: `.github/workflows/update-*.yml`
- Update scripts: `.github/scripts/update-*.sh`
