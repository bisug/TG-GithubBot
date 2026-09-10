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
- **Admin-cache key collision**: `adminCacheKey` packed `chatID` and `userID`
  with `<<32 |`, which deterministically collided for Telegram's wide 64-bit
  (often negative) group/user IDs — could leak admin status across users.
  Now FNV-1a-hashes both IDs; `IsAdmin` results are keyed correctly.
- **OAuth state replay**: `/oauth/callback` swapped a non-atomic
  `stateCache.Delete` for a single-use atomic gate. A `state` issued at
  `/connect` can now be redeemed exactly once; concurrent or replayed
  callbacks are rejected instead of double-exchanging the `code`.
- **Repo-search cache key**: pending `/addrepo` search prompts were keyed by
  Telegram message ID alone, which is only unique per chat — two chats could
  collide and consume each other's prompt. Keys are now `chatID:messageID`.
- **Corrupted PR action button labels**: replacement characters (U+FFFD) in
  the Merge/Close buttons rendered as garbage; restored proper icons.
- **`isMarkdownParseError` over-matching**: broad `button`/`markup`/`entities`
  substring checks misclassified unrelated 400s (e.g. `BUTTON_DATA_INVALID`)
  as markdown failures and hid the real error behind a plain-text retry.
  Tightened to concrete parse/markup signatures.

### Deferred
- Phase 6 digests (require an event storage layer that does not exist yet).
- GitHub App API clients (ghinstallation), installations registry, org-level
  webhook creation — endpoint groundwork landed; consumers documented in
  `.agents/github-app.md`.
