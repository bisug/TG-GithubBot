# GitHub App mode — design notes (Phase 5)

Status: DESIGN ONLY, not yet implemented.

## Why

Repository webhooks require each user to have admin access on the repo and
consume one webhook slot per repo per bot. A GitHub App installation:

- receives events for all repos the installation can access,
- gets higher API rate limits,
- can receive app-only events (installation, merge_group,
  repository_dispatch, workflow_dispatch, security_advisory, ...),
- supports org-level webhooks.

## Required pieces

1. **Dependency**: `github.com/bradleyfalzon/ghinstallation/v2`
   - `ghinstallation.NewAppsTransportKeyFromFile(http.Transport, appID, keyFile)`
     → app JWT client (list installations, etc.)
   - `ghinstallation.NewKeyFromFile(..., installationID, keyFile)`
     → per-installation client (auto-refreshing installation tokens)
2. **Config** (env):
   - `GITHUB_APP_ID` (int64)
   - `GITHUB_APP_PRIVATE_KEY` (path to PEM, or inline via `GITHUB_APP_PRIVATE_KEY_B64`)
   - `APP_WEBHOOK_SECRET` (separate from repo webhook secret)
3. **Endpoints**:
   - Existing `/webhook/{token}` keeps repo webhooks.
   - New `/app-webhook/{token}` (same token-in-path scheme) for app-level
     events. HMAC verified with `APP_WEBHOOK_SECRET`.
4. **Event routing**: app events mostly reuse existing formatters
   (installation, installation_repositories, etc. already have typed
   formatters). App events have no repository — route to a configured
   "admin chat" or chats linked to the installation account.
5. **Installation registry** (Mongo collection `installations`):
   `{installation_id, account_login, account_id, chat_ids[]}` so
   `installation_repositories` added/removed can fan out to linked chats.
6. **Org-level webhook creation**: when a GitHub App is configured, the
   add-repo flow can offer "link whole org" using the app installation
   instead of per-repo webhooks.

## Migration path

- Both modes can coexist: if `GITHUB_APP_ID` is unset, behavior is
  unchanged (pure repo-webhook mode).
- `/connect` continues to use user OAuth for user-scoped actions
  (approve/close/merge as the user). App tokens are for reading/creating
  webhooks, not acting as a user.

## Open questions

- Which chat receives app-level events when multiple chats link the same
  installation? (Current lean: all linked chats, deduped by delivery ID.)
- Should installation events create/remove chat links automatically?
