#!/bin/bash
# Helper script to manage auto-merge labels for PRs
# Usage: ./manage-automerge.sh <pr-number> <action>
# Actions: block, unblock, status

set -e

PR_NUMBER="${1}"
ACTION="${2:-status}"

if [ -z "$PR_NUMBER" ]; then
    echo "Usage: $0 <pr-number> <block|unblock|status>"
    echo ""
    echo "Actions:"
    echo "  block    - Add 'do-not-merge' label to prevent auto-merge"
    echo "  unblock  - Remove 'do-not-merge' label to allow auto-merge"
    echo "  status   - Show current PR status and labels"
    exit 1
fi

# Check if gh CLI is installed
if ! command -v gh &> /dev/null; then
    echo "Error: GitHub CLI (gh) is not installed"
    echo "Install it from: https://cli.github.com/"
    exit 1
fi

case "$ACTION" in
    block)
        echo "Blocking auto-merge for PR #${PR_NUMBER}..."
        gh pr edit "$PR_NUMBER" --add-label "do-not-merge"
        echo "✓ Added 'do-not-merge' label"
        gh pr view "$PR_NUMBER" --json labels,state,mergeable,statusCheckRollup
        ;;

    unblock)
        echo "Unblocking auto-merge for PR #${PR_NUMBER}..."
        gh pr edit "$PR_NUMBER" --remove-label "do-not-merge"
        echo "✓ Removed 'do-not-merge' label"
        gh pr view "$PR_NUMBER" --json labels,state,mergeable,statusCheckRollup
        ;;

    status)
        echo "PR #${PR_NUMBER} Status:"
        echo "======================="
        gh pr view "$PR_NUMBER" --json number,title,author,labels,state,isDraft,mergeable,statusCheckRollup \
            --template '
Number: #{{.number}}
Title: {{.title}}
Author: {{.author.login}}
State: {{.state}}
Draft: {{.isDraft}}
Mergeable: {{.mergeable}}

Labels:
{{- range .labels}}
  - {{.name}}
{{- end}}

Status Checks:
{{- range .statusCheckRollup}}
  - {{.context}}: {{.state}}
{{- end}}
'

        # Check auto-merge eligibility
        echo ""
        echo "Auto-merge Eligibility:"
        echo "======================="

        LABELS=$(gh pr view "$PR_NUMBER" --json labels --jq '.labels[].name')

        if echo "$LABELS" | grep -q "automated"; then
            echo "✓ Has 'automated' label"
        else
            echo "✗ Missing 'automated' label"
        fi

        if echo "$LABELS" | grep -q "validation-passed"; then
            echo "✓ Has 'validation-passed' label"
        else
            echo "✗ Missing 'validation-passed' label"
        fi

        if echo "$LABELS" | grep -qE "do-not-merge|needs-review|on-hold"; then
            echo "✗ Has blocking label (do-not-merge/needs-review/on-hold)"
        else
            echo "✓ No blocking labels"
        fi

        AUTHOR=$(gh pr view "$PR_NUMBER" --json author --jq '.author.login')
        if [ "$AUTHOR" = "github-actions[bot]" ]; then
            echo "✓ Created by github-actions[bot]"
        else
            echo "✗ Not created by github-actions[bot] (author: $AUTHOR)"
        fi
        ;;

    *)
        echo "Error: Unknown action '$ACTION'"
        echo "Valid actions: block, unblock, status"
        exit 1
        ;;
esac
