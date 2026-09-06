package commands

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github-webhook/internal/bot/ui"
	"github-webhook/internal/cache"
	"github-webhook/internal/config"
	"github-webhook/internal/db"
	gh "github-webhook/internal/github"
	"github-webhook/internal/models"
	"github-webhook/internal/utils"

	"html"
	"net/http"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/google/go-github/v90/github"
)

type CommandHandler struct {
	Config        *config.Config
	DB            *db.DB
	OAuth         *gh.OAuth
	StateCache    *cache.Cache[string, int64]
	ClientFactory *gh.ClientFactory
	EncryptionKey string
	ContextCache  *cache.Cache[string, models.MessageContext]
}

func NewCommandHandler(cfg *config.Config, database *db.DB, oauth *gh.OAuth, stateCache *cache.Cache[string, int64], factory *gh.ClientFactory, key string, ctxCache *cache.Cache[string, models.MessageContext]) *CommandHandler {
	return &CommandHandler{
		Config:        cfg,
		DB:            database,
		OAuth:         oauth,
		StateCache:    stateCache,
		ClientFactory: factory,
		EncryptionKey: key,
		ContextCache:  ctxCache,
	}
}

func requireAdminOrPrivate(b *gotgbot.Bot, ctx *ext.Context, deniedMessage string) error {
	if ctx.EffectiveChat != nil && ctx.EffectiveChat.Type == gotgbot.ChatTypePrivate {
		return nil
	}

	if ctx.EffectiveChat != nil && ctx.EffectiveUser != nil && utils.IsAdmin(b, ctx.EffectiveChat.Id, ctx.EffectiveUser.Id) {
		return nil
	}

	_, err := ctx.EffectiveMessage.Reply(b, deniedMessage, nil)
	return err
}

func (h *CommandHandler) Start(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := `<b>Welcome to the GitHub Bot!</b> 🤖

I can help you manage your GitHub repositories and notifications directly from Telegram.

<b>Get Started:</b>
1. Use /connect to link your GitHub account.
2. Use /addrepo to link a repository and start receiving notifications.
3. Use /settings to customize your notification preferences.

Need help? Type /help for a full list of commands.`
	_, err := ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (h *CommandHandler) Connect(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != gotgbot.ChatTypePrivate {
		_, err := ctx.EffectiveMessage.Reply(b, "⚠️ <b>/connect works only in a private chat with the bot.</b>\nOpen a direct chat and run /connect there.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return err
	}

	url, err := h.loginURLForUser(ctx.EffectiveUser.Id)
	if err != nil {
		return err
	}

	return h.replyWithConnectButton(b, ctx, "Connect your GitHub account to enable automatic webhook setup and actions like approving PRs.", url)
}

func (h *CommandHandler) loginURLForUser(userID int64) (string, error) {
	nonce, err := gh.GenerateState()
	if err != nil {
		return "", err
	}

	state, err := utils.Encrypt(fmt.Sprintf("%d:%d:%s", userID, time.Now().Unix(), nonce), h.EncryptionKey)
	if err != nil {
		return "", err
	}

	h.StateCache.Set(state, userID, 10*time.Minute)
	return h.OAuth.GetLoginURL(state), nil
}

func (h *CommandHandler) replyWithConnectButton(b *gotgbot.Bot, ctx *ext.Context, text string, url string) error {
	_, err := ctx.EffectiveMessage.Reply(b, text, &gotgbot.SendMessageOpts{
		ParseMode: "HTML",
		ReplyMarkup: ui.Markup(ui.Row(ui.URL("Connect GitHub", url,
			ui.WithStyle(ui.StylePrimary),
			ui.WithCustomEmojiEnv(ui.IconConnect),
		))),
	})
	return err
}

func settingsCallback(parts ...string) string {
	return strings.Join(parts, ":")
}

func addRepoPageCallback(page int) string {
	return settingsCallback("c", "ar", "pg", strconv.Itoa(page))
}

func addRepoIDCallback(repoID int64) string {
	return settingsCallback("c", "ar", "id", strconv.FormatInt(repoID, 10))
}

func repoSettingsCallback(link models.RepoLink) string {
	linkID := link.RepoFullName
	if link.WebhookID != 0 {
		linkID = strconv.FormatInt(link.WebhookID, 10)
	}
	return settingsCallback("c", "r", linkID)
}

func (h *CommandHandler) AddRepo(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := requireAdminOrPrivate(b, ctx, "Only admins can add repositories."); err != nil {
		return err
	}

	args := ctx.Args()
	if len(args) < 2 {
		return h.listUserRepos(b, ctx)
	}

	repoFullName := args[1]
	client, err := gh.GetClientForUser(context.Background(), h.DB, h.ClientFactory, ctx.EffectiveUser.Id, h.EncryptionKey)
	if err != nil {
		if errors.Is(err, gh.ErrUnauthorized) {
			url, urlErr := h.loginURLForUser(ctx.EffectiveUser.Id)
			if urlErr != nil {
				return urlErr
			}
			return h.replyWithConnectButton(b, ctx, fmt.Sprintf("Connect your GitHub account first to link repository <b>%s</b>.", html.EscapeString(repoFullName)), url)
		}
		_, _ = ctx.EffectiveMessage.Reply(b, "⚠️ <b>Authentication error.</b>\nPlease reconnect your GitHub account using /connect in a private chat.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return nil
	}
	owner, repo, ok := strings.Cut(repoFullName, "/")
	if !ok || owner == "" || repo == "" {
		_, _ = ctx.EffectiveMessage.Reply(b, "❌ <b>Invalid repository format.</b>\nUse <code>owner/repo</code>, for example <code>octocat/hello-world</code>.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return nil
	}

	// Verify repository existence
	_, _, getErr := client.Repositories.Get(context.Background(), owner, repo)
	if getErr != nil {
		if h.handleAuthError(b, ctx, getErr) {
			return nil
		}
		var errResp *github.ErrorResponse
		if errors.As(getErr, &errResp) && errResp.Response.StatusCode == http.StatusNotFound {
			_, _ = ctx.EffectiveMessage.Reply(b, "❌ <b>Repository not found.</b>\nPlease check the name and ensure you have access.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
			return nil
		}
		_, _ = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Error fetching repository: %v", getErr), nil)
		return nil
	}

	token, encErr := utils.Encrypt(fmt.Sprintf("%d", ctx.EffectiveChat.Id), h.EncryptionKey)
	if encErr != nil {
		_, _ = ctx.EffectiveMessage.Reply(b, "❌ <b>Error generating webhook token.</b> Please try again.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return nil
	}

	webhookURL := fmt.Sprintf("%s/webhook/%s", h.Config.TelegramWebhookURL, token)
	webhookConfig := &github.HookConfig{
		URL:         github.String(webhookURL),
		ContentType: github.String("json"),
		Secret:      github.String(h.Config.GitHubWebhookSecret),
	}

	hook := &github.Hook{
		Name:   github.String("web"),
		Events: []string{"*"},
		Config: webhookConfig,
		Active: github.Bool(true),
	}

	createdHook, _, hookErr := client.Repositories.CreateHook(context.Background(), owner, repo, hook)
	if hookErr != nil {
		if h.handleAuthError(b, ctx, hookErr) {
			return nil
		}
		var errResp *github.ErrorResponse
		if errors.As(hookErr, &errResp) && errResp.Response.StatusCode == http.StatusNotFound {
			safeRepoName := html.EscapeString(repoFullName)
			msg := fmt.Sprintf("❌ <b>Insufficient permissions.</b>\nYou need admin access to repository <b>%s</b> to create webhooks.", safeRepoName)
			_, err := ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
			return err
		}

		slog.Error("Webhook creation failed", "repo", repoFullName, "error", hookErr)
		msg := "⚠️ <b>Webhook creation failed.</b>\nPlease ensure you have admin rights and try again."
		_, err := ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return err
	}

	webhookID := createdHook.GetID()
	link := models.RepoLink{
		RepoFullName:    repoFullName,
		WebhookID:       webhookID,
		MessageThreadID: ctx.EffectiveMessage.MessageThreadId,
	}

	err = h.DB.AddRepoLink(context.Background(), ctx.EffectiveChat.Id, link)
	if err != nil {
		// The GitHub webhook already exists; if we fail to persist the link it
		// becomes an orphan the user cannot remove via the bot. Best-effort
		// delete so the repo is left clean.
		if _, delErr := client.Repositories.DeleteHook(context.Background(), owner, repo, webhookID); delErr != nil {
			slog.Error("Failed to clean up orphaned webhook after DB error", "repo", repoFullName, "hook_id", webhookID, "error", delErr)
		}
		_, err := ctx.EffectiveMessage.Reply(b, "❌ <b>Error linking repository.</b> Please try again.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return err
	}

	testMsg := " I asked GitHub to send a test webhook, so you should receive a test notification shortly."
	if err := gh.TriggerRepositoryHookTest(context.Background(), client, owner, repo, webhookID); err != nil {
		slog.Warn("Webhook test delivery failed", "repo", repoFullName, "hook_id", webhookID, "error", err)
		testMsg = fmt.Sprintf(" Linked successfully, but GitHub test delivery failed: %s", html.EscapeString(err.Error()))
	} else {
		slog.Info("Webhook test delivery requested", "repo", repoFullName, "hook_id", webhookID)
	}

	msg := fmt.Sprintf("✅ Repository <b>%s</b> linked successfully!", html.EscapeString(repoFullName))
	msg += testMsg
	_, err = ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (h *CommandHandler) listUserRepos(b *gotgbot.Bot, ctx *ext.Context) error {
	return h.sendRepoList(b, ctx, 1)
}

func (h *CommandHandler) sendRepoList(b *gotgbot.Bot, ctx *ext.Context, page int) error {
	client, err := gh.GetClientForUser(context.Background(), h.DB, h.ClientFactory, ctx.EffectiveUser.Id, h.EncryptionKey)
	if err != nil {
		if errors.Is(err, gh.ErrUnauthorized) {
			_, _ = ctx.EffectiveMessage.Reply(b, "Please /connect your GitHub account first to list repositories.", nil)
		} else {
			_, _ = ctx.EffectiveMessage.Reply(b, "⚠️ <b>Authentication error.</b>\nPlease reconnect your GitHub account using /connect in a private chat.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		}
		return nil
	}
	opts := &github.RepositoryListOptions{
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: github.ListOptions{PerPage: 10, Page: page},
	}

	repos, resp, err := client.Repositories.List(context.Background(), "", opts)
	if err != nil {
		if h.handleAuthError(b, ctx, err) {
			return nil
		}
		_, _ = ctx.EffectiveMessage.Reply(b, "❌ <b>Failed to fetch repositories from GitHub.</b> Please try again later.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return nil
	}

	if len(repos) == 0 && page == 1 {
		_, _ = ctx.EffectiveMessage.Reply(b, "No repositories found.", nil)
		return nil
	}

	var kb [][]gotgbot.InlineKeyboardButton
	for _, repo := range repos {
		kb = append(kb, ui.Row(ui.AddRepoButton(repo.GetFullName(), addRepoIDCallback(repo.GetID()))))
	}

	if navRow := ui.RepoPageNav(page, resp, addRepoPageCallback); len(navRow) > 0 {
		kb = append(kb, navRow)
	}

	_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Select a repository to add (Page %d):", page), &gotgbot.SendMessageOpts{
		ReplyMarkup: ui.Markup(kb...),
	})
	return err
}

func (h *CommandHandler) Settings(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := requireAdminOrPrivate(b, ctx, "Only admins can modify settings."); err != nil {
		return err
	}

	links, err := h.DB.GetChatLinks(context.Background(), ctx.EffectiveChat.Id)
	if err != nil {
		return err
	}

	if len(links) == 0 {
		_, err = ctx.EffectiveMessage.Reply(b, "No repositories linked yet. Use /addrepo to link one.", nil)
		return err
	}

	var kb [][]gotgbot.InlineKeyboardButton
	for _, l := range links {
		kb = append(kb, ui.Row(ui.RepoSettingsButton(l.RepoFullName, repoSettingsCallback(l))))
	}

	_, err = ctx.EffectiveMessage.Reply(b, "Select a repository to configure:", &gotgbot.SendMessageOpts{
		ReplyMarkup: ui.Markup(kb...),
	})
	return err
}

func (h *CommandHandler) RemoveRepo(b *gotgbot.Bot, ctx *ext.Context) error {
	if err := requireAdminOrPrivate(b, ctx, "Only admins can remove repositories."); err != nil {
		return err
	}

	args := ctx.Args()
	if len(args) < 2 {
		_, err := ctx.EffectiveMessage.Reply(b, "Usage: <code>/removerepo owner/repo</code>", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return err
	}

	repoFullName := args[1]
	link, err := h.DB.GetRepoLink(context.Background(), ctx.EffectiveChat.Id, repoFullName)
	if err != nil {
		_, err := ctx.EffectiveMessage.Reply(b, "❌ <b>Repository link not found.</b>\nUse /repos to see your linked repositories.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return err
	}

	var webhookStatusMsg string

	if link.WebhookID != 0 {
		client, err := gh.GetClientForUser(context.Background(), h.DB, h.ClientFactory, ctx.EffectiveUser.Id, h.EncryptionKey)
		if err != nil {
			if errors.Is(err, gh.ErrUnauthorized) {
				webhookStatusMsg = "\n\n⚠️ <b>Warning:</b> You are not connected to GitHub. The webhook could not be removed from the repository settings. Please remove it manually."
			} else {
				webhookStatusMsg = "\n\n⚠️ <b>Warning:</b> Could not decrypt your access token. Webhook not removed from GitHub."
			}
		} else {
			owner, repo, ok := strings.Cut(repoFullName, "/")
			if ok && owner != "" && repo != "" {
				_, err := client.Repositories.DeleteHook(context.Background(), owner, repo, link.WebhookID)
				if err != nil {
					if h.handleAuthError(b, ctx, err) {
						webhookStatusMsg = "\n\n⚠️ <b>Warning:</b> GitHub authentication failed. Webhook not removed."
					} else {
						var errResp *github.ErrorResponse
						if !errors.As(err, &errResp) || errResp.Response.StatusCode != http.StatusNotFound {
							webhookStatusMsg = fmt.Sprintf("\n\n⚠️ <b>Warning:</b> Failed to remove webhook from GitHub: %v", err)
						}
					}
				}
			}
		}
	}

	err = h.DB.RemoveRepoLink(context.Background(), ctx.EffectiveChat.Id, repoFullName)
	if err != nil {
		_, err := ctx.EffectiveMessage.Reply(b, "❌ <b>Error removing repository.</b> Please try again.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return err
	}

	_, err = ctx.EffectiveMessage.Reply(b, fmt.Sprintf("Repository <b>%s</b> removed successfully.%s", repoFullName, webhookStatusMsg), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (h *CommandHandler) Repos(b *gotgbot.Bot, ctx *ext.Context) error {
	links, err := h.DB.GetChatLinks(context.Background(), ctx.EffectiveChat.Id)
	if err != nil {
		return err
	}

	if len(links) == 0 {
		_, err = ctx.EffectiveMessage.Reply(b, "No repositories linked yet. Use /addrepo to link one.", nil)
		return err
	}

	var msg string
	for _, l := range links {
		msg += fmt.Sprintf("• <b>%s</b>\n", l.RepoFullName)
	}

	_, err = ctx.EffectiveMessage.Reply(b, "<b>Linked Repositories:</b>\n"+msg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
	return err
}

func (h *CommandHandler) Help(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := `<b>GitHub Bot Commands:</b>

<b>Account</b>
/connect - Link your GitHub account (<i>Must be used in private chat</i>)

<b>Repository Management</b>
/addrepo [owner/repo] - Link a repository
/removerepo [owner/repo] - Unlink a repository
/repos - List linked repositories
/close - Close an issue or PR (reply to notification).
/reopen - Reopen an issue or PR (reply to notification).
/approve - Approve a PR (reply to notification).

<b>Configuration</b>
/settings - Configure event notifications

<b>Need more help?</b>
Visit the <a href="https://github.com/bisug/TG-GithubBot">GitHub repository</a> for more details.`

	_, err := ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "HTML", LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true}})
	return err
}

func (h *CommandHandler) Privacy(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := `<b>Privacy Policy</b>

We value your privacy and are committed to protecting your data. This policy outlines how we collect, use, and safeguard your information.

<b>1. Data Collection</b>
• <b>Telegram Data:</b> We store your Telegram User ID, Chat ID, and basic profile information to route notifications and manage permissions.
• <b>GitHub Data:</b> When you connect your account, we securely store your encrypted OAuth token. We also store the names of repositories you link and the Webhook IDs created.
• <b>Events:</b> We process incoming GitHub webhook events (e.g., pushes, issues) to send notifications to your chat. The content of these events is processed in real-time and not permanently stored.

<b>2. Data Usage</b>
• <b>Functionality:</b> Your data is used strictly to provide the bot's services: sending notifications, managing repository links, and verifying permissions.
• <b>Security:</b> Your OAuth tokens are encrypted using AES-GCM before being stored in our database.

<b>3. Data Sharing</b>
• We do <b>not</b> share, sell, or rent your personal data to third parties.
• Data is only shared with GitHub APIs to the extent necessary to perform requested actions (e.g., creating webhooks).

<b>4. Data Control & Rights</b>
• <b>Disconnect:</b> You can unlink your GitHub account at any time, which invalidates the stored token.
• <b>Removal:</b> You can remove repositories using /removerepo. To request full data deletion, please contact the developer or simply block the bot.

<b>5. Contact</b>
If you have questions or concerns, please visit our <a href="https://github.com/bisug/TG-GithubBot">GitHub repository</a> or open an issue there.`

	_, err := ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "HTML", LinkPreviewOptions: &gotgbot.LinkPreviewOptions{IsDisabled: true}})
	return err
}

func (h *CommandHandler) Logout(b *gotgbot.Bot, ctx *ext.Context) error {
	err := h.DB.ClearUserToken(context.Background(), ctx.EffectiveUser.Id)
	if err != nil {
		_, err = ctx.EffectiveMessage.Reply(b, "❌ <b>Error logging out.</b> Please try again.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return err
	}
	_, err = ctx.EffectiveMessage.Reply(b, "✅ You have been logged out. Use /connect to reconnect.", nil)
	return err
}

func (h *CommandHandler) handleAuthError(b *gotgbot.Bot, ctx *ext.Context, err error) bool {
	var errResp *github.ErrorResponse
	if errors.As(err, &errResp) {
		// 401 always means the token is dead. 403 is ambiguous: it can be a
		// revoked token, but also a legitimate permission denial (e.g. approving
		// your own PR) or an abuse/rate limit — clearing the token on those would
		// force a pointless re-auth. Only clear on 401, or a 403 that explicitly
		// mentions bad credentials.
		if errResp.Response.StatusCode == http.StatusUnauthorized {
			_ = h.DB.ClearUserToken(context.Background(), ctx.EffectiveUser.Id)
			msg := "⚠️ <b>GitHub authentication failed.</b>\nIt seems your token has expired or was revoked. Please /connect again."
			_, _ = ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
			return true
		}
		if errResp.Response.StatusCode == http.StatusForbidden &&
			strings.Contains(strings.ToLower(errResp.Message), "bad credentials") {
			_ = h.DB.ClearUserToken(context.Background(), ctx.EffectiveUser.Id)
			msg := "⚠️ <b>GitHub authentication failed.</b>\nIt seems your token has expired or was revoked. Please /connect again."
			_, _ = ctx.EffectiveMessage.Reply(b, msg, &gotgbot.SendMessageOpts{ParseMode: "HTML"})
			return true
		}
	}
	return false
}

func (h *CommandHandler) Close(b *gotgbot.Bot, ctx *ext.Context) error {
	return h.handleIssueAction(b, ctx, "closed")
}

func (h *CommandHandler) Reopen(b *gotgbot.Bot, ctx *ext.Context) error {
	return h.handleIssueAction(b, ctx, "open")
}

func (h *CommandHandler) Approve(b *gotgbot.Bot, ctx *ext.Context) error {
	msg := ctx.EffectiveMessage
	if err := requireAdminOrPrivate(b, ctx, "Only admins can approve pull requests in this chat."); err != nil {
		return err
	}

	if msg.ReplyToMessage == nil {
		_, err := msg.Reply(b, "Please use this command in reply to a notification.", nil)
		return err
	}

	key := fmt.Sprintf("%d:%d", ctx.EffectiveChat.Id, msg.ReplyToMessage.MessageId)
	mContext, found := h.ContextCache.Get(key)
	if !found {
		_, err := msg.Reply(b, "This notification's context is no longer available (it may be older than 48 hours, or the bot restarted).\nUse the buttons on the notification, or open the PR on GitHub to act on it.", nil)
		return err
	}

	if mContext.Type != "pr" && mContext.Type != "pr_review" && mContext.Type != "pr_review_comment" {
		_, err := msg.Reply(b, "This command is only for Pull Requests.", nil)
		return err
	}

	client, err := h.getAuthenticatedClient(b, ctx)
	if err != nil {
		return nil
	}

	review := &github.PullRequestReviewRequest{
		Event: github.String("APPROVE"),
	}
	_, _, err = client.PullRequests.CreateReview(context.Background(), mContext.Owner, mContext.Repo, mContext.IssueNumber, review)

	if err != nil {
		if h.handleAuthError(b, ctx, err) {
			return nil
		}
		_, _ = msg.Reply(b, fmt.Sprintf("❌ <b>Failed to approve:</b> %v", err), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return nil
	}

	_, err = msg.Reply(b, fmt.Sprintf("✅ PR #%d approved.", mContext.IssueNumber), nil)
	return err
}

func (h *CommandHandler) handleIssueAction(b *gotgbot.Bot, ctx *ext.Context, state string) error {
	msg := ctx.EffectiveMessage
	if err := requireAdminOrPrivate(b, ctx, "Only admins can update issues or pull requests in this chat."); err != nil {
		return err
	}

	if msg.ReplyToMessage == nil {
		_, err := msg.Reply(b, "Please use this command in reply to a notification.", nil)
		return err
	}

	key := fmt.Sprintf("%d:%d", ctx.EffectiveChat.Id, msg.ReplyToMessage.MessageId)
	mContext, found := h.ContextCache.Get(key)
	if !found {
		_, err := msg.Reply(b, "This notification's context is no longer available (it may be older than 48 hours, or the bot restarted).\nUse the buttons on the notification, or open the item on GitHub to act on it.", nil)
		return err
	}

	client, err := h.getAuthenticatedClient(b, ctx)
	if err != nil {
		return nil
	}

	req := github.UpdateIssueRequest{State: github.Ptr(state)}
	_, _, err = client.Issues.Update(context.Background(), mContext.Owner, mContext.Repo, mContext.IssueNumber, req)

	if err != nil {
		if h.handleAuthError(b, ctx, err) {
			return nil
		}
		_, _ = msg.Reply(b, fmt.Sprintf("❌ <b>Failed to update the issue/PR:</b> %v", err), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return nil
	}

	action := "closed"
	if state == "open" {
		action = "reopened"
	}
	_, err = msg.Reply(b, fmt.Sprintf("✅ Issue/PR #%d %s.", mContext.IssueNumber, action), nil)
	return err
}

func (h *CommandHandler) getAuthenticatedClient(b *gotgbot.Bot, ctx *ext.Context) (*github.Client, error) {
	client, err := gh.GetClientForUser(context.Background(), h.DB, h.ClientFactory, ctx.EffectiveUser.Id, h.EncryptionKey)
	if err != nil {
		if errors.Is(err, gh.ErrUnauthorized) {
			url, urlErr := h.loginURLForUser(ctx.EffectiveUser.Id)
			if urlErr != nil {
				return nil, urlErr
			}
			_ = h.replyWithConnectButton(b, ctx, "Connect your GitHub account first.", url)
			return nil, fmt.Errorf("auth required")
		}
		_, _ = ctx.EffectiveMessage.Reply(b, "⚠️ <b>Authentication error.</b>\nPlease reconnect your GitHub account using /connect in a private chat.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		return nil, err
	}

	return client, nil
}
