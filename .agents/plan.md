# TG-GithubBot Implementation Plan

Phased plan to close every gap found in the events/features audit.
Each phase = one or more commits. Update `CHANGELOG.md` as phases land.

## Current state (baseline: e179653)

- 57 events in `SupportedEvents`; 47 have typed formatters, the rest fall back
  to the generic formatter.
- 4 events parseable only via the generic fallback (go-github v90 has no
  payload structs for them): `issue_dependencies`, `repository_advisory`,
  `secret_scanning_scan`, `sub_issues`.
- 3 dead entries (classic Projects, deprecated by GitHub, unparseable):
  `project`, `project_card`, `project_column`.
- 19 org/app-only events correctly blocked for repo hooks
  (`RepoHookForbiddenEvents`).
- UX gaps: no test-webhook button, `/repos` shows no status, no repo search,
  no onboarding deep-link, push header not clickable.
- No merge button, no GitHub App mode, no digests.

## Phase 1 — Typed formatters for the 4 generic-only events

go-github v90 cannot parse these, so define local payload structs and parse
them in `parseWebhookEvent` before the generic fallback. All four are
available on repository webhooks, so they are reachable in production.

| Event | Actions | Key payload fields |
|---|---|---|
| `issue_dependencies` | blocked_by_added, blocked_by_removed, blocking_added, blocking_removed | blocked_issue, blocking_issue, blocking_issue_repo |
| `repository_advisory` | published, reported | repository_advisory |
| `secret_scanning_scan` | (no action) | type, source, started_at, completed_at, secret_types, custom_pattern_name, custom_pattern_scope |
| `sub_issues` | parent_issue_added, parent_issue_removed, sub_issue_added, sub_issue_removed | parent_issue, parent_issue_repo, sub_issue |

Implementation:
1. New file `internal/github/typed_events.go` with payload structs + parse
   hook in `parseWebhookEvent`.
2. New formatters in `format.go` following existing patterns (emoji title,
   Repository/By rows, `FormatMessageWithButton`).
3. Wire into `formatMessage` switch in `webhooks.go`.
4. Unit tests for parse + format of each event.

Commit: "Add typed formatters for issue_dependencies, repository_advisory,
secret_scanning_scan, sub_issues"

## Phase 2 — Cleanup

1. Remove `project`, `project_card`, `project_column` from
   `SupportedEvents` (classic Projects is deprecated; payloads unparseable).
   Remove any orphaned formatters/labels.
2. Fix `handleRepoPage` PerPage 5 → 10 to match `sendRepoList`.

Commit: "Remove deprecated classic Projects events; align repo page size"

## Phase 3 — UX features

1. **Test Webhook button** in repo menu → `TriggerRepositoryHookPing` +
   callback answer.
2. **`/repos` status**: show 🔔/🔕 per repo (mute state) — derive from
   stored hook config where available.
3. **Repo search**: search flow in the add-repo picker (filter client-side
   over fetched pages; simplest robust option).
4. **Onboarding deep-link**: `/start` button linking to OAuth connect flow;
   after successful OAuth, auto-open repo picker.
5. **Push header link**: make repo name in push title clickable.

Commit per feature or one grouped commit.

## Phase 4 — Merge button

- Extend `withPRActionButtons` with ✅ Merge (`act:merge:uuid`).
- `HandlePRAction` merge case → `PullRequests.Merge`.
- Optional `/merge` command.

Commit: "Add merge action button for pull requests"

## Phase 5 — GitHub App mode (architectural)

- Add `bradleyfalzon/ghinstallation` dependency.
- Env: `GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY` (config + sample.env).
- App webhook endpoint for app-level events (installation, etc. —
  formatters already exist).
- Installation token management; org-level webhook creation option.

Detailed design in `github-app.md`. Implement only if session allows;
otherwise document and defer.

## Phase 6 — Digests (deferred)

Scheduled per-chat summary of stored events. Requires event storage layer
that does not exist yet. Defer with note.

## Event availability reference

Full matrix in `events-matrix.md` (repository/org/app/business/marketplace
availability per event, from GitHub webhook docs).
