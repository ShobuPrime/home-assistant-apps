# Repository Automation

This repository uses GitHub Actions to automate addon updates, validation, and merging.

## Workflows

### 1. Portainer Version Updates

**Files:**
- [`.github/workflows/update-portainer-lts.yml`](workflows/update-portainer-lts.yml)
- [`.github/workflows/update-portainer-sts.yml`](workflows/update-portainer-sts.yml)

**Trigger:** Daily at 2 AM UTC (or manual via workflow_dispatch)

**What it does:**
1. Checks for new Portainer releases via GitHub API
2. Compares with current version in `config.yaml`
3. If update available:
   - Updates version in `config.yaml`, `build.yaml`, `Dockerfile`
   - Updates version references in `README.md` and `DOCS.md`
   - Prepends changelog to `CHANGELOG.md`
   - Creates a PR with detailed information

**Labels applied:** `automated`, `portainer`, `lts`/`sts`, `update`

### 2. PR Validation + Auto-merge

**File:** [`.github/workflows/pr-validate.yml`](workflows/pr-validate.yml)

**Trigger:** When a PR is opened, synchronized, or reopened

**Checks performed:**

#### Structure Validation
- Verifies all required files exist (`config.yaml`, `Dockerfile`, `README.md`, `DOCS.md`, `CHANGELOG.md`)
- Validates `config.yaml` has required fields (name, version, slug, description, arch)
- Validates version format follows semver (X.Y.Z)
- Checks Dockerfile has FROM instruction

#### Changelog Validation
- Ensures CHANGELOG.md is updated when addon files are modified
- Verifies CHANGELOG.md contains an entry for the current version

#### YAML Linting
- Runs yamllint on all `.yaml` and `.yml` files
- Enforces consistent formatting

#### Build Testing
- Validates Dockerfile syntax for changed addons

**On success:** Adds `validation-passed` label, then attempts auto-merge if eligible

**On failure:** Posts a comment with detailed errors

#### Primary Auto-merge (built into PR Validation)

After all validation jobs pass, an `auto-merge` job runs directly within the PR Validation workflow. This is the **primary merge path** and avoids the GitHub limitation where `GITHUB_TOKEN`-triggered label events cannot trigger other workflows.

The auto-merge job:
1. Checks the PR was created by `github-actions[bot]`
2. Verifies `automated` label is present and no blocking labels exist
3. Waits for the Builder workflow to complete successfully (polls for up to 10 minutes)
4. Verifies the PR is mergeable
5. Performs a squash merge

### Auto-merge Flow

```
Update Workflow (scheduled/manual)
    |
    v
Creates PR with 'automated' label (peter-evans/create-pull-request)
    |
    v
Fires repository_dispatch 'automated-pr-created' with PR details
    |
    +---> PR Validation runs (structure, changelog, YAML lint)
    |         |-- On success: adds 'validation-passed' label
    |         |-- Auto-merge job: polls for Builder completion, then merges
    |
    +---> Builder runs (Docker image build test)
```

**Why `repository_dispatch`?** PRs created via `peter-evans/create-pull-request` use
`GITHUB_TOKEN`, and GitHub suppresses downstream `pull_request` workflow triggers from
`GITHUB_TOKEN` events. The `repository_dispatch` event is explicitly exempted from this
restriction, so it reliably triggers both PR Validation and Builder workflows.

## Preventing Auto-merge

To prevent a PR from being auto-merged, add one of these labels:
- `do-not-merge` - Completely blocks merging
- `needs-review` - Indicates human review is required
- `on-hold` - Temporarily holds the PR

## Manual Triggering

You can manually trigger the update workflows:

1. Go to **Actions** tab
2. Select the workflow (e.g., "Update Portainer EE LTS")
3. Click **Run workflow**
4. Select branch and click **Run workflow**

## Customizing Validation

### Adding Custom Checks

Edit [`.github/workflows/pr-validate.yml`](workflows/pr-validate.yml) and add a new job:

```yaml
custom-check:
  name: My Custom Check
  runs-on: ubuntu-latest
  steps:
    - name: Checkout
      uses: actions/checkout@v4

    - name: Run check
      run: |
        # Your validation logic here
        echo "Running custom validation..."
```

Then add it to the `needs` array in the `summary` job.

### Modifying Auto-merge Behavior

In `pr-validate.yml`, modify the `auto-merge` job's conditions and script. Adjust Builder polling timeout via `maxAttempts` and `pollInterval`.

## Documentation Update Strategy

The update script (`.github/scripts/update-portainer.sh`) uses conservative regex patterns to update documentation:

**What gets updated:**
- "Currently running Portainer X.X.X" statements
- "running version X.X.X" statements
- Version badges/shields

**What does NOT get updated:**
- Section headers (e.g., "Portainer 2.33+ Ingress Compatibility")
- Generic "Portainer" references without version numbers
- Historical changelog entries

This prevents unintended modifications to documentation that references specific version requirements or compatibility notes.

## Troubleshooting

### Validation Failing

Check the PR comments for specific error messages. Common issues:
- Missing CHANGELOG.md entry
- Invalid version format
- YAML syntax errors

### Auto-merge Not Triggering

If PRs are not being merged:

1. **Check the PR Validation workflow**:
   - Look at the `Auto-merge if eligible` job in the PR Validation workflow run
   - Verify it ran and check its logs for skip reasons

2. **Common causes**:
   - PR missing `automated` or `validation-passed` label
   - Blocking label present (`do-not-merge`, `needs-review`, `on-hold`)
   - Builder workflow failed or hasn't completed
   - PR was not created by `github-actions[bot]`
   - PR has merge conflicts (not mergeable)

### Update Script Issues

If the update script fails:
1. Check GitHub API rate limits
2. Verify network connectivity in Actions
3. Check script logs in workflow run details

## Security Considerations

- Auto-merge only works for PRs created by `github-actions[bot]`
- Both Builder and PR Validation must pass before merge
- Validation checks prevent malformed addons from being merged
- Manual review can be forced with the `needs-review` label
- All workflows use `GITHUB_TOKEN` with minimal required permissions
