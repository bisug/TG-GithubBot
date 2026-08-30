# TG-GithubBot

![Go](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-blue)
![Telegram](https://img.shields.io/badge/Telegram-Bot-26A5E4?logo=telegram&logoColor=white)
![MongoDB](https://img.shields.io/badge/DB-MongoDB-47A248?logo=mongodb&logoColor=white)

TG-GithubBot is a self-hosted Telegram bot that links GitHub repositories to Telegram chats. It delivers repository activity as formatted notifications, manages GitHub webhooks automatically, and lets connected users comment on, close, and approve issues and pull requests without leaving Telegram.

### One-Click Deploy

[![Deploy to Render](https://render.com/images/deploy-to-render-button.svg)](https://render.com/deploy?repo=https://github.com/bisug/TG-GithubBot)
[![Deploy to Heroku](https://www.herokucdn.com/deploy/button.svg)](https://www.heroku.com/deploy?template=https://github.com/bisug/TG-GithubBot)
[![Deploy to Koyeb](https://www.koyeb.com/static/images/deploy/button.svg)](https://app.koyeb.com/deploy?type=git&builder=docker&repository=github.com/bisug/TG-GithubBot&branch=master&name=tg-githubbot)

> [!NOTE]
> One-click deploys still require an external MongoDB (e.g. [MongoDB Atlas](https://www.mongodb.com/atlas)) and a GitHub OAuth App — see [Cloud Platforms](#cloud-platforms-render-heroku-koyeb).

## Table of Contents

- [Features](#features)
- [Tech Stack](#tech-stack)
- [How It Works](#how-it-works)
- [Requirements](#requirements)
- [Quick Start](#quick-start)
- [Setup](#setup)
  - [Create the Telegram Bot](#create-the-telegram-bot)
  - [Create the GitHub OAuth App](#create-the-github-oauth-app)
- [Configuration](#configuration)
- [Deployment](#deployment)
  - [Docker Compose](#docker-compose-recommended)
  - [Reverse Proxy](#reverse-proxy-example-caddy)
  - [Cloud Platforms](#cloud-platforms-render-heroku-koyeb)
  - [Local Testing](#local-testing-with-a-tunnel)
  - [Manual Build](#manual-build)
- [Usage](#usage)
  - [First-Time Setup](#first-time-setup)
  - [Commands](#commands)
  - [Permissions and Access Model](#permissions-and-access-model)
- [Supported GitHub Events](#supported-github-events)
- [Security](#security)
- [Backups](#backups)
- [Troubleshooting](#troubleshooting)
- [Development](#development)
- [Project Structure](#project-structure)
- [License](#license)

## Features

- **Real-time notifications** — pushes, pull requests, issues, CI/CD runs, releases, discussions, and 70+ other GitHub events delivered to Telegram.
- **GitHub OAuth login** — connect accounts with encrypted token storage (AES-256-GCM).
- **Automatic webhook management** — repository webhooks are created, updated, and removed from Telegram.
- **Repository discovery** — browse and link repositories with inline buttons.
- **Per-repository event settings** — choose exactly which events each chat receives.
- **Two-way interaction** — reply to notifications to comment on issues and PRs; use `/close`, `/reopen`, and `/approve` for quick actions.
- **Group-friendly** — admin-only repository management in group chats.
- **Persistent state** — MongoDB storage for users, chats, linked repositories, and webhook IDs.
- **Easy deployment** — Docker Compose with MongoDB included, plus blueprints for Render, Heroku, and Koyeb.

## Tech Stack

| Layer | Technology | Purpose |
| :--- | :--- | :--- |
| Language | [![Go](https://img.shields.io/badge/Go-1.27%2B-00ADD8?logo=go&logoColor=white)](https://go.dev) | Core application runtime. |
| Telegram framework | [![gotgbot](https://img.shields.io/badge/gotgbot-v2-26A5E4?logo=telegram&logoColor=white)](https://github.com/PaulSonOfLars/gotgbot) | Bot API client, commands, inline keyboards, long polling and webhooks. |
| GitHub API | [![go-github](https://img.shields.io/badge/go--github-v90-181717?logo=github&logoColor=white)](https://github.com/google/go-github) | Repository discovery, webhook management, issue/PR actions. |
| OAuth 2.0 | [![OAuth](https://img.shields.io/badge/OAuth-2.0-4285F4?logo=oauth&logoColor=white)](https://pkg.go.dev/golang.org/x/oauth2) | GitHub OAuth login flow. |
| Database | [![MongoDB](https://img.shields.io/badge/MongoDB-driver_v2-47A248?logo=mongodb&logoColor=white)](https://www.mongodb.com) | Persistence for users, chats, repo links, and webhook IDs. |
| Encryption | [![x/crypto](https://img.shields.io/badge/x--crypto-AES--256--GCM-00ADD8?logo=go&logoColor=white)](https://pkg.go.dev/golang.org/x/crypto) | Encrypting OAuth tokens at rest. |
| Formatting | [![html-to-markdown](https://img.shields.io/badge/html--to--markdown-v2-2088D1?logo=markdown&logoColor=white)](https://github.com/JohannesKaufmann/html-to-markdown) | Converting GitHub HTML content to Telegram MarkdownV2. |
| Config | [![godotenv](https://img.shields.io/badge/godotenv-1.5.1-8DD6F9?logo=dotenv&logoColor=black)](https://github.com/joho/godotenv) | Loading `.env` files. |
| Deployment | [![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?logo=docker&logoColor=white)](https://www.docker.com) | Containerized deployment with bundled MongoDB. |
| CI | [![GitHub Actions](https://img.shields.io/badge/GitHub-Actions-2088FF?logo=githubactions&logoColor=white)](https://github.com/features/actions) | Formatting checks, build, vet, and tests. |

## How It Works

- **Telegram updates** — received via long polling (`USE_POLLING=true`, recommended for cloud platforms and local development) or Telegram webhooks (`USE_POLLING=false`, recommended for a dedicated VPS).
- **GitHub activity** — always delivered via repository webhooks for real-time performance.
- **Security** — every GitHub payload is validated against an HMAC signature (`GITHUB_WEBHOOK_SECRET`).

`TELEGRAM_WEBHOOK_URL` is the public HTTPS base URL of the bot. It is used for the GitHub OAuth callback and GitHub webhook delivery, and for Telegram webhooks when polling is disabled. It must be reachable by GitHub even in polling mode.

> [!IMPORTANT]
> When `USE_POLLING=false`, the bot registers its Telegram webhook at startup. If you later switch to polling, you may need to delete the webhook manually via the Telegram API before updates arrive.

## Requirements

- **Telegram bot token** — from [@BotFather](https://t.me/BotFather).
- **GitHub OAuth App** — see [Setup](#create-the-github-oauth-app).
- **Public HTTPS URL** — required for GitHub webhooks and the OAuth callback.
- **MongoDB** — any instance; Docker Compose includes one.
- **Docker and Docker Compose** — for the recommended deployment.
- **Go 1.27 or newer** — only for manual builds.

## Quick Start

```bash
git clone https://github.com/bisug/TG-GithubBot.git
cd TG-GithubBot
cp sample.env .env   # then edit .env — see Configuration
docker compose up -d --build
```

Open a private chat with your bot, send `/start`, then `/connect` to link your GitHub account. Full walkthrough: [Usage](#usage).

## Setup

### Create the Telegram Bot

1. Open [@BotFather](https://t.me/BotFather) in Telegram.
2. Run `/newbot` and follow the prompts.
3. Copy the bot token and set it as `TELEGRAM_TOKEN`.

Optionally register the command list so Telegram autocompletes it:

```text
/setcommands
start - Start the bot
help - Show help
connect - Connect GitHub account
addrepo - Link a repository
removerepo - Unlink a repository
repos - List linked repositories
settings - Configure repository events
privacy - Show privacy policy
logout - Disconnect GitHub account
reload - Refresh admin cache
close - Close an issue or PR
reopen - Reopen an issue or PR
approve - Approve a PR
```

### Create the GitHub OAuth App

Create an OAuth App under **GitHub**, then **Settings**, **Developer settings**, **OAuth Apps**, **New OAuth App**, with:

| Field | Value |
| :--- | :--- |
| Application name | `TG-GithubBot` |
| Homepage URL | `https://your-domain.com` |
| Authorization callback URL | `https://your-domain.com/oauth/callback` |

Copy the **Client ID** and **Client Secret** into `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET`.

The bot requests these OAuth scopes:

| Scope | Purpose |
| :--- | :--- |
| `repo` | Access public and private repositories; required to manage webhooks and perform PR actions (approve/close/reopen). |
| `admin:repo_hook` | Create, update, and delete the repository webhooks that deliver notifications. |
| `read:user` | Identify the GitHub account and link it to a Telegram ID. |

## Configuration

Copy the sample environment file and edit it:

```bash
cp sample.env .env
```

```powershell
# Windows PowerShell
Copy-Item sample.env .env
```

### Environment Variables

| Variable | Required | Default | Description |
| :--- | :---: | :--- | :--- |
| `TELEGRAM_TOKEN` | Yes | — | Bot token from @BotFather. |
| `TELEGRAM_WEBHOOK_URL` | Yes | — | Public HTTPS base URL, no trailing slash. Used for the OAuth callback and GitHub webhooks. |
| `GITHUB_CLIENT_ID` | Yes | — | GitHub OAuth App client ID. |
| `GITHUB_CLIENT_SECRET` | Yes | — | GitHub OAuth App client secret. |
| `ENCRYPTION_KEY` | Yes | — | 64-character hex string (32 bytes) used to encrypt OAuth tokens. Must remain stable. |
| `MONGODB_URI` | Yes | — | MongoDB connection string. |
| `GITHUB_WEBHOOK_SECRET` | Recommended | — | Shared secret for validating GitHub webhook payloads. |
| `DATABASE_NAME` | No | `github_bot` | MongoDB database name. |
| `PORT` | No | `8080` | HTTP server port. Use `10000` on Render. |
| `USE_POLLING` | No | `false` | `true` receives Telegram updates via long polling. Docker Compose defaults this to `true`. |

### Generating Secrets

```bash
openssl rand -hex 32
```

Use one generated value for `ENCRYPTION_KEY` and another for `GITHUB_WEBHOOK_SECRET`.

PowerShell alternative:

```powershell
-join ((1..32) | ForEach-Object { '{0:x2}' -f (Get-Random -Minimum 0 -Maximum 256) })
```

> [!CAUTION]
> `GITHUB_WEBHOOK_SECRET` must be a strong, random string — a weak or default secret lets anyone inject fake GitHub activity into your chats.
> `ENCRYPTION_KEY` must never change after deployment; stored OAuth tokens cannot be recovered without the original key.

Never commit `.env` to version control.

### Optional: Button Custom Emoji

Telegram Bot API 9.4 supports custom emoji icons and visual button styles. Button styles are enabled automatically; custom emoji icons are optional and configured via custom emoji IDs. Without them, the bot renders styled text buttons.

```dotenv
TG_BUTTON_ICON_ADD=
TG_BUTTON_ICON_ALL=
TG_BUTTON_ICON_BACK=
TG_BUTTON_ICON_CANCEL=
TG_BUTTON_ICON_CHOOSE=
TG_BUTTON_ICON_CONFIRM=
TG_BUTTON_ICON_CONNECT=
TG_BUTTON_ICON_GITHUB=
TG_BUTTON_ICON_NEXT=
TG_BUTTON_ICON_PREVIOUS=
TG_BUTTON_ICON_PUSH=
TG_BUTTON_ICON_SETTINGS=
TG_BUTTON_ICON_STOP=
```

Use the raw custom emoji ID as the value — not the emoji character.

## Deployment

### Docker Compose (Recommended)

1. Clone the repository:

   ```bash
   git clone https://github.com/bisug/TG-GithubBot.git
   cd TG-GithubBot
   ```

2. Create and edit `.env`:

   ```bash
   cp sample.env .env
   nano .env
   ```

3. Start the bot and MongoDB:

   ```bash
   docker compose up -d --build
   ```

4. Verify:

   ```bash
   docker compose logs -f bot
   ```

   The health endpoint should respond at `http://your-server-ip:8080`.

For production, put a reverse proxy (Caddy, Nginx, Traefik, Cloudflare Tunnel, or a platform HTTPS proxy) in front of port `8080`.

### Reverse Proxy (Example: Caddy)

Install Caddy and point your domain's DNS record at the server.

`Caddyfile`:

```caddyfile
your-domain.com {
	reverse_proxy 127.0.0.1:8080
}
```

Set `TELEGRAM_WEBHOOK_URL=https://your-domain.com` and restart with `docker compose up -d --build`.

### Local Testing with a Tunnel

Expose port `8080` through a tunnel such as ngrok:

```bash
docker compose up -d --build
ngrok http 8080
```

Then:

1. Set `TELEGRAM_WEBHOOK_URL` to the HTTPS tunnel URL.
2. Update the GitHub OAuth App callback URL to `https://<tunnel-url>/oauth/callback`.
3. Restart the bot: `docker compose up -d --build`.

### Manual Build

Requires a running MongoDB instance and Go 1.27+.

```bash
go mod download
go run ./cmd/bot
```

Build a binary:

```bash
go build -o bot ./cmd/bot
./bot
```

```powershell
# Windows PowerShell
go build -o bot.exe ./cmd/bot
.\bot.exe
```

### Cloud Platforms (Render, Heroku, Koyeb)

#### General Cloud Setup

Cloud platforms host the bot as a web service, but they have ephemeral filesystems and may spin down when idle. Follow these rules for a stable deployment:

- **External MongoDB is required.** Use [MongoDB Atlas](https://www.mongodb.com/atlas) (generous free tier) or a managed add-on from your platform, and set its connection string as `MONGODB_URI`.
- **Use polling.** Set `USE_POLLING=true` for maximum reliability on free tiers — the bot starts receiving commands the moment the container wakes, with no extra networking setup.
- **Match the OAuth callback.** Point your GitHub OAuth App's authorization callback URL at `https://<your-service-url>/oauth/callback`.
- **Expect sleep on free tiers.** Accessing the public URL or receiving a GitHub event wakes the bot.
- **Never change `ENCRYPTION_KEY`** after deployment, or stored GitHub tokens are lost.
- **Webhook paths are automatic.** GitHub webhooks are created at `https://<your-service-url>/webhook/<token>`.

#### Render

The repo ships a Render Blueprint (`render.yaml`) that configures the Docker runtime, a `/healthz` health check, port `10000`, and `USE_POLLING=true`.

1. Push the repo to GitHub or GitLab.
2. On [Render](https://render.com), click **New**, then **Blueprint**, and select the repo.
3. Fill in the secrets when prompted (`TELEGRAM_TOKEN`, `TELEGRAM_WEBHOOK_URL`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GITHUB_WEBHOOK_SECRET`, `ENCRYPTION_KEY`, `MONGODB_URI`).
4. After the first deploy, set `TELEGRAM_WEBHOOK_URL` to the service URL, e.g. `https://tg-githubbot.onrender.com`.
5. Update the GitHub OAuth App callback URL to `https://tg-githubbot.onrender.com/oauth/callback`.

Deploys run automatically on every push; manual deploys can be triggered from the dashboard.

#### Heroku

The repo includes `heroku.yml` (container stack) and `app.json` (one-click **Deploy to Heroku** button with all env vars pre-wired).

```bash
heroku create your-app-name
heroku stack:set container
heroku config:set TELEGRAM_TOKEN=... TELEGRAM_WEBHOOK_URL=https://your-app-name.herokuapp.com \
  GITHUB_CLIENT_ID=... GITHUB_CLIENT_SECRET=... GITHUB_WEBHOOK_SECRET=... \
  ENCRYPTION_KEY=$(openssl rand -hex 32) MONGODB_URI=... USE_POLLING=true
git push heroku master
```

Or deploy the Docker image directly:

```bash
heroku container:push web
heroku container:release web
```

Heroku injects `PORT` automatically, and `app.json` defaults `USE_POLLING=true` — keep it on. Update the OAuth callback URL to `https://your-app-name.herokuapp.com/oauth/callback`.

#### Koyeb

Koyeb builds the Dockerfile from the Git repo and exposes a public `*.koyeb.app` URL.

```bash
koyeb app init tg-githubbot \
  --git github.com/<YOUR_USERNAME>/TG-GithubBot \
  --git-branch master \
  --git-builder docker \
  --ports 8080:http \
  --routes /:8080 \
  --env PORT=8080 \
  --env USE_POLLING=true \
  --env TELEGRAM_TOKEN=... \
  --env TELEGRAM_WEBHOOK_URL=https://tg-githubbot-<your-org>.koyeb.app \
  --env GITHUB_CLIENT_ID=... \
  --env GITHUB_CLIENT_SECRET=... \
  --env GITHUB_WEBHOOK_SECRET=... \
  --env ENCRYPTION_KEY=... \
  --env MONGODB_URI=...
```

Or via the [Koyeb dashboard](https://app.koyeb.com): **Create App**, choose **GitHub**, select the repo, builder **Dockerfile**, port `8080`, then add the environment variables listed above.

Set `TELEGRAM_WEBHOOK_URL` to the Koyeb public URL and update the OAuth callback URL to `https://<your-koyeb-url>/oauth/callback`. Koyeb's free instance sleeps after inactivity; polling makes wake-ups seamless.

## Usage

### First-Time Setup

1. Open a private chat with the bot and send `/start`.
2. Send `/connect` and complete the GitHub OAuth flow to link your account.
3. Add the bot to a group if you want notifications there.
4. In the target chat, link a repository:

   ```text
   /addrepo owner/repo
   ```

   Or send `/addrepo` without arguments to browse your repositories with inline buttons.

5. Fine-tune which events the chat receives with `/settings`.

> [!NOTE]
> Existing linked repositories keep their current webhook event settings. To switch a repository to wildcard delivery ("Send me everything"), use `/settings` or remove and re-add the repository.

### Commands

| Command | Where | Description |
| :--- | :--- | :--- |
| `/start` | Any chat | Start the bot. |
| `/help` | Any chat | Show help. |
| `/connect` | Private chat | Connect your GitHub account via OAuth. |
| `/addrepo [owner/repo]` | Any chat | Link a repository; without arguments, browse with inline buttons. |
| `/removerepo [owner/repo]` | Any chat | Unlink a repository. |
| `/repos` | Any chat | List repositories linked to this chat. |
| `/settings` | Any chat | Configure which events each linked repository delivers. |
| `/privacy` | Any chat | Show the privacy policy. |
| `/logout` | Private chat | Clear your stored GitHub token. |
| `/reload` | Groups | Refresh the cached group admin list. |
| `/close` | Reply to a notification | Close the linked issue or PR. |
| `/reopen` | Reply to a notification | Reopen the linked issue or PR. |
| `/approve` | Reply to a notification | Approve the linked PR. |

### Permissions and Access Model

- `/connect` and `/logout` work only in private chat.
- In groups, only Telegram admins with the **Change Group Info** permission can add or remove repositories and change settings.
- The GitHub account running `/addrepo` must have permission to create repository webhooks (repo admin).
- Reply actions (`/close`, `/reopen`, `/approve`, and comment replies) use the GitHub token of the Telegram user who sends them.
- OAuth tokens are encrypted with AES-256-GCM before being stored in MongoDB.

### Updating

```bash
git pull
docker compose up -d --build
docker compose logs -f bot
```

## Supported GitHub Events

The bot formats and delivers **78 GitHub webhook events**, including:

- **Push** — commits and branch updates
- **Pull requests** — open, close, review, approve, merge
- **Issues** — open, close, comment, label, milestone
- **CI/CD** — workflow runs and jobs, check suites, deployments
- **Community** — stars, forks, watches, releases, discussions
- **Security** — code scanning, Dependabot, and secret scanning alerts

For the complete list, see [`internal/github/events.go`](internal/github/events.go).

## Security

- OAuth tokens are encrypted at rest with AES-256-GCM; the key never leaves your `.env`.
- GitHub webhook payloads are validated with HMAC signatures (`GITHUB_WEBHOOK_SECRET`).
- Group administration is restricted to Telegram admins; reply actions run as the replying user's own GitHub token.

Operational checklist:

- Serve only behind HTTPS.
- Keep `.env` private and never commit it.
- Keep `ENCRYPTION_KEY` stable and backed up.
- Use a persistent MongoDB volume.
- Rotate GitHub OAuth and webhook secrets immediately if leaked.
- Use a dedicated GitHub OAuth App for this bot.

## Backups

MongoDB stores linked chats, repository webhook IDs, and encrypted OAuth tokens.

Back up the Docker volume (verify the exact name with `docker volume ls`):

```bash
docker run --rm \
  -v tg-githubbot_mongodb_data:/data/db \
  -v "$PWD:/backup" \
  alpine tar czf /backup/mongo-data-backup.tar.gz /data/db
```

Keep copies of `.env`, the MongoDB data, and `ENCRYPTION_KEY`. Without the original `ENCRYPTION_KEY`, encrypted tokens cannot be recovered.

## Troubleshooting

### `/connect` fails

- `TELEGRAM_WEBHOOK_URL` is correct and has no path — use `https://your-domain.com`, not `https://your-domain.com/oauth/callback`.
- The GitHub OAuth App callback URL is exactly `https://your-domain.com/oauth/callback`.
- The public URL actually reaches the bot.
- `ENCRYPTION_KEY` did not change between sending `/connect` and opening the callback.
- On Render free, open the service URL first to wake the instance, then retry.

If the browser shows `Invalid or expired state`, run `/connect` again — OAuth links are intentionally short-lived.

### GitHub says webhook delivery failed

- The domain uses HTTPS.
- The reverse proxy forwards to port `8080`.
- `GITHUB_WEBHOOK_SECRET` matches the secret configured on the GitHub webhook.
- The bot logs show no signature validation errors.

### `/addrepo` fails with a permissions error

The connected GitHub user must have admin rights on the repository — the bot creates repository webhooks.

### No events arrive after adding a repo

Check the webhook under **Repository**, then **Settings**, then **Webhooks**, and confirm:

- The payload URL starts with `https://your-domain.com/webhook/`.
- Content type is `application/json`.
- A secret is set.
- The webhook is active.
- Recent deliveries show HTTP `200`.

### Group commands say you are not admin

Telegram admin status is cached. Run `/reload` and retry.

## Development

```bash
go test ./... -count=1      # run tests
gofmt -w ./cmd ./internal   # format code
go vet ./...                # static analysis
```

CI runs formatting checks, build, vet, and tests on every push (see `.github/workflows/ci.yml`).

## Project Structure

```text
cmd/bot/main.go            Application entry point
internal/config            Environment loading
internal/db                MongoDB access
internal/cache             In-memory TTL cache
internal/bot/commands      Telegram commands
internal/bot/callbacks     Telegram inline callbacks
internal/bot/middleware    Chat tracking middleware
internal/github            OAuth, webhook parsing, event formatting
internal/models            Shared data models
internal/utils             Crypto and Telegram helpers
```

## License

Released under the MIT License. See [LICENSE](LICENSE).

## Support

Open a GitHub issue with logs and reproduction steps.
