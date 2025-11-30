# Main Branch Protection

## Policy
- Branch: `main`.
- Required status check: `CI / build-test` (job name from `.github/workflows/ci.yml`).
- Require branches to be up to date before merging.
- Require at least **1** approving review; dismiss stale reviews on new commits.
- Enforce admins; disallow force pushes and branch deletion.

## Apply via GitHub UI
1. Settings → Branches → “Add branch protection rule”.
2. Branch name pattern: `main`.
3. Enable “Require a pull request before merging” with min approvals = 1 and “Dismiss stale pull request approvals when new commits are pushed”.
4. Enable “Require status checks to pass before merging” and select `CI / build-test`; also tick “Require branches to be up to date before merging”.
5. Enable “Include administrators”.
6. Disable “Allow force pushes” and “Allow deletions”.
7. Save the rule.

## Apply via `gh api` (requires repo admin token)
```bash
OWNER="<your-github-username-or-org>"
REPO="tg_pay_gateway_bot"

cat <<'EOF' >/tmp/main-branch-protection.json
{
  "required_status_checks": {
    "strict": true,
    "contexts": ["CI / build-test"]
  },
  "enforce_admins": true,
  "required_pull_request_reviews": {
    "required_approving_review_count": 1,
    "dismiss_stale_reviews": true,
    "require_code_owner_review": false
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_linear_history": false,
  "block_creations": false,
  "required_conversation_resolution": true
}
EOF

gh api \
  -X PUT \
  -H "Accept: application/vnd.github+json" \
  -H "X-GitHub-Api-Version: 2022-11-28" \
  "repos/${OWNER}/${REPO}/branches/main/protection" \
  --input /tmp/main-branch-protection.json
```
