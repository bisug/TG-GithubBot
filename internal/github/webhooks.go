package github

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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
	"github-webhook/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/go-github/v85/github"
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
	DeliverySeen *cache.Cache[string, struct{}]                // Key: X-GitHub-Delivery (idempotency)
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
				// log.Printf("Decrypted Chat ID from token: %d", chatID)
			} else {
				log.Printf("Failed to parse decrypted token as int64: %v", err)
			}
		} else {
			log.Printf("Failed to decrypt webhook token: %v", err)
		}
	}

	if chatID == 0 {
		log.Printf("Webhook rejected: invalid token event=%s delivery=%s hook_id=%s remote=%s path=%s", eventType, deliveryID, hookIDHeader, r.RemoteAddr, r.URL.Path)
		http.Error(w, "Unauthorized: Token required", http.StatusUnauthorized)
		return
	}

	log.Printf("Webhook received event=%s delivery=%s hook_id=%s chat=%d remote=%s", eventType, deliveryID, hookIDHeader, chatID, r.RemoteAddr)

	payload, err := github.ValidatePayload(r, []byte(s.Config.GitHubWebhookSecret))
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			log.Printf("Webhook rejected: payload too large event=%s delivery=%s hook_id=%s chat=%d limit=%d", eventType, deliveryID, hookIDHeader, chatID, maxBytesErr.Limit)
			http.Error(w, "Payload too large", http.StatusRequestEntityTooLarge)
			return
		}

		if s.Config.GitHubWebhookSecret == "" {
			log.Printf("Webhook Warning: No GITHUB_WEBHOOK_SECRET configured. Validation skipped. event=%s chat=%d", eventType, chatID)
			body, _ := io.ReadAll(r.Body)
			payload = body
		} else {
			log.Printf("Webhook REJECTED: Signature mismatch. This means the secret in GitHub doesn't match GITHUB_WEBHOOK_SECRET on Render. event=%s delivery=%s chat=%d error=%v", eventType, deliveryID, chatID, err)
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	event, err := parseWebhookEvent(eventType, payload)
	if err != nil {
		log.Printf("Webhook rejected: parse failed event=%s delivery=%s hook_id=%s chat=%d error=%v", eventType, deliveryID, hookIDHeader, chatID, err)
		http.Error(w, "Parse error", http.StatusInternalServerError)
		return
	}

	var hookID int64
	if idStr := hookIDHeader; idStr != "" {
		hookID, _ = strconv.ParseInt(idStr, 10, 64)
	}

	if deliveryID != "" {
		if _, seen := s.DeliverySeen.Get(deliveryID); seen {
			log.Printf("Webhook duplicate delivery ignored event=%s delivery=%s chat=%d", eventType, deliveryID, chatID)
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
				log.Printf("Webhook processing panic event=%s delivery=%s hook_id=%d chat=%d elapsed=%s panic=%v", eventType, deliveryID, hookID, chatID, time.Since(receivedAt).Round(time.Millisecond), recovered)
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
				log.Printf("Failed to update repo name for chat %d: %v", chatID, err)
			} else {
				log.Printf("Updated repo name to %s for chat %d", newFullName, chatID)
			}
		}
	}

	msg, markup := s.formatMessage(event)
	markup = s.withPRActionButtons(event, markup)
	if msg == "" {
		log.Printf("Webhook skipped: empty formatted message event=%s delivery=%s chat=%d elapsed=%s", eventType, deliveryID, chatID, time.Since(receivedAt).Round(time.Millisecond))
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

	opts := &gotgbot.SendMessageOpts{
		ParseMode: "MarkdownV2",
		LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
			IsDisabled: true,
		},
		ReplyMarkup:     markup,
		MessageThreadId: threadID,
	}

	sentMsg, err := s.Bot.SendMessage(chatID, msg, opts)
	if err != nil {
		log.Printf("Markdown send failed; retrying plain text chat=%d event=%s delivery=%s error=%v", chatID, eventType, deliveryID, err)
		fallbackOpts := &gotgbot.SendMessageOpts{
			LinkPreviewOptions: &gotgbot.LinkPreviewOptions{
				IsDisabled: true,
			},
			ReplyMarkup:     markup,
			MessageThreadId: threadID,
		}
		sentMsg, err = s.Bot.SendMessage(chatID, plainTextForTelegram(msg), fallbackOpts)
		if err != nil {
			log.Printf("Error sending message to chat %d event=%s delivery=%s elapsed=%s: %v", chatID, eventType, deliveryID, time.Since(receivedAt).Round(time.Millisecond), err)
			return
		}
	}

	s.storeMessageContext(sentMsg.MessageId, chatID, event)
	log.Printf("Webhook delivered chat=%d event=%s delivery=%s message_id=%d elapsed=%s", chatID, eventType, deliveryID, sentMsg.MessageId, time.Since(receivedAt).Round(time.Millisecond))
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
		log.Printf("Failed to generate PR action id for %s/%s#%d: %v", owner, repo, prNum, err)
		return markup
	}

	s.ActionCache.Set(id, models.PRActionContext{Owner: owner, Repo: repo, PRNumber: prNum}, 48*time.Hour)

	row := []gotgbot.InlineKeyboardButton{
		ui.Callback("✅ Approve", "act:approve:"+id, ui.WithStyle(ui.StyleSuccess)),
		ui.Callback("🔒 Close", "act:close:"+id, ui.WithStyle(ui.StyleDanger)),
	}

	if markup == nil {
		return &gotgbot.InlineKeyboardMarkup{InlineKeyboard: [][]gotgbot.InlineKeyboardButton{row}}
	}

	markup.InlineKeyboard = append(markup.InlineKeyboard, row)
	return markup
}

func (s *WebhookServer) formatMessage(event interface{}) (msg string, markup *gotgbot.InlineKeyboardMarkup) {
	defer func() {
		if recover() != nil {
			msg, markup = FormatGenericEvent(event)
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
