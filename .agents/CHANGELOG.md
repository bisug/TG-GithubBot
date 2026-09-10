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
- **Admin check cache**: replaced the hand-rolled FNV-hashed `int64` cache
  (custom mutex + manual TTL sweep) with the shared `cache.Cache` keyed by a
  `{chatID, userID}` struct — eliminates packing collisions by construction
  and drops ~40 lines of bespoke machinery.
- **Repo-link lookups**: `GetRepoLink` / `GetRepoLinkByWebhookID` share one
  `findLink` scan; `UpdateRepoLinkName` evicts the cached links instead of
  hand-mutating the copy (readers repopulate from Mongo — no stale window).
- **GitHub error classification**: the 401 / 403-"bad credentials" token-death
  check and the 404 webhook check are now shared `github.IsInvalidTokenError`
  / `github.IsNotFoundError` helpers used by both commands and callbacks.
- **Command preambles**: `/approve`, `/merge`, `/close`, `/reopen` share a
  single `replyActionTarget` validation helper; the duplicated
  `lookupMessageContext` in `reply_handler.go` and `commands.go` is one
  function.
- **Callback data**: the `"*"` wildcard expansion and per-event toggle logic
  are shared helpers (`expandWildcardEvents`, `toggleEvent`); the enabled
  counter is counted in the render loop instead of a second pass.
- **Polling retry loops**: the two near-identical getUpdates retry loops in
  `main.go` share `runPollingLoop`.
- **Event formatters**: 7 repeated "escape each entry then join" loops use a
  generic `joinFormatted`; the four anonymous action-map + fallback dances
  (Member/Repository/Release/Team) use a named `actionInfo` lookup helper;
  `FormatRepositoryEvent("renamed")` no longer double-escapes the repo name
  that `FormatRepo` already escapes.
- **Webhook test/ping hooks**: `TriggerRepositoryHookTest`/`Ping` return the
  library error directly instead of the `if err != nil { return err }` dance.
- **Repo search reply**: consumes the pending-search entry atomically via
  `cache.Consume` (the `chatID != ctx.EffectiveChat.Id` re-check was
  tautological — the key already embeds the chat).
- **Chat upsert debounce**: uses atomic `cache.AddIfAbsent` instead of a
  check-then-set race window.

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
- **OAuth login failing after restart**: `/oauth/callback` consumed the
  used-state gate that is seeded in memory at `/connect` time; a restart
  between the two steps rejected every valid login ("state already used").
  `cache.ClaimSingleUse` now claims atomically (CAS issued→claimed, no
  delete/reinsert race) and claims-on-absent for states issued before a
  restart, binding each claim to the owning telegram ID.
- **Polling fallback 409 loop**: after a failed `SetAllBotWebhooks`, the
  fallback polling loop started without deleting an existing Telegram webhook,
  so every `getUpdates` failed with a 409 conflict forever. The webhook is now
  cleared (best-effort) before falling back.
- **DB errors reported as "not connected"**: `GetClientForUser` mapped any
  database failure to `ErrUnauthorized`, telling users to `/connect` during a
  MongoDB outage. Only `mongo.ErrNoDocuments` (or an empty stored token) means
  "not connected"; other errors propagate.
- **Webhook link-check misclassification**: `processEvent` logged database
  failures as Info "repository not linked", masking outages as unlink
  activity. DB failures are now logged distinctly as errors.
- **Channel-reply panic**: replying (with text) to a bot notification in a
  channel hit a nil `EffectiveUser` dereference in the comment-reply handler
  (recovered by the dispatcher, but the reply was lost). Guarded.
- **Webhook delivery dedup race**: the `X-GitHub-Delivery` seen-set used a
  non-atomic check-then-set; two racing redeliveries could both process.
  Now claimed atomically via `cache.AddIfAbsent`.
- **Mid-tag message truncation**: the 4096-rune cap cut HTML mid-tag/mid-entity
  and left tags unclosed, forcing Telegram to reject the message and triggering
  the plain-text fallback. `truncateTelegramHTML` now avoids tag/entity splits
  and closes tags left open by the cut.
- **Webhook token in logs**: the invalid-token rejection logged the full request
  path, which contains the chat's bearer token. Redacted (endpoint prefix only).

### Deferred
- Phase 6 digests (require an event storage layer that does not exist yet).
- GitHub App API clients (ghinstallation), installations registry, org-level
  webhook creation — endpoint groundwork landed; consumers documented in
  `.agents/github-app.md`.
