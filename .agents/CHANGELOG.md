# Changelog

All notable changes to TG-GithubBot. Format loosely follows Keep a Changelog.

## [Unreleased]

### Added
- `.agents/` implementation plan, changelog, and event availability matrix.
- Typed formatters for `issue_dependencies`, `repository_advisory`,
  `secret_scanning_scan`, `sub_issues` (go-github v90 has no payload structs
  for these; local structs + parse hook in `parseWebhookEvent`).
- Test-webhook button in the repo settings menu (triggers a GitHub ping).
- Repo search in the add-repo picker (ForceReply prompt, filters over
  fetched pages, up to 30 matches).
- `/start connect` deep-link behaving like `/connect` in private chats.
- Clickable repo/branch link in push notification headers.
- Merge action: 🔀 button on PR notifications and `/merge` command
  (reply to a PR notification within 48h).
- GitHub App webhook endpoint `/app-webhook/<token>` sharing the repo-webhook
  pipeline; optional `GITHUB_APP_ID`, `GITHUB_APP_PRIVATE_KEY`,
  `GITHUB_APP_WEBHOOK_SECRET` env vars (unset = pure repo-webhook mode).

### Removed
- Deprecated classic Projects events (`project`, `project_card`,
  `project_column`) from `SupportedEvents`.

### Changed
- Repo picker page size 5 → 10 to match `/repos` listing.
- `/repos` renders repository names as links with link previews disabled.

### Fixed
- (baseline e179653) 16 audit issues, markdown→HTML migration, low-priority
  polish. See git history for details.

### Deferred
- Phase 6 digests (require an event storage layer that does not exist yet).
- GitHub App API clients (ghinstallation), installations registry, org-level
  webhook creation — endpoint groundwork landed; consumers documented in
  `.agents/github-app.md`.
