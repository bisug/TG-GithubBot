package callbacks

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"strconv"
	"strings"

	"github-webhook/internal/bot/commands"
	"github-webhook/internal/bot/ui"
	"github-webhook/internal/cache"
	"github-webhook/internal/config"
	"github-webhook/internal/db"
	"github-webhook/internal/github"
	"github-webhook/internal/models"
	"github-webhook/internal/utils"

	"net/http"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	gh "github.com/google/go-github/v90/github"
)

type CallbackHandler struct {
	Config        *config.Config
	DB            *db.DB
	ClientFactory *github.ClientFactory
	EncryptionKey string
	ActionCache   *cache.Cache[string, models.PRActionContext]
	// CommandHandler is set when the search flow is enabled; it owns the
	// ForceReply prompt + pending-search cache.
	CommandHandler *commands.CommandHandler
}

func (h *CallbackHandler) getClient(b *gotgbot.Bot, ctx *ext.Context) (*gh.Client, error) {
	client, err := github.GetClientForUser(context.Background(), h.DB, h.ClientFactory, ctx.EffectiveUser.Id, h.EncryptionKey)
	if err != nil {
		msg := "Authentication failed."
		if errors.Is(err, github.ErrUnauthorized) {
			msg = "Please /connect to GitHub first (private chat)."
		}
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: msg, ShowAlert: true})
		return nil, err
	}
	return client, nil
}

func NewCallbackHandler(cfg *config.Config, database *db.DB, factory *github.ClientFactory, key string, actionCache *cache.Cache[string, models.PRActionContext]) *CallbackHandler {
	return &CallbackHandler{
		Config:        cfg,
		DB:            database,
		ClientFactory: factory,
		EncryptionKey: key,
		ActionCache:   actionCache,
	}
}

// WithCommandHandler wires the command handler so callback flows (repo
// search) can reuse its message-based prompts.
func (h *CallbackHandler) WithCommandHandler(ch *commands.CommandHandler) *CallbackHandler {
	h.CommandHandler = ch
	return h
}

// Event aliases to compress callback data
var eventToShort = map[string]string{}
var shortToEvent = map[string]string{}

func init() {
	for _, e := range github.SupportedEvents {
		eventToShort[e.Name] = e.Short
		shortToEvent[e.Short] = e.Name
	}
}

const (
	cbPrefixSettings = "c"
	cbListRepos      = "ls"
	cbRepoMenu       = "r"
	cbAddRepo        = "ar"
	cbToggleEvent    = "te"
	cbBulkEvents     = "be"
	cbPresets        = "presets"
	cbIndividual     = "iev"
	cbStop           = "stop"
	cbStopConfirm    = "stopok"
	cbTestHook       = "test"
)

func cb(parts ...string) string {
	return strings.Join(parts, ":")
}

func cbRepo(action string, link *models.RepoLink, extra ...string) string {
	parts := append([]string{cbPrefixSettings, action, callbackLinkID(link)}, extra...)
	return cb(parts...)
}

func cbAddRepoPage(page int) string {
	return cb(cbPrefixSettings, cbAddRepo, "pg", strconv.Itoa(page))
}

func cbAddRepoID(repoID int64) string {
	return cb(cbPrefixSettings, cbAddRepo, "id", strconv.FormatInt(repoID, 10))
}

func (h *CallbackHandler) HandleSettings(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != gotgbot.ChatTypePrivate && !utils.IsAdmin(b, ctx.EffectiveChat.Id, ctx.EffectiveUser.Id) {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Only admins can change settings", ShowAlert: true})
		return nil
	}

	data := ctx.CallbackQuery.Data
	parts := strings.Split(data, ":")

	// c:ls -> conf:list
	// c:r -> conf:repo
	// c:te -> conf:toggle_evt
	// c:ep -> conf:evt_pg

	if len(parts) < 2 {
		slog.Warn("Ignoring malformed callback data", "data", data)
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid request."})
		return nil
	}

	prefix := parts[0] // c, conf
	action := parts[1] // ls, r, te, ep

	if prefix == cbPrefixSettings {
		if action == cbListRepos {
			return h.showRepoList(b, ctx)
		}
		if action == cbAddRepo {
			if len(parts) < 4 {
				slog.Warn("Ignoring malformed add-repo callback data", "data", data)
				_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid request."})
				return nil
			}
			subAction := parts[2]
			if subAction == "pg" {
				page, _ := strconv.Atoi(parts[3])
				return h.handleRepoPage(b, ctx, page)
			}
			if subAction == "id" {
				repoID, _ := strconv.ParseInt(parts[3], 10, 64)
				return h.handleAddRepoByID(b, ctx, repoID)
			}
			if subAction == "search" {
				return h.handleRepoSearch(b, ctx)
			}
		}

		if len(parts) < 3 {
			slog.Warn("Ignoring malformed settings callback data", "data", data)
			_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid request."})
			return nil
		}

		link, err := h.resolveRepoLink(context.Background(), ctx.EffectiveChat.Id, parts[2])
		if err != nil {
			_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Repo not found"})
			return nil
		}

		if action == cbRepoMenu {
			// c:r:linkID
			return h.showRepoMenu(b, ctx, link)
		}

		if action == cbToggleEvent && len(parts) >= 4 {
			// c:te:linkID:shortEvt:page
			shortEvt := parts[3]
			page := 1
			if len(parts) == 5 {
				page, _ = strconv.Atoi(parts[4])
			}

			evt, ok := shortToEvent[shortEvt]
			if !ok {
				evt = shortEvt
			}

			expanded := false
			hook, _, ok := h.setHookEvents(b, ctx, link, func(events []string) []string {
				// Expand the "*" wildcard so individual toggles are visible
				// and editable against concrete events.
				var currentEvents []string
				for _, e := range events {
					if e == "*" {
						expanded = true
						for _, se := range github.SupportedEvents {
							currentEvents = append(currentEvents, se.Name)
						}
						break
					}
					currentEvents = append(currentEvents, e)
				}

				found := false
				var newEvents []string
				for _, e := range currentEvents {
					if e == evt {
						found = true
					} else {
						newEvents = append(newEvents, e)
					}
				}
				if !found {
					newEvents = append(newEvents, evt)
				}
				return newEvents
			})
			if !ok {
				return nil
			}

			note := ""
			if expanded {
				note = "\n\n⚠️ The \"all events\" wildcard was expanded to a concrete event list; events added by GitHub in the future may need to be enabled manually."
			}
			return h.renderIndividualEvents(b, ctx, link, hook, page, note)
		} else if action == cbBulkEvents && len(parts) >= 4 {
			// c:be:linkID:mode (all|mute)
			return h.handleBulkEvents(b, ctx, link, parts[3])
		} else if action == cbPresets && len(parts) >= 3 {
			// c:presets:linkID:mode
			// mode: push, all
			if len(parts) < 4 {
				_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid request."})
				return nil
			}
			mode := parts[3]
			return h.handlePresets(b, ctx, link, mode)
		} else if action == cbIndividual && len(parts) == 4 {
			// c:iev:linkID:page
			page, _ := strconv.Atoi(parts[3])
			return h.showIndividualEvents(b, ctx, link, page)
		} else if action == cbStop {
			return h.showStopNotificationsConfirm(b, ctx, link)
		} else if action == cbStopConfirm {
			return h.handleStopNotifications(b, ctx, link)
		} else if action == cbTestHook {
			return h.handleTestHook(b, ctx, link)
		}
	}

	_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Unknown request."})
	return nil
}

func (h *CallbackHandler) resolveRepoLink(ctx context.Context, chatID int64, linkID string) (*models.RepoLink, error) {
	links, err := h.DB.GetChatLinks(ctx, chatID)
	if err != nil {
		return nil, err
	}

	if webhookID, err := strconv.ParseInt(linkID, 10, 64); err == nil && webhookID != 0 {
		for _, link := range links {
			if link.WebhookID == webhookID {
				return &link, nil
			}
		}
	}

	for _, link := range links {
		if link.RepoFullName == linkID {
			return &link, nil
		}
	}

	return nil, errors.New("link not found")
}

func callbackLinkID(l *models.RepoLink) string {
	if l.WebhookID != 0 {
		return strconv.FormatInt(l.WebhookID, 10)
	}
	return l.RepoFullName
}

func (h *CallbackHandler) showRepoMenu(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink) error {
	kb := h.repoMenuButtons(l)
	kb = append(kb, ui.Row(ui.BackButton(cb(cbPrefixSettings, cbListRepos))))

	_, _, err := ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:        fmt.Sprintf("Configuration for <b>%s</b>:", l.RepoFullName),
		ReplyMarkup: ui.Markup(kb...),
		ParseMode:   "HTML",
	})
	return err
}

func (h *CallbackHandler) showStopNotificationsConfirm(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink) error {
	kb := ui.Markup(
		ui.Row(ui.Callback("Stop notifications", cbRepo(cbStopConfirm, l),
			ui.WithStyle(ui.StyleDanger),
			ui.WithCustomEmojiEnv(ui.IconConfirm),
		)),
		ui.Row(ui.Callback("Cancel", cbRepo(cbRepoMenu, l),
			ui.WithStyle(ui.StylePrimary),
			ui.WithCustomEmojiEnv(ui.IconCancel),
		)),
	)

	_, _, err := ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:        fmt.Sprintf("Stop notifications for <b>%s</b> in this chat?", l.RepoFullName),
		ReplyMarkup: kb,
		ParseMode:   "HTML",
	})
	return err
}

func (h *CallbackHandler) handleStopNotifications(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink) error {
	warning := ""

	if l.WebhookID != 0 {
		client, err := github.GetClientForUser(context.Background(), h.DB, h.ClientFactory, ctx.EffectiveUser.Id, h.EncryptionKey)
		if err != nil {
			if errors.Is(err, github.ErrUnauthorized) {
				warning = "\n\nWarning: your GitHub account is not connected, so the GitHub webhook could not be removed automatically."
			} else {
				warning = "\n\nWarning: your GitHub token could not be decrypted, so the GitHub webhook was not removed automatically."
			}
		} else {
			parts := strings.Split(l.RepoFullName, "/")
			if len(parts) == 2 {
				_, err = client.Repositories.DeleteHook(context.Background(), parts[0], parts[1], l.WebhookID)
				if err != nil {
					if h.handleAuthError(b, ctx, err) {
						return nil
					}

					var errResp *gh.ErrorResponse
					if !errors.As(err, &errResp) || errResp.Response.StatusCode != http.StatusNotFound {
						warning = fmt.Sprintf("\n\nWarning: failed to remove the GitHub webhook automatically: %v", err)
					}
				}
			}
		}
	}

	if err := h.DB.RemoveRepoLink(context.Background(), ctx.EffectiveChat.Id, l.RepoFullName); err != nil {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Failed to update database.", ShowAlert: true})
		return nil
	}

	_, _, err := ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:      fmt.Sprintf("Notifications stopped for <b>%s</b>.%s", l.RepoFullName, warning),
		ParseMode: "HTML",
	})
	return err
}

// setHookEvents fetches the webhook, applies mutate to its event list, repairs
// the hook config (URL/secret) so stale webhook URLs are fixed on any save, and
// writes it back to GitHub. On failure the user is already notified and
// (nil, false) is returned.
func (h *CallbackHandler) setHookEvents(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink, mutate func(events []string) []string) (*gh.Hook, *gh.Client, bool) {
	client, err := h.getClient(b, ctx)
	if err != nil {
		return nil, nil, false
	}
	repoParts := strings.Split(l.RepoFullName, "/")
	if len(repoParts) != 2 {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid repository name."})
		return nil, nil, false
	}
	owner, repoName := repoParts[0], repoParts[1]

	hook, _, hErr := client.Repositories.GetHook(context.Background(), owner, repoName, l.WebhookID)
	if hErr != nil {
		if h.handleAuthError(b, ctx, hErr) {
			return nil, nil, false
		}
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Failed to fetch GitHub webhook settings.", ShowAlert: true})
		return nil, nil, false
	}

	hook.Events = github.FilterRepoHookEvents(mutate(hook.Events))

	chatToken, encErr := utils.Encrypt(fmt.Sprintf("%d", ctx.EffectiveChat.Id), h.EncryptionKey)
	if encErr != nil {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Error repairing webhook URL.", ShowAlert: true})
		return nil, nil, false
	}
	hook.Config = &gh.HookConfig{
		URL:         gh.String(fmt.Sprintf("%s/webhook/%s", h.Config.TelegramWebhookURL, chatToken)),
		ContentType: gh.String("json"),
		Secret:      gh.String(h.Config.GitHubWebhookSecret),
	}

	if _, _, editErr := client.Repositories.EditHook(context.Background(), owner, repoName, l.WebhookID, hook); editErr != nil {
		if h.handleAuthError(b, ctx, editErr) {
			return nil, nil, false
		}
		slog.Error("Failed to update GitHub webhook", "repo", l.RepoFullName, "hook_id", l.WebhookID, "chat", ctx.EffectiveChat.Id, "error", editErr)
		text := "Failed to update GitHub. Check that your connected account has Admin access to the repo."
		if isNotFoundErr(editErr) {
			text = "Failed to update GitHub. The webhook may have been removed, or your account lacks Admin access to the repo."
		}
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: text, ShowAlert: true})
		return nil, nil, false
	}
	return hook, client, true
}

func (h *CallbackHandler) handlePresets(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink, mode string) error {
	var newEvents []string
	var responseText string
	switch mode {
	case "push":
		newEvents = []string{"push"}
		responseText = "✅ <b>Success!</b> I've updated the repository settings to send <b>push events only</b>."
	case "all":
		newEvents = []string{"*"}
		responseText = "✅ <b>Success!</b> I've updated the repository settings to send <b>everything</b>."
	default:
		preset, ok := github.EventPresets[mode]
		if !ok {
			_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Unknown preset."})
			return nil
		}
		newEvents = preset.Events
		responseText = fmt.Sprintf("✅ <b>Success!</b> I've updated the repository settings to send <b>%s</b> events.", html.EscapeString(preset.Label))
	}

	_, client, ok := h.setHookEvents(b, ctx, l, func([]string) []string { return newEvents })
	if !ok {
		return nil
	}
	parts := strings.Split(l.RepoFullName, "/")

	if err := github.TriggerRepositoryHookPing(context.Background(), client, parts[0], parts[1], l.WebhookID); err != nil {
		slog.Warn("Webhook ping delivery failed", "repo", l.RepoFullName, "hook_id", l.WebhookID, "error", err)
		responseText += fmt.Sprintf("\n\n⚠️ GitHub ping delivery failed: %s", html.EscapeString(err.Error()))
	} else {
		slog.Info("Webhook ping delivery requested", "repo", l.RepoFullName, "hook_id", l.WebhookID)
		responseText += "\n\nWebhook URL and secret repaired. GitHub ping delivery requested; you should receive a ping notification shortly."
	}

	kb := ui.Markup(ui.Row(ui.BackButton(cbRepo(cbRepoMenu, l))))

	_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:        responseText,
		ReplyMarkup: kb,
		ParseMode:   "HTML",
	})
	return nil
}

// handleBulkEvents applies a bulk event change in a single GitHub API call:
// mode "all" enables every supported event (via the "*" wildcard), and mode
// "mute" disables all notifications while keeping the webhook alive
// (listening only for pings).
func (h *CallbackHandler) handleBulkEvents(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink, mode string) error {
	var newEvents []string
	var responseText string
	switch mode {
	case "all":
		newEvents = []string{"*"}
		responseText = "✅ <b>All events enabled.</b> GitHub will now send everything for this repository."
	case "mute":
		newEvents = []string{"ping"}
		responseText = "🔕 <b>All notifications muted.</b> The webhook is kept but only listens for pings. Use presets or \"Choose events\" to re-enable notifications."
	default:
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Unknown action."})
		return nil
	}

	if _, _, ok := h.setHookEvents(b, ctx, l, func([]string) []string { return newEvents }); !ok {
		return nil
	}
	slog.Info("Bulk event update applied", "repo", l.RepoFullName, "hook_id", l.WebhookID, "mode", mode, "chat", ctx.EffectiveChat.Id)

	kb := ui.Markup(
		ui.Row(ui.Callback("🔔 Unmute (all events)", cbRepo(cbBulkEvents, l, "all"),
			ui.WithStyle(ui.StyleSuccess),
			ui.WithCustomEmojiEnv(ui.IconAll),
		)),
		ui.Row(ui.Callback("Choose events", cbRepo(cbIndividual, l, "1"),
			ui.WithStyle(ui.StylePrimary),
			ui.WithCustomEmojiEnv(ui.IconChoose),
		)),
		ui.Row(ui.BackButton(cbRepo(cbRepoMenu, l))),
	)

	_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:        responseText,
		ReplyMarkup: kb,
		ParseMode:   "HTML",
	})
	return nil
}

func (h *CallbackHandler) showIndividualEvents(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink, page int) error {
	client, err := github.GetClientForUser(context.Background(), h.DB, h.ClientFactory, ctx.EffectiveUser.Id, h.EncryptionKey)
	if err != nil {
		if errors.Is(err, github.ErrUnauthorized) {
			_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{Text: "Error: You must be connected to GitHub to edit settings. Use /connect in a private chat."})
		} else {
			_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{Text: "Authentication failed. Please /connect again in a private chat."})
		}
		return nil
	}
	parts := strings.Split(l.RepoFullName, "/")
	if len(parts) != 2 {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid repository name."})
		return nil
	}

	hook, _, err := client.Repositories.GetHook(context.Background(), parts[0], parts[1], l.WebhookID)
	if err != nil {
		if h.handleAuthError(b, ctx, err) {
			return nil
		}
		_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{Text: "Error fetching webhook settings from GitHub. Check permissions."})
		return nil
	}

	return h.renderIndividualEvents(b, ctx, l, hook, page, "")
}

// renderIndividualEvents edits the message to show the per-event toggle
// keyboard built from the given (already-fetched) hook, avoiding an extra
// GitHub API round-trip after every toggle.
func (h *CallbackHandler) renderIndividualEvents(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink, hook *gh.Hook, page int, note string) error {
	parts := strings.Split(l.RepoFullName, "/")
	owner, repoName := parts[0], parts[1]

	enabledEvents := make(map[string]bool)
	if hook != nil {
		for _, e := range hook.Events {
			if e == "*" {
				for _, supported := range github.SupportedEvents {
					enabledEvents[supported.Name] = true
				}
				break
			}
			enabledEvents[e] = true
		}
	}

	var kb [][]gotgbot.InlineKeyboardButton
	var row []gotgbot.InlineKeyboardButton

	for _, e := range github.SupportedEvents {
		status := "❌"
		if enabledEvents[e.Name] {
			status = "✅"
		}

		cbData := cbRepo(cbToggleEvent, l, e.Short, strconv.Itoa(page))
		btnText := fmt.Sprintf("%s %s", status, e.Label)

		style := ui.StyleDanger
		if enabledEvents[e.Name] {
			style = ui.StyleSuccess
		}

		row = append(row, ui.Callback(btnText, cbData, ui.WithStyle(style)))

		if len(row) == 2 {
			kb = append(kb, row)
			row = []gotgbot.InlineKeyboardButton{}
		}
	}
	if len(row) > 0 {
		kb = append(kb, row)
	}

	// Bulk actions: one tap instead of toggling every event individually.
	enabledCount := 0
	for _, e := range github.SupportedEvents {
		if enabledEvents[e.Name] {
			enabledCount++
		}
	}
	bulkRow := []gotgbot.InlineKeyboardButton{
		ui.Callback("✅ Enable all", cbRepo(cbBulkEvents, l, "all"), ui.WithStyle(ui.StyleSuccess)),
		ui.Callback("🔕 Mute all", cbRepo(cbBulkEvents, l, "mute"), ui.WithStyle(ui.StyleDanger)),
	}
	kb = append([][]gotgbot.InlineKeyboardButton{bulkRow}, kb...)

	webhookSettingsURL := fmt.Sprintf("https://github.com/%s/%s/settings/hooks/%d", owner, repoName, l.WebhookID)
	kb = append(kb, ui.Row(ui.URL("Edit more on GitHub", webhookSettingsURL,
		ui.WithStyle(ui.StylePrimary),
		ui.WithCustomEmojiEnv(ui.IconGitHub),
	)))
	kb = append(kb, ui.Row(ui.BackButton(cbRepo(cbRepoMenu, l))))

	_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:        fmt.Sprintf("Individual Events for <b>%s</b> (%d/%d enabled). Tap to toggle:%s", html.EscapeString(l.RepoFullName), enabledCount, len(github.SupportedEvents), note),
		ReplyMarkup: ui.Markup(kb...),
	})
	return nil
}

func (h *CallbackHandler) showRepoList(b *gotgbot.Bot, ctx *ext.Context) error {
	links, err := h.DB.GetChatLinks(context.Background(), ctx.EffectiveChat.Id)
	if err != nil {
		return err
	}

	if len(links) == 0 {
		_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{Text: "No repositories linked. Use /addrepo first."})
		return nil
	}

	var kb [][]gotgbot.InlineKeyboardButton
	for _, l := range links {
		kb = append(kb, ui.Row(ui.RepoSettingsButton(l.RepoFullName, cbRepo(cbRepoMenu, &l))))
	}

	_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:        "Select a repository to configure:",
		ReplyMarkup: ui.Markup(kb...),
	})
	return nil
}

// handleRepoSearch answers the callback and sends the ForceReply prompt via
// the shared CommandHandler so the pending-search bookkeeping lives in one place.
func (h *CallbackHandler) handleRepoSearch(b *gotgbot.Bot, ctx *ext.Context) error {
	_, _ = ctx.CallbackQuery.Answer(b, nil)
	if h.CommandHandler == nil {
		_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{Text: "Search is unavailable here. Use /addrepo owner/repo directly."})
		return nil
	}
	return h.CommandHandler.PromptRepoSearch(b, ctx)
}

func (h *CallbackHandler) handleRepoPage(b *gotgbot.Bot, ctx *ext.Context, page int) error {
	client, err := h.getClient(b, ctx)
	if err != nil {
		return nil
	}
	opts := &gh.RepositoryListOptions{
		Sort:        "updated",
		Direction:   "desc",
		ListOptions: gh.ListOptions{PerPage: 10, Page: page},
	}

	repos, resp, err := client.Repositories.List(context.Background(), "", opts)
	if err != nil {
		if h.handleAuthError(b, ctx, err) {
			return nil
		}
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "GitHub API error.", ShowAlert: true})
		return nil
	}

	var kb [][]gotgbot.InlineKeyboardButton
	for _, repo := range repos {
		kb = append(kb, ui.Row(ui.AddRepoButton(repo.GetFullName(), cbAddRepoID(repo.GetID()))))
	}

	if navRow := ui.RepoPageNav(page, resp, cbAddRepoPage); len(navRow) > 0 {
		kb = append(kb, navRow)
	}

	_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:        fmt.Sprintf("Select a repository to add (Page %d):", page),
		ReplyMarkup: ui.Markup(kb...),
	})

	return nil
}

func (h *CallbackHandler) handleAddRepoByID(b *gotgbot.Bot, ctx *ext.Context, repoID int64) error {
	client, err := h.getClient(b, ctx)
	if err != nil {
		return nil
	}

	repo, _, err := client.Repositories.GetByID(context.Background(), repoID)
	if err != nil {
		if h.handleAuthError(b, ctx, err) {
			return nil
		}
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Repo not found or access denied.", ShowAlert: true})
		return nil
	}

	chatToken, encErr := utils.Encrypt(fmt.Sprintf("%d", ctx.EffectiveChat.Id), h.EncryptionKey)
	if encErr != nil {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Error generating webhook token.", ShowAlert: true})
		return nil
	}

	webhookURL := fmt.Sprintf("%s/webhook/%s", h.Config.TelegramWebhookURL, chatToken)
	webhookConfig := &gh.HookConfig{
		URL:         gh.String(webhookURL),
		ContentType: gh.String("json"),
		Secret:      gh.String(h.Config.GitHubWebhookSecret),
	}

	hook := &gh.Hook{
		Name:   gh.String("web"),
		Events: []string{"*"},
		Config: webhookConfig,
		Active: gh.Bool(true),
	}

	createdHook, _, hookErr := client.Repositories.CreateHook(context.Background(), repo.GetOwner().GetLogin(), repo.GetName(), hook)
	if hookErr != nil {
		if h.handleAuthError(b, ctx, hookErr) {
			return nil
		}
		msg := fmt.Sprintf("Webhook creation failed: %v. Check permissions", hookErr)
		_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{Text: msg, ParseMode: "HTML"})
		return nil
	}

	webhookID := createdHook.GetID()
	link := models.RepoLink{
		RepoFullName:    repo.GetFullName(),
		WebhookID:       webhookID,
		MessageThreadID: ctx.EffectiveMessage.MessageThreadId,
	}

	err = h.DB.AddRepoLink(context.Background(), ctx.EffectiveChat.Id, link)
	if err != nil {
		// The GitHub webhook already exists; if we fail to persist the link it
		// becomes an orphan the user cannot remove via the bot. Best-effort
		// delete so the repo is left clean.
		if _, delErr := client.Repositories.DeleteHook(context.Background(), repo.GetOwner().GetLogin(), repo.GetName(), webhookID); delErr != nil {
			slog.Error("Failed to clean up orphaned webhook after DB error", "repo", repo.GetFullName(), "hook_id", webhookID, "error", delErr)
		}
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Error linking repository."})
		return nil
	}

	slog.Info("Webhook test delivery requested", "repo", repo.GetFullName(), "hook_id", webhookID)
	if err := github.TriggerRepositoryHookTest(context.Background(), client, repo.GetOwner().GetLogin(), repo.GetName(), webhookID); err != nil {
		slog.Warn("Webhook test delivery failed", "repo", repo.GetFullName(), "hook_id", webhookID, "error", err)
	}

	// Reuse showRepoMenu to let user choose notifications immediately
	kb := h.repoMenuButtons(&link)

	msg := fmt.Sprintf("✅ Repository <b>%s</b> linked successfully!\n\nChoose what events to notify:", repo.GetFullName())
	_, _, _ = ctx.EffectiveMessage.EditText(b, &gotgbot.EditMessageTextOpts{
		Text:        msg,
		ReplyMarkup: ui.Markup(kb...),
		ParseMode:   "HTML",
	})
	return nil
}

func (h *CallbackHandler) repoMenuButtons(l *models.RepoLink) [][]gotgbot.InlineKeyboardButton {
	buttons := [][]gotgbot.InlineKeyboardButton{
		ui.Row(ui.Callback("Push only", cbRepo(cbPresets, l, "push"),
			ui.WithStyle(ui.StylePrimary),
			ui.WithCustomEmojiEnv(ui.IconPush),
		)),
		ui.Row(ui.Callback("All events", cbRepo(cbPresets, l, "all"),
			ui.WithStyle(ui.StyleSuccess),
			ui.WithCustomEmojiEnv(ui.IconAll),
		)),
	}

	// Presets, two per row.
	var presetRow []gotgbot.InlineKeyboardButton
	presetOrder := []string{"ci", "issues", "prs", "releases", "security"}
	for _, name := range presetOrder {
		preset, ok := github.EventPresets[name]
		if !ok {
			continue
		}
		presetRow = append(presetRow, ui.Callback(preset.Label, cbRepo(cbPresets, l, name)))
		if len(presetRow) == 2 {
			buttons = append(buttons, presetRow)
			presetRow = []gotgbot.InlineKeyboardButton{}
		}
	}
	if len(presetRow) > 0 {
		buttons = append(buttons, presetRow)
	}

	buttons = append(buttons,
		ui.Row(ui.Callback("Choose events", cbRepo(cbIndividual, l, "1"),
			ui.WithStyle(ui.StylePrimary),
			ui.WithCustomEmojiEnv(ui.IconChoose),
		)),
		ui.Row(ui.Callback("Test webhook", cbRepo(cbTestHook, l),
			ui.WithStyle(ui.StylePrimary),
			ui.WithCustomEmojiEnv(ui.IconGitHub),
		)),
		ui.Row(ui.Callback("Stop notifications", cbRepo(cbStop, l),
			ui.WithStyle(ui.StyleDanger),
			ui.WithCustomEmojiEnv(ui.IconStop),
		)),
	)
	return buttons
}

// handleTestHook asks GitHub to send a ping delivery to the repository webhook
// so the user can verify end-to-end delivery from the repo menu.
func (h *CallbackHandler) handleTestHook(b *gotgbot.Bot, ctx *ext.Context, l *models.RepoLink) error {
	if l.WebhookID == 0 {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "This link has no webhook ID; cannot test.", ShowAlert: true})
		return nil
	}

	client, err := h.getClient(b, ctx)
	if err != nil {
		return nil
	}

	owner, repo, ok := strings.Cut(l.RepoFullName, "/")
	if !ok {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid repository name.", ShowAlert: true})
		return nil
	}

	if err := github.TriggerRepositoryHookPing(context.Background(), client, owner, repo, l.WebhookID); err != nil {
		if h.handleAuthError(b, ctx, err) {
			return nil
		}
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: fmt.Sprintf("Test failed: %v", err), ShowAlert: true})
		return nil
	}

	slog.Info("Webhook ping requested from repo menu", "repo", l.RepoFullName, "hook_id", l.WebhookID)
	_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Ping sent! A test notification should arrive shortly.", ShowAlert: true})
	return nil
}

func (h *CallbackHandler) HandlePRAction(b *gotgbot.Bot, ctx *ext.Context) error {
	if ctx.EffectiveChat.Type != gotgbot.ChatTypePrivate && !utils.IsAdmin(b, ctx.EffectiveChat.Id, ctx.EffectiveUser.Id) {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Only admins can perform PR actions", ShowAlert: true})
		return nil
	}

	data := ctx.CallbackQuery.Data
	parts := strings.Split(data, ":") // act:approve:uuid

	if len(parts) != 3 {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Invalid request."})
		return nil
	}

	action := parts[1]
	actionID := parts[2]

	prContext, ok := h.ActionCache.Get(actionID)
	if !ok {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Action expired. Please open the PR link manually.", ShowAlert: true})
		return nil
	}

	owner := prContext.Owner
	repo := prContext.Repo
	prNum := prContext.PRNumber

	repoFullName := fmt.Sprintf("%s/%s", owner, repo)
	_, err := h.DB.GetRepoLink(context.Background(), ctx.EffectiveChat.Id, repoFullName)
	if err != nil {
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "This chat is not linked to the repo.", ShowAlert: true})
		return nil
	}

	client, err := h.getClient(b, ctx)
	if err != nil {
		return nil
	}
	ctxBg := context.Background()

	var msg string

	switch action {
	case "approve":
		_, _, err = client.PullRequests.CreateReview(ctxBg, owner, repo, prNum, &gh.PullRequestReviewRequest{Event: gh.String("APPROVE")})
		msg = "Approved!"
	case "close":
		state := "closed"
		_, _, err = client.PullRequests.Edit(ctxBg, owner, repo, prNum, &gh.PullRequest{State: &state})
		msg = "Closed!"
	default:
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "Unknown action."})
		return nil
	}

	if err != nil {
		if h.handleAuthError(b, ctx, err) {
			return nil
		}
		_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: fmt.Sprintf("Failed: %v", err), ShowAlert: true})
		return nil
	}

	_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: msg, ShowAlert: true})
	h.ActionCache.Delete(actionID)
	return nil
}

func (h *CallbackHandler) handleAuthError(b *gotgbot.Bot, ctx *ext.Context, err error) bool {
	var errResp *gh.ErrorResponse
	if errors.As(err, &errResp) {
		// 401 always means the token is dead. 403 is ambiguous (revoked token,
		// permission denial, rate limit) — only clear the token when GitHub
		// explicitly says the credentials are bad.
		if errResp.Response.StatusCode == http.StatusUnauthorized {
			_ = h.DB.ClearUserToken(context.Background(), ctx.EffectiveUser.Id)
			_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "GitHub auth error. Token revoked or expired.", ShowAlert: true})
			return true
		}
		if errResp.Response.StatusCode == http.StatusForbidden &&
			strings.Contains(strings.ToLower(errResp.Message), "bad credentials") {
			_ = h.DB.ClearUserToken(context.Background(), ctx.EffectiveUser.Id)
			_, _ = ctx.CallbackQuery.Answer(b, &gotgbot.AnswerCallbackQueryOpts{Text: "GitHub auth error. Token revoked or expired.", ShowAlert: true})
			return true
		}
	}
	return false
}

// isNotFoundErr reports whether err is a GitHub 404 API response. GitHub returns
// 404 both when the resource truly does not exist and, deliberately, when the
// connected account lacks permission to see/manage it (e.g. editing a webhook on
// a repo the account is not an admin of).
func isNotFoundErr(err error) bool {
	var errResp *gh.ErrorResponse
	return errors.As(err, &errResp) && errResp.Response.StatusCode == http.StatusNotFound
}
