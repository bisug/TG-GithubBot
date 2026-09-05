package github

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github-webhook/internal/bot/ui"
	"github-webhook/internal/cache"
	"github-webhook/internal/config"
	"github-webhook/internal/db"
	"github-webhook/internal/models"
	"github-webhook/internal/ratelimit"
	"github-webhook/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/go-github/v90/github"
)

const (
	maxWebhookPayloadBytes = 25 * 1024 * 1024
	webhookDBTimeout       = 5 * time.Second
)

var markdownLinkPattern = regexp.MustCompile(`\[([^\]]+)\]\(([^)]+)\)`)
var multipleNewlinesRegex = regexp.MustCompile(`\n{3,}`)

type WebhookServer struct {
	Config       *config.Config
	DB           *db.DB
	Bot          *gotgbot.Bot
	ContextCache *cache.Cache[string, models.MessageContext]  // Key: "chat_id:message_id"
	ActionCache  *cache.Cache[string, models.PRActionContext] // Key: UUID
	DeliverySeen *cache.Cache[string, struct{}]               // Key: X-GitHub-Delivery (idempotency)
	Pacer        *ratelimit.Pacer                             // paces outbound sendMessage calls per chat
	Wg           sync.WaitGroup
}

func NewWebhookServer(cfg *config.Config, database *db.DB, bot *gotgbot.Bot, ctxCache *cache.Cache[string, models.MessageContext], actionCache *cache.Cache[string, models.PRActionContext]) *WebhookServer {
	return &WebhookServer{
		Config:       cfg,
		DB:           database,
		Bot:          bot,
		ContextCache: ctxCache,
		ActionCache:  actionCache,
		DeliverySeen: cache.New[string, struct{}](),
		Pacer:        ratelimit.NewPacer(),
	}
}

func (s *WebhookServer) Handler(w http.ResponseWriter, r *http.Request) {
	// Path: /webhook/<token>
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookPayloadBytes)
	defer r.Body.Close()

	receivedAt := time.Now()
	var chatID int64
	eventType := github.WebHookType(r)
	deliveryID := r.Header.Get("X-GitHub-Delivery")
	hookIDHeader := r.Header.Get("X-GitHub-Hook-ID")
	path := r.URL.Path
	if strings.HasPrefix(path, "/webhook/") && len(path) > 9 {
		token := path[9:] // strip "/webhook/"
		decrypted, err := utils.Decrypt(token, s.Config.EncryptionKey)
		if err == nil {
			id, err := strconv.ParseInt(decrypted, 10, 64)
			if err == nil {
				chatID = id
				// slog.Debug("Decrypted chat ID from token", "chat", chatID)
			} else {
				slog.Error("Failed to parse decrypted token as int64", "error", err)
			}
		} else {
			slog.Error("Failed to decrypt webhook token", "error", err)
		}
	}

	if chatID == 0 {
		slog.Warn("Webhook rejected: invalid token", "event", eventType, "delivery", deliveryID, "hook_id", hookIDHeader, "remote", r.RemoteAddr, "path", r.URL.Path)
		http.Error(w, "Unauthorized: Token required", http.StatusUnauthorized)
		return
	}

	slog.Info("Webhook received", "event", eventType, "delivery", deliveryID, "hook_id", hookIDHeader, "chat", chatID, "remote", r.RemoteAddr)

	// Read and validate the body exactly once. ValidatePayloadFromBody consumes
	// r.Body and returns the payload; it verifies the GitHub HMAC signature only
	// when a secret (or a signature header) is present, so the dev-mode path with
	// no secret and no signature still yields the real payload. The previous code
	// called ValidatePayload (which drains the body) and then tried io.ReadAll on
	// the now-empty body in the no-secret branch, producing an empty payload.
	sig := r.Header.Get("X-Hub-Signature-256")
	if sig == "" {
		sig = r.Header.Get("X-Hub-Signature")
	}
	payload, err := github.ValidatePayloadFromBody(r.Header.Get("Content-Type"), r.Body, sig, []byte(s.Config.GitHubWebhookSecret))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			slog.Warn("Webhook rejected: payload too large", "event", eventType, "delivery", deliveryID, "hook_id", hookIDHeader, "chat", chatID, "limit", maxBytesErr.Limit)
			http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
			return
		}

		if s.Config.GitHubWebhookSecret == "" {
			slog.Error("Webhook REJECTED: signature present but GITHUB_WEBHOOK_SECRET is empty; cannot verify", "event", eventType, "delivery", deliveryID, "chat", chatID, "error", err)
		} else {
			slog.Error("Webhook REJECTED: signature mismatch (GitHub secret does not match GITHUB_WEBHOOK_SECRET)", "event", eventType, "delivery", deliveryID, "chat", chatID, "error", err)
		}
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	event, err := parseWebhookEvent(eventType, payload)
	if err != nil {
		slog.Warn("Webhook rejected: parse failed", "event", eventType, "delivery", deliveryID, "hook_id", hookIDHeader, "chat", chatID, "error", err)
		http.Error(w, "Parse error", http.StatusInternalServerError)
		return
	}

	var hookID int64
	if idStr := hookIDHeader; idStr != "" {
		hookID, _ = strconv.ParseInt(idStr, 10, 64)
	}

	if deliveryID != "" {
		if _, seen := s.DeliverySeen.Get(deliveryID); seen {
			slog.Info("Webhook duplicate delivery ignored", "event", eventType, "delivery", deliveryID, "chat", chatID)
			w.WriteHeader(http.StatusOK)
			return
		}
		s.DeliverySeen.Set(deliveryID, struct{}{}, 10*time.Minute)
	}

	s.Wg.Add(1)
	go func() {
		defer s.Wg.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				slog.Error("Webhook processing panic", "event", eventType, "delivery", deliveryID, "hook_id", hookID, "chat", chatID, "elapsed", time.Since(receivedAt).Round(time.Millisecond), "panic", recovered)
			}
		}()

		s.processEvent(event, chatID, hookID, eventType, deliveryID, receivedAt)
	}()
	w.WriteHeader(http.StatusOK)
}

func (s *WebhookServer) processEvent(event interface{}, chatID int64, hookID int64, eventType string, deliveryID string, receivedAt time.Time) {
	if e, ok := event.(*github.RepositoryEvent); ok && e.GetAction() == "renamed" {
		newFullName := e.GetRepo().GetFullName()
		if newFullName != "" && hookID != 0 {
			ctx, cancel := context.WithTimeout(context.Background(), webhookDBTimeout)
			err := s.DB.UpdateRepoLinkName(ctx, chatID, hookID, newFullName)
			cancel()
			if err != nil {
				slog.Warn("Failed to update repo name", "chat", chatID, "error", err)
			} else {
				slog.Info("Updated repo name", "repo", newFullName, "chat", chatID)
			}
		}
	}

	msg, markup := s.formatMessage(event)
	markup = s.withPRActionButtons(event, markup)
	if msg == "" {
		slog.Info("Webhook skipped: empty formatted message", "event", eventType, "delivery", deliveryID, "chat", chatID, "elapsed", time.Since(receivedAt).Round(time.Millisecond))
		return
	}

	msg = normalizeMessage(msg)

	// Telegram rejects messages over 4096 runes. The plain-text fallback already
	// truncates; cap here too so a long formatted event is not lost outright.
	const maxTelegramText = 4096
	if runes := []rune(msg); len(runes) > maxTelegramText {
		msg = string(runes[:maxTelegramText-1]) + "…"
	}

	var threadID int64
	if hookID != 0 {
		ctx, cancel := context.WithTimeout(context.Background(), webhookDBTimeout)
		link, err := s.DB.GetRepoLinkByWebhookID(ctx, chatID, hookID)
		cancel()
		if err == nil && link != nil {
			threadID = link.MessageThreadID
		}
	}

	sentMsg, err := s.sendEventMessage(chatID, threadID, msg, markup, eventType, deliveryID, receivedAt)
	if err != nil {
		return
	}

	s.storeMessageContext(sentMsg.MessageId, chatID, event)
	slog.Info("Webhook delivered", "chat", chatID, "event", eventType, "delivery", deliveryID, "message_id", sentMsg.MessageId, "elapsed", time.Since(receivedAt).Round(time.Millisecond))
}

// requestTimeout is how long a single sendMessage HTTP call to Telegram may run.
// The gotgbot default is 5s; we allow more headroom so genuine slow responses are
// not mistaken for failures under load.
const requestTimeout = 20 * time.Second

// maxSendAttempts caps how many times we retry a send that Telegram rate-limits
// (429) or that fails transiently. Chat-not-found and markdown parse errors are
// handled separately and never hit this loop the same way.
const maxSendAttempts = 3

// sendEventMessage paces, sends, and retries a webhook notification, choosing
// per-error-class behaviour:
//   - HTTP 429 (Too Many Requests): keep the markdown text, sleep for Telegram's
//     retry-after, then retry. We do NOT fall back to plain text here (that would
//     only add a second request while we are already being throttled).
//   - "chat not found" (400): the chat is unreachable/gone; give up to avoid
//     hammering a dead target.
//   - markdown parse error (400): fall back to plain text once.
func (s *WebhookServer) sendEventMessage(chatID, threadID int64, msg string, markup gotgbot.ReplyMarkup, eventType, deliveryID string, receivedAt time.Time) (*gotgbot.Message, error) {
	// Funnel every send through the per-chat pacer so bursts (a commit firing
	// check_run + check_suite + workflow_run + workflow_job + ... together) drain
	// at a safe rate instead of tripping Telegram's per-chat limit.
	s.Pacer.Wait(chatID, threadID)

	opts := &gotgbot.SendMessageOpts{
		ParseMode: "MarkdownV2",
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
			IsDisabled: true,
		},
		ReplyMarkup:     markup,
		MessageThreadId: threadID,
		RequestOpts:     &gotgbot.RequestOpts{Timeout: requestTimeout},
	}

	sent, err := s.sendMessageWithRetry(chatID, msg, opts, eventType, deliveryID, receivedAt)
	if err != nil {
		return nil, err
	}
	return sent, nil
}

// sendMessageWithRetry sends msg with the given opts, handling 429 (sleep +
// retry), transient failures (brief retry), chat-not-found (give up), and
// markdown parse errors (fall back to plain text once).
func (s *WebhookServer) sendMessageWithRetry(chatID int64, msg string, opts *gotgbot.SendMessageOpts, eventType, deliveryID string, receivedAt time.Time) (*gotgbot.Message, error) {
	var lastErr error
	for attempt := 1; attempt <= maxSendAttempts; attempt++ {
		sent, err := s.Bot.SendMessage(chatID, msg, opts)
		if err == nil {
			return sent, nil
		}
		lastErr = err

		if retryAfter, ok := isTooManyRequests(err); ok {
			if attempt < maxSendAttempts {
				slog.Warn("Telegram rate limited; sleeping", "retry_after", retryAfter.Round(time.Second), "chat", chatID, "event", eventType, "delivery", deliveryID, "attempt", attempt)
				time.Sleep(retryAfter)
				continue
			}
			slog.Error("Error sending message (rate limited, gave up)", "chat", chatID, "event", eventType, "delivery", deliveryID, "elapsed", time.Since(receivedAt).Round(time.Millisecond), "error", err)
			return nil, err
		}

		if isChatNotFound(err) {
			// Permanent: the chat no longer exists or the bot cannot reach it. Log
			// clearly so the operator removes the stale GitHub webhook.
			slog.Warn("Skipping unreachable chat: chat not found (remove the webhook for this chat)", "chat", chatID, "event", eventType, "delivery", deliveryID)
			return nil, err
		}

		if isMarkdownParseError(err) {
			slog.Warn("Markdown send failed; retrying plain text", "chat", chatID, "event", eventType, "delivery", deliveryID, "error", err)
			fallbackOpts := &gotgbot.SendMessageOpts{
				LinkPreviewOptions: opts.LinkPreviewOptions,
				ReplyMarkup:        opts.ReplyMarkup,
				MessageThreadId:    opts.MessageThreadId,
				RequestOpts:        opts.RequestOpts,
			}
			sent, fallbackErr := s.Bot.SendMessage(chatID, plainTextForTelegram(msg), fallbackOpts)
			if fallbackErr != nil {
				slog.Error("Error sending message", "chat", chatID, "event", eventType, "delivery", deliveryID, "elapsed", time.Since(receivedAt).Round(time.Millisecond), "error", err)
				return nil, fallbackErr
			}
			return sent, nil
		}

		// Generic/transient error (network, 5xx, ...): retry briefly with backoff.
		if attempt < maxSendAttempts {
			backoff := time.Duration(attempt) * time.Second
			slog.Warn("Transient send error; retrying", "backoff", backoff, "chat", chatID, "event", eventType, "delivery", deliveryID, "attempt", attempt, "error", err)
			time.Sleep(backoff)
			continue
		}
		slog.Error("Error sending message", "chat", chatID, "event", eventType, "delivery", deliveryID, "elapsed", time.Since(receivedAt).Round(time.Millisecond), "error", err)
		return nil, err
	}
	return nil, lastErr
}

// isTooManyRequests reports whether err is a Telegram HTTP 429 and returns the
// retry-after duration (defaulting to 2s if Telegram did not send one).
func isTooManyRequests(err error) (time.Duration, bool) {
	var te *gotgbot.TelegramError
	if !errors.As(err, &te) || te.Code != http.StatusTooManyRequests {
		return 0, false
	}
	if te.ResponseParams != nil && te.ResponseParams.RetryAfter > 0 {
		return time.Duration(te.ResponseParams.RetryAfter) * time.Second, true
	}
	return 2 * time.Second, true
}

// isChatNotFound reports whether err is Telegram's "Bad Request: chat not found".
func isChatNotFound(err error) bool {
	var te *gotgbot.TelegramError
	return errors.As(err, &te) && te.Code == http.StatusBadRequest &&
		strings.Contains(strings.ToLower(te.Description), "chat not found")
}

// isMarkdownParseError reports whether err is a Telegram 400 caused by markdown
// syntax that Telegram could not parse. Only these errors should fall back to
// plain text.
func isMarkdownParseError(err error) bool {
	var te *gotgbot.TelegramError
	if !errors.As(err, &te) || te.Code != http.StatusBadRequest {
		return false
	}
	d := strings.ToLower(te.Description)
	return strings.Contains(d, "can't parse") ||
		strings.Contains(d, "parse entities") ||
		strings.Contains(d, "entities") ||
		strings.Contains(d, "button") ||
		strings.Contains(d, "markup")
}

// normalizeMessage trims trailing spaces on each line, collapses 3+ consecutive newlines into 2
func normalizeMessage(s string) string {
	if s == "" {
		return s
	}

	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		lines[i] = strings.TrimRight(ln, " \t")
	}
	out := strings.Join(lines, "\n")

	out = multipleNewlinesRegex.ReplaceAllString(out, "\n\n")

	out = strings.TrimSpace(out)
	return out
}

func plainTextForTelegram(s string) string {
	s = markdownLinkPattern.ReplaceAllString(s, "$1 ($2)")
	replacer := strings.NewReplacer(
		"\\", "",
		"*", "",
		"_", "",
		"`", "",
		"~", "",
		"||", "",
	)
	s = replacer.Replace(s)
	const maxTelegramText = 4096
	runes := []rune(s)
	if len(runes) <= maxTelegramText {
		return s
	}
	return string(runes[:maxTelegramText-1]) + "…"
}

func (s *WebhookServer) storeMessageContext(messageID int64, chatID int64, event interface{}) {
	key := fmt.Sprintf("%d:%d", chatID, messageID)
	var ctx models.MessageContext

	switch e := event.(type) {
	case *github.PullRequestEvent:
		ctx = models.MessageContext{
			Owner:       e.GetRepo().GetOwner().GetLogin(),
			Repo:        e.GetRepo().GetName(),
			IssueNumber: e.GetPullRequest().GetNumber(),
			Type:        "pr",
		}
	case *github.IssuesEvent:
		ctx = models.MessageContext{
			Owner:       e.GetRepo().GetOwner().GetLogin(),
			Repo:        e.GetRepo().GetName(),
			IssueNumber: e.GetIssue().GetNumber(),
			Type:        "issue",
		}
	case *github.IssueCommentEvent:
		ctx = models.MessageContext{
			Owner:       e.GetRepo().GetOwner().GetLogin(),
			Repo:        e.GetRepo().GetName(),
			IssueNumber: e.GetIssue().GetNumber(),
			CommentID:   e.GetComment().GetID(),
			Type:        "issue_comment",
		}
	case *github.PullRequestReviewEvent:
		ctx = models.MessageContext{
			Owner:       e.GetRepo().GetOwner().GetLogin(),
			Repo:        e.GetRepo().GetName(),
			IssueNumber: e.GetPullRequest().GetNumber(),
			Type:        "pr_review",
		}
	case *github.PullRequestReviewCommentEvent:
		ctx = models.MessageContext{
			Owner:       e.GetRepo().GetOwner().GetLogin(),
			Repo:        e.GetRepo().GetName(),
			IssueNumber: e.GetPullRequest().GetNumber(),
			CommentID:   e.GetComment().GetID(),
			Type:        "pr_review_comment",
		}
	case *github.PullRequestTargetEvent:
		// withPRActionButtons covers this event, so keep reply context consistent.
		ctx = models.MessageContext{
			Owner:       e.GetRepo().GetOwner().GetLogin(),
			Repo:        e.GetRepo().GetName(),
			IssueNumber: e.GetPullRequest().GetNumber(),
			Type:        "pr",
		}
	default:
		return
	}

	s.ContextCache.Set(key, ctx, 48*time.Hour)
}

// prActionTarget returns the owner, repo and PR number for events that support
// inline PR actions (approve/close). ok is false for unsupported events.
func prActionTarget(event interface{}) (owner, repo string, prNum int, ok bool) {
	switch e := event.(type) {
	case *github.PullRequestEvent:
		return e.GetRepo().GetOwner().GetLogin(), e.GetRepo().GetName(), e.GetPullRequest().GetNumber(), true
	case *github.PullRequestReviewEvent:
		return e.GetRepo().GetOwner().GetLogin(), e.GetRepo().GetName(), e.GetPullRequest().GetNumber(), true
	case *github.PullRequestReviewCommentEvent:
		return e.GetRepo().GetOwner().GetLogin(), e.GetRepo().GetName(), e.GetPullRequest().GetNumber(), true
	case *github.PullRequestTargetEvent:
		return e.GetRepo().GetOwner().GetLogin(), e.GetRepo().GetName(), e.GetPullRequest().GetNumber(), true
	}
	return "", "", 0, false
}

// withPRActionButtons appends Approve/Close inline buttons to PR notifications and
// stores the corresponding action context so the callback handler can resolve it.
func (s *WebhookServer) withPRActionButtons(event interface{}, markup *gotgbot.InlineKeyboardMarkup) *gotgbot.InlineKeyboardMarkup {
	owner, repo, prNum, ok := prActionTarget(event)
	if !ok {
		return markup
	}

	id, err := GenerateState()
	if err != nil {
		slog.Warn("Failed to generate PR action id", "owner", owner, "repo", repo, "pr", prNum, "error", err)
		return markup
	}

	s.ActionCache.Set(id, models.PRActionContext{Owner: owner, Repo: repo, PRNumber: prNum}, 48*time.Hour)

	row := ui.Row(
		ui.Callback("✅ Approve", "act:approve:"+id, ui.WithStyle(ui.StyleSuccess)),
		ui.Callback("🔒 Close", "act:close:"+id, ui.WithStyle(ui.StyleDanger)),
	)

	if markup == nil {
		return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{row}}
	}

	markup.InlineKeyboard = append(markup.InlineKeyboard, row)
	return markup
}

func (s *WebhookServer) formatMessage(event interface{}) (msg string, markup *gotgbot.InlineKeyboardMarkup) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Formatter panic", "event_type", fmt.Sprintf("%T", event), "panic", r)
			msg, markup = "", nil
		}
	}()

	switch e := event.(type) {
	case *GenericWebhookEvent:
		return FormatGenericWebhookEvent(e)
	case *github.PushEvent:
		return FormatPushEvent(e)
	case *github.PullRequestEvent:
		return FormatPullRequestEvent(e)
	case *github.IssuesEvent:
		return FormatIssuesEvent(e)
	case *github.PingEvent:
		return FormatPingEvent(e)
	case *github.PullRequestReviewEvent:
		return FormatPullRequestReviewEvent(e)
	case *github.PullRequestReviewCommentEvent:
		return FormatPullRequestReviewCommentEvent(e)
	case *github.RepositoryEvent:
		return FormatRepositoryEvent(e)
	case *github.RepositoryDispatchEvent:
		return FormatRepositoryDispatchEvent(e)
	case *github.OrganizationEvent:
		return FormatOrganizationEvent(e)
	case *github.OrgBlockEvent:
		return FormatOrgBlockEvent(e)
	case *github.CheckRunEvent:
		return FormatCheckRunEvent(e)
	case *github.CheckSuiteEvent:
		return FormatCheckSuiteEvent(e)
	case *github.WorkflowRunEvent:
		return FormatWorkflowRunEvent(e)
	case *github.WorkflowJobEvent:
		return FormatWorkflowJobEvent(e)
	case *github.DeploymentEvent:
		return FormatDeploymentEvent(e)
	case *github.DeploymentStatusEvent:
		return FormatDeploymentStatusEvent(e)
	case *github.SecurityAdvisoryEvent:
		return FormatSecurityAdvisoryEvent(e)
	case *github.RepositoryVulnerabilityAlertEvent:
		return FormatRepositoryVulnerabilityAlertEvent(e)
	case *github.BranchProtectionRuleEvent:
		return FormatBranchProtectionRuleEvent(e)
	case *github.BranchProtectionConfigurationEvent:
		return FormatBranchProtectionConfigurationEvent(e)
	case *github.ContentReferenceEvent:
		return FormatContentReferenceEvent(e)
	case *github.CustomPropertyEvent:
		return FormatCustomPropertyEvent(e)
	case *github.CustomPropertyValuesEvent:
		return FormatCustomPropertyValuesEvent(e)
	case *github.CodeScanningAlertEvent:
		return FormatCodeScanningAlertEvent(e)
	case *github.DependabotAlertEvent:
		return FormatDependabotAlertEvent(e)
	case *github.DeploymentProtectionRuleEvent:
		return FormatDeploymentProtectionRuleEvent(e)
	case *github.DeploymentReviewEvent:
		return FormatDeploymentReviewEvent(e)
	case *github.DiscussionCommentEvent:
		return FormatDiscussionCommentEvent(e)
	case *github.DiscussionEvent:
		return FormatDiscussionEvent(e)
	case *github.GitHubAppAuthorizationEvent:
		return FormatGitHubAppAuthorizationEvent(e)
	case *github.InstallationRepositoriesEvent:
		return FormatInstallationRepositoriesEvent(e)
	case *github.InstallationTargetEvent:
		return FormatInstallationTargetEvent(e)
	case *github.MergeGroupEvent:
		return FormatMergeGroupEvent(e)
	case *github.PersonalAccessTokenRequestEvent:
		return FormatPersonalAccessTokenRequestEvent(e)
	case *github.ProjectV2Event:
		return FormatProjectV2Event(e)
	case *github.ProjectV2ItemEvent:
		return FormatProjectV2ItemEvent(e)
	case *github.PullRequestReviewThreadEvent:
		return FormatPullRequestReviewThreadEvent(e)
	case *github.PullRequestTargetEvent:
		return FormatPullRequestTargetEvent(e)
	case *github.RegistryPackageEvent:
		return FormatRegistryPackageEvent(e)
	case *github.RepositoryImportEvent:
		return FormatRepositoryImportEvent(e)
	case *github.RepositoryRulesetEvent:
		return FormatRepositoryRulesetEvent(e)
	case *github.SecretScanningAlertEvent:
		return FormatSecretScanningAlertEvent(e)
	case *github.SecretScanningAlertLocationEvent:
		return FormatSecretScanningAlertLocationEvent(e)
	case *github.SecurityAndAnalysisEvent:
		return FormatSecurityAndAnalysisEvent(e)
	case *github.SponsorshipEvent:
		return FormatSponsorshipEvent(e)
	case *github.UserEvent:
		return FormatUserEvent(e)
	case *github.MembershipEvent:
		return FormatMembershipEvent(e)
	case *github.MilestoneEvent:
		return FormatMilestoneEvent(e)
	case *github.CommitCommentEvent:
		return FormatCommitCommentEvent(e)
	case *github.ForkEvent:
		return FormatForkEvent(e)
	case *github.ReleaseEvent:
		return FormatReleaseEvent(e)
	case *github.StarEvent:
		return FormatStarEvent(e)
	case *github.WatchEvent:
		return FormatWatchEvent(e)
	case *github.LabelEvent:
		return FormatLabelEvent(e)
	case *github.MarketplacePurchaseEvent:
		return FormatMarketplacePurchaseEvent(e)
	case *github.PageBuildEvent:
		return FormatPageBuildEvent(e)
	case *github.DeployKeyEvent:
		return FormatDeployKeyEvent(e)
	case *github.CreateEvent:
		return FormatCreateEvent(e)
	case *github.DeleteEvent:
		return FormatDeleteEvent(e)
	case *github.IssueCommentEvent:
		return FormatIssueCommentEvent(e)
	case *github.MemberEvent:
		return FormatMemberEvent(e)
	case *github.PublicEvent:
		return FormatPublicEvent(e)
	case *github.StatusEvent:
		return FormatStatusEvent(e)
	case *github.WorkflowDispatchEvent:
		return FormatWorkflowDispatchEvent(e)
	case *github.TeamAddEvent:
		return FormatTeamAddEvent(e)
	case *github.TeamEvent:
		return FormatTeamEvent(e)
	case *github.PackageEvent:
		return FormatPackageEvent(e)
	case *github.GollumEvent:
		return FormatGollumEvent(e)
	case *github.MetaEvent:
		return FormatMetaEvent(e)
	case *github.InstallationEvent:
		return FormatInstallationEvent(e)
	default:
		return FormatGenericEvent(event)
	}
}
