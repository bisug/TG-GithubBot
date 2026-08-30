# TG-GithubBot

TG-GithubBot is a Telegram bot that connects Telegram chats to GitHub repositories. It can create GitHub webhooks, send GitHub activity notifications to Telegram, and let connected GitHub users reply to issues or pull requests directly from Telegram.

## Features

- GitHub event notifications in Telegram chats.
- GitHub OAuth login with encrypted token storage.
- Automatic repository webhook creation from Telegram.
- Repository discovery with inline Telegram buttons.
- Per-repository event settings.
- Telegram replies posted back as GitHub issue or pull request comments.
- Reply commands for issue and PR actions:
  - `/close`
  - `/reopen`
  - `/approve`
- Admin-only repository management in groups.
- MongoDB persistence for users, chats, linked repositories, and webhook IDs.
- Docker Compose deployment with MongoDB included.

## How It Works

This bot uses a flexible, platform-agnostic architecture:

- **Telegram Commands**: The bot can receive commands via **Webhooks** (recommended for production VPS) or **Long Polling** (recommended for Cloud Web Services like Render/Heroku or local development). Switch modes using the `USE_POLLING` environment variable.
- **GitHub Activity**: Repository notifications are received via **Webhooks** for maximum performance.
- **Security**: All GitHub payloads are validated with a cryptographic signature (`GITHUB_WEBHOOK_SECRET`).

The `TELEGRAM_WEBHOOK_URL` is the public base URL used for GitHub OAuth, GitHub Webhooks, and (optionally) Telegram Webhooks.

> [!IMPORTANT]
> When `USE_POLLING=false` (default), the bot automatically registers its webhook with Telegram at startup. If you switch from Webhook to Polling mode, you may need to manually delete the webhook via the Telegram API if updates stop arriving.

## Supported GitHub Events

The bot supports a wide range of repository events. For a full technical list of compatible events, see the `internal/github/format.go` file. Common events include:

- **Push / Commit Activity**
- **Pull Requests** (Open, Close, Review, Approve)
- **Issues** (Open, Close, Comment)
- **Workflow Runs** (CI/CD Success/Failure)
- **Discussions** and **Releases**

## Requirements

- Go 1.26.3 or newer for manual builds.
- Docker and Docker Compose for recommended deployment.
- A Telegram bot token from [@BotFather](https://t.me/BotFather).
- A GitHub OAuth App.
- A public HTTPS URL for the bot.
- MongoDB. Docker Compose starts MongoDB automatically.

## Create The Telegram Bot

1. Open [@BotFather](https://t.me/BotFather) in Telegram.
2. Run `/newbot`.
3. Choose a bot name and username.
4. Copy the bot token.
5. Use that token as `TELEGRAM_TOKEN`.

Optional but recommended BotFather settings:

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

## Create The GitHub OAuth App

Create an OAuth App in GitHub:

```text
GitHub -> Settings -> Developer settings -> OAuth Apps -> New OAuth App
```

Use these values:

```text
Application name: TG-GithubBot
Homepage URL: https://your-domain.com
Authorization callback URL: https://your-domain.com/oauth/callback
```

After creating the OAuth App, copy:

- Client ID
- Client Secret

Use them as:

```dotenv
GITHUB_CLIENT_ID=...
GITHUB_CLIENT_SECRET=...
```

The bot requests these OAuth scopes:

| Scope | Rationale |
| :--- | :--- |
| `repo` | Full control of private/public repos. Required to create/delete repository webhooks and perform PR actions (Merge/Approve). |
| `admin:repo_hook` | Required to programmatically manage the webhooks that send notifications to Telegram. |
| `read:user` | Used to identify the GitHub user and link their account to their Telegram ID. |

## Environment Variables

Copy the sample file:

```bash
cp sample.env .env
```

On Windows PowerShell:

```powershell
Copy-Item sample.env .env
```

Edit `.env`:

```dotenv
# Telegram bot token from BotFather
TELEGRAM_TOKEN=123456:ABC-DEF...

# Public base URL with no trailing slash
TELEGRAM_WEBHOOK_URL=https://your-domain.com

# GitHub OAuth App credentials
GITHUB_CLIENT_ID=Iv1...
GITHUB_CLIENT_SECRET=...
GITHUB_WEBHOOK_SECRET=...

# HTTP server port (Use 10000 for Render, 8080 for Standard/Docker)
PORT=8080

# (Optional) Set to true for Polling (Best for Cloud/Local). Default: false (Webhooks)
USE_POLLING=false

# MongoDB connection string
MONGODB_URI=mongodb://localhost:27017

# MongoDB database name
DATABASE_NAME=github_bot

# Stable 32-byte hex key for encrypting stored tokens
ENCRYPTION_KEY=...
```

> [!CAUTION]
> **Security Warning**: `GITHUB_WEBHOOK_SECRET` must be a strong, random string. If left as default, unauthorized parties could inject fake GitHub activity into your Telegram chats.

Generate secure secrets:

```bash
openssl rand -hex 32
```

Use one generated value for `ENCRYPTION_KEY`. Use another generated value for `GITHUB_WEBHOOK_SECRET`.

PowerShell alternative for `ENCRYPTION_KEY`:

```powershell
-join ((1..32) | ForEach-Object { '{0:x2}' -f (Get-Random -Minimum 0 -Maximum 256) })
```

Important:

- `ENCRYPTION_KEY` must stay stable. If you change it, stored OAuth tokens and webhook chat tokens can no longer be decrypted.
- `TELEGRAM_WEBHOOK_URL` must be reachable by GitHub over HTTPS.
- Do not commit `.env`.

### Optional Button Custom Emoji

Telegram Bot API 9.4 supports custom emoji icons and visual button styles. Button styles are enabled automatically by the bot. Custom emoji icons are optional and can be configured with Telegram custom emoji IDs.

If these variables are not set, the bot still works and uses styled text buttons without custom emoji icons.

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

Use only the raw custom emoji ID as the value. Do not paste the emoji character itself.

## Deploy With Docker Compose

Docker Compose is the recommended deployment path.

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

4. Check logs:

   ```bash
   docker compose logs -f bot
   ```

5. Confirm the health page:

   ```text
   http://your-server-ip:8080
   ```

For production, put a reverse proxy such as Caddy, Nginx, Traefik, Cloudflare Tunnel, or a platform HTTPS proxy in front of port `8080`.

## Example Caddy Reverse Proxy

Install Caddy on your server and point your domain DNS record to the server.

Example `Caddyfile`:

```caddyfile
your-domain.com {
	reverse_proxy 127.0.0.1:8080
}
```

Then set:

```dotenv
TELEGRAM_WEBHOOK_URL=https://your-domain.com
```

Restart the bot after changing `.env`:

```bash
docker compose up -d --build
```

## Test Locally With A Tunnel

For local testing, expose port `8080` with a tunnel.

Using ngrok:

```bash
docker compose up -d --build
ngrok http 8080
```

Set `TELEGRAM_WEBHOOK_URL` to the HTTPS ngrok URL:

```dotenv
TELEGRAM_WEBHOOK_URL=https://example.ngrok-free.app
```

Update the GitHub OAuth App callback URL to:

```text
https://example.ngrok-free.app/oauth/callback
```

Restart the bot:

```bash
docker compose up -d --build
```

## Manual Run Without Docker

You need a running MongoDB instance first.

Example `.env` for local MongoDB:

```dotenv
MONGODB_URI=mongodb://localhost:27017
DATABASE_NAME=github_bot
PORT=8080
```

Install dependencies and run:

```bash
go mod download
go run ./cmd/bot
```

Build a binary:

```bash
go build -o bot ./cmd/bot
./bot
```

On Windows PowerShell:

```powershell
go build -o bot.exe ./cmd/bot
.\bot.exe
```

## First-Time Bot Setup

1. Start a private chat with your bot.
2. Send:

   ```text
   /start
   ```

3. Connect your GitHub account in private chat:

   ```text
   /connect
   ```

4. Add the bot to a group if you want group notifications.
5. In the target chat, link a repository:

   ```text
   /addrepo owner/repo
   ```

   Or browse your repositories:

   ```text
   /addrepo
   ```

6. Configure event settings:

   ```text
   /settings
   ```

Existing linked repositories keep their current GitHub webhook event settings. To move an old linked repo to wildcard event delivery, either use `/settings` and choose "Send me everything", or remove and re-add the repository.

## Commands

```text
/start - Start the bot
/help - Show help
/connect - Connect GitHub account in private chat
/addrepo [owner/repo] - Link a repository to the current chat
/removerepo [owner/repo] - Unlink a repository from the current chat
/repos - List linked repositories
/settings - Configure linked repository events
/privacy - Show privacy policy
/logout - Clear your stored GitHub token
/reload - Refresh group admin cache
/close - Close an issue or PR when replying to a bot notification
/reopen - Reopen an issue or PR when replying to a bot notification
/approve - Approve a PR when replying to a bot notification
```

## Permissions And Access Model

- `/connect` must be used in private chat.
- In groups, only Telegram admins with the **"Change Group Info"** permission can add/remove repositories or change settings.
- The GitHub user running `/addrepo` must have permission to create repository webhooks.
- Reply actions use the GitHub token of the Telegram user who sends the reply or command.
- OAuth tokens are encrypted with AES-GCM (256-bit) before being stored in MongoDB.

## Updating The Bot

Pull the latest code and restart:

```bash
git pull
docker compose up -d --build
```

Watch logs:

```bash
docker compose logs -f github-bot
```

## Deploy On Cloud Web Services (Render, Heroku, Koyeb, etc.)

Cloud platforms can host the bot as a **Web Service**. Because these platforms typically have ephemeral filesystems and may spin down during idle time, follow these universal steps for a stable deployment.

### 1. External MongoDB Requirement
Cloud web services do not persist local files. You **must** use an external MongoDB database:
- **MongoDB Atlas**: Recommended (has a generous free tier).
- **Platform Add-ons**: You can also use managed MongoDB add-ons provided by your platform.

Use the connection string as your `MONGODB_URI`.

### 2. Environment Variables
Configure these variables in your platform's dashboard:

| Variable | Description |
| :--- | :--- |
| `PORT` | Set to `10000` (Render) or `8080` (Standard). Most platforms provide this automatically. |
| `USE_POLLING` | Set to `true` for maximum reliability on free tiers. |
| `TELEGRAM_TOKEN` | Your bot token from @BotFather. |
| `TELEGRAM_WEBHOOK_URL` | The public URL of your service (e.g., `https://my-bot.koyeb.app`). |
| `GITHUB_CLIENT_ID` | From your GitHub OAuth App. |
| `GITHUB_CLIENT_SECRET` | From your GitHub OAuth App. |
| `GITHUB_WEBHOOK_SECRET` | A random secret for payload validation. |
| `ENCRYPTION_KEY` | 64 hex characters (32 bytes) for token security. |
| `MONGODB_URI` | Your external database connection string. |

### 3. The Polling Advantage
On cloud platforms (especially free tiers), setting `USE_POLLING=true` is highly recommended. It bypasses complex networking issues and ensures the bot starts receiving commands the moment the container wakes up.

### 4. GitHub OAuth Setup
Update your GitHub OAuth App's **Authorization callback URL** to match your public service URL:
`https://your-service-url.com/oauth/callback`

#### Summary for Cloud Users
- **Stay Awake**: Free tiers may sleep. Accessing the public URL or receiving a GitHub event will wake the bot.
- **Stable Encryption**: Never change your `ENCRYPTION_KEY` after deployment, or you will lose access to stored GitHub tokens.
- **Webhook Paths**: GitHub webhooks will be created automatically at `https://your-service.com/webhook/<token>`.

### Deploy On Render

The repo ships a Render Blueprint (`render.yaml`) that configures everything: Docker runtime, `/healthz` health check, port `10000`, and `USE_POLLING=true`.

1. Push this repo to GitHub/GitLab.
2. On [Render](https://render.com), click **New → Blueprint** and select the repo. Render reads `render.yaml`.
3. Fill in the secret values when prompted (`TELEGRAM_TOKEN`, `TELEGRAM_WEBHOOK_URL`, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GITHUB_WEBHOOK_SECRET`, `ENCRYPTION_KEY`, `MONGODB_URI`).
4. Set `TELEGRAM_WEBHOOK_URL` to your service URL, e.g. `https://tg-githubbot.onrender.com` (find it on the service page after the first deploy).
5. Update your GitHub OAuth App callback URL to `https://tg-githubbot.onrender.com/oauth/callback`.

Deploys are automatic on every push (`autoDeploy: true`). You can also trigger manual deploys from the dashboard.

### Deploy On Heroku

The repo includes `heroku.yml` (container stack, builds from the Dockerfile) and `app.json` (one-click **Deploy to Heroku** button with all env vars pre-wired).

Using the CLI:

```bash
heroku create your-app-name
heroku stack:set container
heroku config:set TELEGRAM_TOKEN=... TELEGRAM_WEBHOOK_URL=https://your-app-name.herokuapp.com \
  GITHUB_CLIENT_ID=... GITHUB_CLIENT_SECRET=... GITHUB_WEBHOOK_SECRET=... \
  ENCRYPTION_KEY=$(openssl rand -hex 32) MONGODB_URI=... USE_POLLING=true
git push heroku master
```

Or deploy with Docker directly:

```bash
heroku container:push web
heroku container:release web
```

Notes:
- Heroku injects `PORT` automatically; the bot reads it at runtime.
- `app.json` sets `USE_POLLING=true` by default — keep it on for reliability.
- Update your GitHub OAuth App callback URL to `https://your-app-name.herokuapp.com/oauth/callback`.

### Deploy On Koyeb

Koyeb builds the Dockerfile from your Git repo and exposes it on a public `*.koyeb.app` URL.

Using the CLI:

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

Or via the [Koyeb dashboard](https://app.koyeb.com): **Create App → GitHub →** select the repo, builder **Dockerfile**, port `8080`, then add the environment variables from the table above.

Notes:
- Set `TELEGRAM_WEBHOOK_URL` to your Koyeb public URL (shown on the service page).
- Update your GitHub OAuth App callback URL to `https://<your-koyeb-url>/oauth/callback`.
- Koyeb's free instance sleeps after inactivity; `USE_POLLING=true` makes wake-ups seamless.

## Backups

MongoDB stores linked chats, repository webhook IDs, and encrypted OAuth tokens.

Back up the Docker volume:

```bash
docker run --rm \
  -v tg-githubbot_mongo-data:/data/db \
  -v "$PWD:/backup" \
  alpine tar czf /backup/mongo-data-backup.tar.gz /data/db
```

Keep a copy of:

- `.env`
- MongoDB data
- `ENCRYPTION_KEY`

Without the original `ENCRYPTION_KEY`, encrypted tokens cannot be recovered.

## Troubleshooting

### Bot starts but `/connect` fails

Check:

- `TELEGRAM_WEBHOOK_URL` is correct.
- `TELEGRAM_WEBHOOK_URL` has no path. Use `https://your-domain.com`, not `https://your-domain.com/oauth/callback`.
- The GitHub OAuth App callback URL is exactly:

  ```text
  https://your-domain.com/oauth/callback
  ```

- The public URL reaches the bot.
- Your `ENCRYPTION_KEY` did not change between sending `/connect` and opening the GitHub callback.
- On Render free, open the Render service URL first if the service is asleep, then send `/connect` again.

If the browser shows `Invalid or expired state`, return to Telegram and run `/connect` again. OAuth links are intentionally short-lived.

### GitHub says webhook delivery failed

Check:

- Your domain uses HTTPS.
- The reverse proxy points to port `8080`.
- `GITHUB_WEBHOOK_SECRET` in `.env` matches the secret configured on the GitHub webhook.
- The bot logs show no signature validation errors.

### `/addrepo` fails with permissions error

The connected GitHub user must have admin rights on the repository because the bot creates repository webhooks.

### No events arrive after adding a repo

Check the repository webhook in GitHub:

```text
Repository -> Settings -> Webhooks
```

Confirm:

- Payload URL starts with `https://your-domain.com/webhook/`.
- Content type is `application/json`.
- Secret is set.
- The webhook is active.
- Recent deliveries show HTTP `200`.

### Group commands say you are not admin

Telegram admin status is cached. Run:

```text
/reload
```

Then retry the command.

## Development

Run tests:

```bash
go test ./... -count=1
```

Format code:

```bash
gofmt -w ./cmd ./internal
```

List packages:

```bash
go list ./...
```

## Project Layout

```text
cmd/bot/main.go                  Application entry point
internal/config                  Environment loading
internal/db                      MongoDB access
internal/cache                   In-memory TTL cache
internal/bot/commands            Telegram commands
internal/bot/callbacks           Telegram inline callbacks
internal/bot/middleware          Chat tracking middleware
internal/github                  OAuth, webhook parsing, event formatting
internal/models                  Shared data models
internal/utils                   Crypto and Telegram helpers
```

## Production Notes

- Run behind HTTPS.
- Keep `.env` private.
- Keep `ENCRYPTION_KEY` stable and backed up.
- Use a persistent MongoDB volume.
- Monitor `docker compose logs`.
- Rotate GitHub OAuth secrets and webhook secrets if leaked.
- Use a dedicated GitHub OAuth App for this bot.

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE).

## Support

Open an issue on GitHub with logs and reproduction steps.
