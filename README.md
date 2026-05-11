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

1. The bot runs a Telegram polling loop.
2. The same process also exposes an HTTP server.
3. GitHub OAuth redirects to:

   ```text
   https://your-domain.com/oauth/callback
   ```

4. GitHub repository webhooks post to:

   ```text
   https://your-domain.com/webhook/<encrypted-chat-token>
   ```

5. The bot validates GitHub webhook signatures, formats events, and sends them to the linked Telegram chat.

The bot does not need a Telegram webhook. `TELEGRAM_WEBHOOK_URL` is the public base URL for GitHub OAuth callbacks and GitHub webhooks.

## Supported GitHub Events

New repository webhooks are created with GitHub's wildcard subscription (`*`) so GitHub sends every event valid for that repository webhook.

The bot currently exposes and parses the event types supported by `go-github v85`, including:

- Branch protection events
- Check run and check suite events
- Code scanning alerts
- Commit comments
- Create and delete events
- Dependabot alerts
- Deploy keys
- Deployments, deployment statuses, deployment reviews, and deployment protection rules
- Discussions and discussion comments
- Forks
- GitHub App authorization events
- Wiki events
- Installation events
- Issue comments
- Issues
- Labels
- Marketplace purchase events
- Members and memberships
- Merge groups
- Milestones
- Organization events
- Packages and registry packages
- Page builds
- Projects v2 and project item events
- Pull requests
- Pull request reviews, review comments, and review threads
- Pushes
- Releases
- Repository changes, dispatches, imports, rulesets, and vulnerability alerts
- Secret scanning alerts
- Security advisories
- Sponsorships
- Stars and watches
- Commit statuses
- Teams and team repository access
- Workflow dispatches, workflow jobs, and workflow runs

Some event types only apply to organization, enterprise, marketplace, or GitHub App webhooks. GitHub decides which wildcard events are valid for the repository webhook that this bot creates.

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

```text
repo
admin:repo_hook
read:user
```

These are needed so a connected user can list repositories, create/delete repository webhooks, and perform issue/PR actions.

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

# Secret used to validate GitHub webhook payloads
GITHUB_WEBHOOK_SECRET=change_me_to_a_random_secret

# MongoDB connection
MONGODB_URI=mongodb://mongo:27017
DATABASE_NAME=github_bot

# 64 hex characters, generated from 32 random bytes
ENCRYPTION_KEY=...

# HTTP server port inside the container
PORT=8080
```

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
   docker compose logs -f github-bot
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
- In groups, only Telegram admins can add/remove repositories or change settings.
- The GitHub user running `/addrepo` must have permission to create repository webhooks.
- Reply actions use the GitHub token of the Telegram user who sends the reply or command.
- OAuth tokens are encrypted with AES-GCM before being stored in MongoDB.

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

## Deploy On Render Free Web Service

Render can host the bot as a free Web Service, but Render does not run `docker-compose.yml` for a Web Service and the free web service filesystem is ephemeral. Use an external MongoDB database such as MongoDB Atlas Free instead of the Compose MongoDB container.

Free Render services also spin down after idle time. That is acceptable for testing, but the first GitHub webhook or OAuth callback after spin-down can be delayed while the service wakes up.

### 1. Create A Free MongoDB Atlas Database

1. Create a MongoDB Atlas account.
2. Create a free M0 cluster.
3. Create a database user.
4. Add a network access rule.
   - For the simplest Render test deployment, allow access from anywhere: `0.0.0.0/0`.
   - For production, restrict database access as tightly as your hosting setup allows.
5. Copy the connection string.

It should look similar to:

```text
mongodb+srv://USERNAME:PASSWORD@cluster0.xxxxx.mongodb.net/?retryWrites=true&w=majority
```

Use this value as `MONGODB_URI`.

### 2. Push This Repository To GitHub

Render deploys from a Git provider or from a public Git URL. Commit and push your changes first:

```bash
git add .
git commit -m "Prepare Render deployment"
git push
```

### 3. Create The Render Web Service

Recommended path:

1. Open the Render Dashboard.
2. Click **New**.
3. Select **Blueprint** if you want Render to use the included [render.yaml](render.yaml).
4. Select this repository.
5. Review the service named `tg-githubbot`.
6. Enter the required secret environment variables when Render asks for them.
7. Deploy.

Manual path:

1. Open the Render Dashboard.
2. Click **New**.
3. Select **Web Service**.
4. Connect this GitHub repository.
5. Set **Language** to **Docker**.
6. Select the **Free** instance type.
7. Set the health check path to `/`.
8. Add the environment variables below.
9. Deploy.

### 4. Render Environment Variables

Set these in Render:

```dotenv
PORT=10000
DATABASE_NAME=github_bot
TELEGRAM_TOKEN=123456:ABC-DEF...
TELEGRAM_WEBHOOK_URL=https://your-render-service-name.onrender.com
GITHUB_CLIENT_ID=Iv1...
GITHUB_CLIENT_SECRET=...
GITHUB_WEBHOOK_SECRET=...
ENCRYPTION_KEY=...
MONGODB_URI=mongodb+srv://USERNAME:PASSWORD@cluster0.xxxxx.mongodb.net/?retryWrites=true&w=majority
```

Generate `ENCRYPTION_KEY` and `GITHUB_WEBHOOK_SECRET` locally:

```bash
openssl rand -hex 32
```

Use one generated value for `ENCRYPTION_KEY` and a different generated value for `GITHUB_WEBHOOK_SECRET`.

Important:

- `PORT` should be `10000` on Render.
- `TELEGRAM_WEBHOOK_URL` must be the public Render URL with no trailing slash.
- `MONGODB_URI` must point to MongoDB Atlas or another external MongoDB server.
- Keep `ENCRYPTION_KEY` stable. Changing it breaks stored OAuth tokens and webhook routing tokens.

### 5. Update The GitHub OAuth App

After Render gives you the service URL, update your GitHub OAuth App:

```text
Homepage URL: https://your-render-service-name.onrender.com
Authorization callback URL: https://your-render-service-name.onrender.com/oauth/callback
```

Also make sure Render has:

```dotenv
TELEGRAM_WEBHOOK_URL=https://your-render-service-name.onrender.com
```

Redeploy the Render service after changing environment variables.

### 6. Verify The Render Deployment

Open:

```text
https://your-render-service-name.onrender.com
```

You should see the bot health page.

Then check Render logs. You should see startup lines similar to:

```text
Bot started: @your_bot_username
Server listening on port 10000
```

### 7. Connect And Add A Repository

In Telegram:

```text
/start
/connect
```

After GitHub OAuth succeeds, add a repository in the target chat:

```text
/addrepo owner/repo
```

GitHub should create a webhook pointing to:

```text
https://your-render-service-name.onrender.com/webhook/<encrypted-chat-token>
```

### Render Free Limitations

- The bot can sleep after idle time.
- The first request after sleep can be slow.
- Do not store MongoDB data on the Render web service filesystem.
- Free web services are best for testing or hobby use, not production reliability.
- If the bot is asleep, Telegram polling is also stopped until an HTTP request wakes the service.

To wake it manually, open the Render URL in a browser.

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
