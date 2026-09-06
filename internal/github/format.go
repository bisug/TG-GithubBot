package github

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/google/go-github/v90/github"
)

func FormatIssuesEvent(event *github.IssuesEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := event.GetRepo().GetFullName()
	action := event.GetAction()
	sender := event.GetSender().GetLogin()
	issue := event.GetIssue()
	title := issue.GetTitle()
	url := issue.GetHTMLURL()
	number := issue.GetNumber()

	msg := fmt.Sprintf(
		"<b>📌 %s issue #%d</b>\n"+
			"<b>Title:</b> %s\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(titleText(action)),
		number,
		EscapeHTML(title),
		FormatRepo(repo),
		FormatUser(sender),
	)

	switch action {
	case "opened", "edited":
		if body := issue.GetBody(); body != "" {
			msg += fmt.Sprintf("<b>Description:</b>\n%s\n", FormatTextWithMarkdown(body))
		}
	case "closed":
		if closer := issue.GetClosedBy(); closer != nil {
			msg += fmt.Sprintf("<b>Closed by:</b> %s\n", EscapeHTML(closer.GetLogin()))
		}
	case "reopened":
		msg += "<i>Issue reopened</i>\n"
	case "assigned":
		var assignees []string
		for _, a := range issue.Assignees {
			assignees = append(assignees, EscapeHTML(a.GetLogin()))
		}
		msg += fmt.Sprintf("<b>Assigned to:</b> %s\n", strings.Join(assignees, ", "))
	case "labeled":
		var labels []string
		for _, l := range issue.Labels {
			labels = append(labels, EscapeHTML(l.GetName()))
		}
		msg += fmt.Sprintf("<b>Labels:</b> %s\n", strings.Join(labels, ", "))
	case "milestoned":
		if m := issue.GetMilestone(); m != nil {
			msg += fmt.Sprintf("<b>Milestone:</b> %s\n", EscapeHTML(m.GetTitle()))
		}
	}

	return FormatMessageWithButton(msg, "View Issue", url)
}

func FormatPullRequestEvent(event *github.PullRequestEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := event.GetRepo().GetFullName()
	action := event.GetAction()
	sender := event.GetSender().GetLogin()
	pr := event.GetPullRequest()
	title := pr.GetTitle()
	url := pr.GetHTMLURL()
	state := pr.GetState()
	number := pr.GetNumber()

	msg := fmt.Sprintf(
		"<b>🚀 PR %s #%d: %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s | <b>State:</b> %s\n",
		EscapeHTML(titleText(action)),
		number,
		EscapeHTML(title),
		FormatRepo(repo),
		FormatUser(sender),
		EscapeHTML(state),
	)

	switch action {
	case "opened":
		msg += fmt.Sprintf("<b>Description:</b>\n%s\n", FormatTextWithMarkdown(pr.GetBody()))
	case "closed":
		if pr.GetMerged() {
			msg += "✅ Merged\n"
		} else {
			msg += "❌ Closed without merging\n"
		}
	case "reopened":
		msg += "🔄 Reopened\n"
	case "edited":
		msg += fmt.Sprintf("✏️ Edited\n<b>Description:</b>\n%s\n", FormatTextWithMarkdown(pr.GetBody()))
	case "assigned":
		var assignees []string
		for _, a := range pr.Assignees {
			assignees = append(assignees, EscapeHTML(a.GetLogin()))
		}
		msg += fmt.Sprintf("<b>Assigned:</b> %s\n", strings.Join(assignees, ", "))
	case "review_requested":
		var reviewers []string
		for _, r := range pr.RequestedReviewers {
			reviewers = append(reviewers, EscapeHTML(r.GetLogin()))
		}
		msg += fmt.Sprintf("<b>Reviewers:</b> %s\n", strings.Join(reviewers, ", "))
	case "labeled":
		var labels []string
		for _, l := range pr.Labels {
			labels = append(labels, EscapeHTML(l.GetName()))
		}
		msg += fmt.Sprintf("<b>Labels:</b> %s\n", strings.Join(labels, ", "))
	case "synchronize":
		msg += "🔄 New commits pushed\n"
	}

	return FormatMessageWithButton(msg, "View PR", url)
}

func FormatPushEvent(event *github.PushEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := event.Repo.GetFullName()
	if repo == "" {
		repo = event.Repo.GetName()
	}
	repoURL := event.Repo.GetHTMLURL()
	if repoURL == "" && repo != "" {
		repoURL = fmt.Sprintf("https://github.com/%s", repo)
	}
	refType, refName := pushRefParts(event.GetRef())
	compareURL := event.GetCompare()
	buttonURL := firstNonEmpty(compareURL, repoURL)

	var commits []*github.HeadCommit
	if len(event.Commits) > 0 {
		commits = event.Commits
	} else if event.HeadCommit != nil {
		commits = []*github.HeadCommit{event.HeadCommit}
	}

	commitCount := len(commits)
	commitPlural := pluralSuffix(commitCount)
	// Clickable repo link in the header so users can jump straight to the repo
	// (or the branch tree) from the notification.
	repoLink := FormatRepo(repo)
	if refType == "branch" && repoURL != "" {
		repoLink = fmt.Sprintf(`<a href="%s/tree/%s">%s</a>`, EscapeHTMLURL(repoURL), EscapeHTMLURL(refName), EscapeHTML(repo))
	}
	title := fmt.Sprintf("🔨 <b>%d new commit%s to</b> %s:<code>%s</code>\n\n", commitCount, commitPlural, repoLink, EscapeHTML(refName))
	if commitCount == 0 {
		title = fmt.Sprintf("🔨 <b>Push to</b> %s:<code>%s</code>\n\n", repoLink, EscapeHTML(refName))
	}
	msg := title

	if event.GetCreated() {
		msg += fmt.Sprintf("🌱 <i>New %s created</i>\n", EscapeHTML(refType))
	} else if event.GetDeleted() {
		msg += fmt.Sprintf("🗑️ <i>%s deleted</i>\n", EscapeHTML(titleText(refType)))
	} else if event.GetForced() {
		msg += "⚠️ <i>Force pushed</i>\n"
	}

	if commitCount == 0 {
		msg += "<i>No commits were included in this GitHub payload.</i>\n"
		return FormatMessageWithButton(msg, "View Repository", buttonURL)
	}

	const maxPushCommits = 10
	shownCommits := commits
	if len(shownCommits) > maxPushCommits {
		shownCommits = shownCommits[:maxPushCommits]
	}

	for _, commit := range shownCommits {
		shortSHA := ShortSHA(commit.GetID())
		commitURL := fmt.Sprintf("%s/commit/%s", repoURL, commit.GetID())
		if buttonURL == "" {
			buttonURL = commitURL
		}
		var authorStr string
		if login := commit.GetAuthor().GetLogin(); login != "" {
			authorStr = FormatUser(login)
		} else {
			authorStr = EscapeHTML(firstNonEmpty(commit.GetAuthor().GetName(), "unknown"))
		}

		commitMessage := EscapeHTML(truncateText(firstLine(commit.GetMessage()), 180))

		msg += fmt.Sprintf(
			"- <a href=\"%s\">%s</a>: %s by %s\n",
			EscapeHTMLURL(commitURL),
			EscapeHTML(shortSHA),
			commitMessage,
			authorStr,
		)
	}

	if remaining := commitCount - len(shownCommits); remaining > 0 {
		msg += fmt.Sprintf("<i>+%d more commit%s not shown.</i>\n", remaining, pluralSuffix(remaining))
	}

	if len(msg) > 4000 {
		msg = fmt.Sprintf(
			"🔨 <b>%d new commit(s) to</b> %s:<code>%s</code>\n\n"+
				"⚠️ <i>Too many commits to display, check the repository for details.</i>\n",
			commitCount, repoLink, EscapeHTML(refName),
		)
	}

	if commitCount == 1 {
		commitURL := fmt.Sprintf("%s/commit/%s", repoURL, commits[0].GetID())
		return FormatMessageWithButton(msg, "View Commit", commitURL)
	}
	return FormatMessageWithButton(msg, "View Commits", buttonURL)
}

func pushRefParts(ref string) (string, string) {
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		return "branch", strings.TrimPrefix(ref, "refs/heads/")
	case strings.HasPrefix(ref, "refs/tags/"):
		return "tag", strings.TrimPrefix(ref, "refs/tags/")
	case ref != "":
		return "ref", ref
	default:
		return "ref", "unknown"
	}
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if line, _, ok := strings.Cut(text, "\n"); ok {
		return strings.TrimSpace(line)
	}
	if text == "" {
		return "No commit message"
	}
	return text
}

func truncateText(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes-1]) + "…"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func pluralSuffix(count int) string {
	if count == 1 {
		return ""
	}
	return "s"
}

// emojiOr returns the emoji mapped to key, or fallback when key is unknown.
func emojiOr(m map[string]string, key, fallback string) string {
	if e, ok := m[key]; ok && e != "" {
		return e
	}
	return fallback
}

// commentActionEmoji maps the shared created/edited/deleted comment actions.
var commentActionEmoji = map[string]string{
	"created": "💬",
	"edited":  "✏️",
	"deleted": "🗑️",
}

func titleText(text string) string {
	words := strings.Fields(strings.ReplaceAll(text, "_", " "))
	for i, word := range words {
		if word == "" {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

func FormatCreateEvent(event *github.CreateEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := event.Repo.GetFullName()
	repoURL := event.Repo.GetHTMLURL()
	sender := event.Sender.GetLogin()
	refType := event.GetRefType()
	ref := event.GetRef()

	msg := fmt.Sprintf(
		"✨ <b>New %s created</b>\n\n"+
			"<b>Name:</b> <code>%s</code>\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(refType),
		EscapeHTML(ref),
		FormatRepo(repo),
		FormatUser(sender),
	)

	if desc := event.GetDescription(); desc != "" {
		msg += fmt.Sprintf("<b>Description:</b> %s\n", FormatTextWithMarkdown(desc))
	}

	if refType == "repository" && event.GetMasterBranch() != "" {
		msg += fmt.Sprintf("<b>Default branch:</b> %s\n", EscapeHTML(event.GetMasterBranch()))
	}

	return FormatMessageWithButton(msg, "View Repository", repoURL)
}

func FormatDeleteEvent(event *github.DeleteEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := event.Repo.GetFullName()
	repoURL := event.Repo.GetHTMLURL()
	sender := event.Sender.GetLogin()
	refType := event.GetRefType()
	ref := event.GetRef()

	emoji := "❌"
	switch refType {
	case "branch":
		emoji = "🌿"
	case "tag":
		emoji = "🏷️"
	}

	msg := fmt.Sprintf(
		"%s <b>Deleted %s:</b> <code>%s</code>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s",
		emoji,
		EscapeHTML(refType),
		EscapeHTML(ref),
		FormatRepo(repo),
		FormatUser(sender),
	)

	return FormatMessageWithButton(msg, "View Repository", repoURL)
}

func FormatForkEvent(event *github.ForkEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	originalRepo := event.Repo.GetFullName()
	forkedRepo := event.Forkee.GetFullName()
	sender := event.Sender.GetLogin()
	msg := fmt.Sprintf(
		"🍴 %s forked by %s\n\n"+
			"✨ <b>Stars:</b> %d | 🍴 <b>Forks:</b> %d",
		FormatRepo(originalRepo),
		FormatUser(sender),
		event.Repo.GetStargazersCount(),
		event.Repo.GetForksCount(),
	)

	return FormatMessageWithButton(msg, "View Fork", fmt.Sprintf("https://github.com/%s", EscapeHTMLURL(forkedRepo)))
}

func FormatCommitCommentEvent(event *github.CommitCommentEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	comment := event.Comment.GetBody()
	commitSHA := event.Comment.GetCommitID()
	repo := event.Repo.GetFullName()
	sender := event.Sender.GetLogin()
	action := event.GetAction()
	commitURL := fmt.Sprintf("https://github.com/%s/commit/%s", EscapeHTMLURL(repo), EscapeHTMLURL(commitSHA))

	actionEmoji := emojiOr(commentActionEmoji, action, "⚠️")

	msg := fmt.Sprintf(
		"%s <b>%s %s comment on commit</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Commit:</b> <a href=\"%s\"><code>%s</code></a>\n",
		actionEmoji,
		FormatUser(sender),
		EscapeHTML(action),
		FormatRepo(repo),
		EscapeHTML(ShortSHA(commitSHA)),
		commitURL,
	)

	if action == "created" || action == "edited" {
		msg += fmt.Sprintf("<b>Comment:</b> %s", FormatTextWithMarkdown(comment))
	}

	return FormatMessageWithButton(msg, "View Comment", event.Comment.GetHTMLURL())
}

func FormatPublicEvent(event *github.PublicEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	msg := fmt.Sprintf(
		"🔓 <b>Repository made public</b>\n\n"+
			"<b>Name:</b> %s\n"+
			"<b>By:</b> %s",
		FormatRepo(event.Repo.GetFullName()),
		FormatUser(event.Sender.GetLogin()),
	)
	return FormatMessageWithButton(msg, "View Repository", event.Repo.GetHTMLURL())
}

func FormatIssueCommentEvent(event *github.IssueCommentEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := event.GetAction()
	issue := event.Issue
	comment := event.Comment
	repo := event.Repo.GetFullName()
	sender := event.Sender.GetLogin()

	actionEmoji := emojiOr(commentActionEmoji, action, "⚠️")

	msg := fmt.Sprintf(
		"%s <b>%s %s comment on</b> <a href=\"%s\">%s#%d</a>\n\n"+
			"<b>Title:</b> %s\n",
		actionEmoji,
		FormatUser(sender),
		EscapeHTML(action),
		EscapeHTMLURL(issue.GetHTMLURL()),
		EscapeHTML(repo),
		issue.GetNumber(),
		EscapeHTML(issue.GetTitle()),
	)

	if action == "created" || action == "edited" {
		msg += fmt.Sprintf("<b>Comment:</b> %s", FormatTextWithMarkdown(comment.GetBody()))
	}

	return FormatMessageWithButton(msg, "View Comment", comment.GetHTMLURL())
}

func FormatMemberEvent(event *github.MemberEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := event.GetAction()
	member := event.Member.GetLogin()
	repo := event.Repo.GetFullName()
	sender := event.Sender.GetLogin()

	actionInfo := map[string]struct {
		emoji string
		verb  string
	}{
		"added":   {"➕", "added to"},
		"removed": {"➖", "removed from"},
		"edited":  {"✏️", "updated in"},
	}[action]

	if actionInfo.emoji == "" {
		actionInfo = struct {
			emoji string
			verb  string
		}{"⚠️", "performed action on"}
	}

	msg := fmt.Sprintf(
		"%s <b>%s</b> %s <b>%s</b>\n\n"+
			"<b>By:</b> %s",
		actionInfo.emoji,
		FormatUser(member),
		EscapeHTML(actionInfo.verb),
		FormatRepo(repo),
		FormatUser(sender),
	)

	if action == "edited" && event.Changes != nil {
		// Only permission/role changes exist on this event; render them explicitly
		// instead of dumping the raw struct (which breaks MarkdownV2).
		if p := event.Changes.Permission; p != nil {
			msg += fmt.Sprintf("\n<b>Permission:</b> %s → %s",
				EscapeHTML(p.GetFrom()), EscapeHTML(p.GetTo()))
		}
		if r := event.Changes.RoleName; r != nil {
			msg += fmt.Sprintf("\n<b>Role:</b> %s → %s",
				EscapeHTML(r.GetFrom()), EscapeHTML(r.GetTo()))
		}
	}

	return FormatMessageWithButton(msg, "View Repository", event.Repo.GetHTMLURL())
}

func FormatRepositoryEvent(event *github.RepositoryEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := event.GetAction()
	repo := event.Repo.GetFullName()
	url := event.Repo.GetHTMLURL()
	sender := event.Sender.GetLogin()

	actionDetails := map[string]struct {
		emoji string
		desc  string
	}{
		"created":    {"🎉", "created"},
		"renamed":    {"🔄", fmt.Sprintf("renamed to %s", EscapeHTML(event.Repo.GetName()))},
		"archived":   {"🔒", "archived"},
		"unarchived": {"🔓", "unarchived"},
	}[action]

	if actionDetails.emoji == "" {
		actionDetails = struct {
			emoji string
			desc  string
		}{"⚠️", fmt.Sprintf("performed %s action", action)}
	}

	msg := fmt.Sprintf(
		"%s %s %s\n\n"+
			"👤 <b>By:</b> %s",
		actionDetails.emoji,
		FormatRepo(repo),
		EscapeHTML(actionDetails.desc),
		FormatUser(sender),
	)
	return FormatMessageWithButton(msg, "View Repository", url)
}

func FormatReleaseEvent(event *github.ReleaseEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := event.GetAction()
	release := event.GetRelease()
	repo := event.GetRepo().GetFullName()
	sender := event.GetSender().GetLogin()

	actionDetails := map[string]struct {
		emoji string
		verb  string
	}{
		"created":   {"🎉", "New release"},
		"published": {"🚀", "Release published"},
		"deleted":   {"🗑️", "Release deleted"},
		"edited":    {"✏️", "Release edited"},
	}[action]

	if actionDetails.emoji == "" {
		actionDetails = struct {
			emoji string
			verb  string
		}{"⚠️", fmt.Sprintf("Unknown action (%s)", action)}
	}

	msg := fmt.Sprintf(
		"%s <b>%s in</b> %s\n\n"+
			"<b>Tag:</b> %s\n"+
			"<b>By:</b> %s",
		actionDetails.emoji,
		EscapeHTML(actionDetails.verb),
		FormatRepo(repo),
		EscapeHTML(release.GetTagName()),
		FormatUser(sender),
	)

	if (action == "created" || action == "edited") && release.GetBody() != "" {
		msg += fmt.Sprintf("\n<b>Notes:</b>\n%s", FormatReleaseBody(release.GetBody()))
	}

	return FormatMessageWithButton(msg, "View Release", release.GetHTMLURL())
}

func FormatWatchEvent(event *github.WatchEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := event.GetAction()
	if action == "started" {
		repo := event.GetRepo()
		msg := fmt.Sprintf(
			"⭐ %s starred %s\n\n"+
				"✨ <b>Stars:</b> %d | 🍴 <b>Forks:</b> %d",
			FormatUser(event.GetSender().GetLogin()),
			FormatRepo(repo.GetFullName()),
			repo.GetStargazersCount(),
			repo.GetForksCount(),
		)
		return FormatMessageWithButton(msg, "View Repository", event.GetRepo().GetHTMLURL())

	}

	msg := fmt.Sprintf(
		"⚠️ <b>Unexpected watch action:</b> %s on %s by %s",
		EscapeHTML(action),
		FormatRepo(event.GetRepo().GetFullName()),
		FormatUser(event.GetSender().GetLogin()),
	)

	return msg, nil
}

func FormatStatusEvent(event *github.StatusEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	state := event.GetState()
	stateEmoji := emojiOr(map[string]string{
		"success": "✅",
		"error":   "❌",
		"pending": "⏳",
	}, state, "⚠️")

	msg := fmt.Sprintf(
		"%s <b>%s for commit</b> <a href=\"%s\"><code>%s</code></a>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Status:</b> %s\n"+
			"<b>By:</b> %s",
		stateEmoji,
		EscapeHTML(titleText(state)),
		EscapeHTML(ShortSHA(event.GetCommit().GetSHA())),
		EscapeHTMLURL(event.GetCommit().GetHTMLURL()),
		FormatRepo(event.GetRepo().GetFullName()),
		EscapeHTML(event.GetDescription()),
		FormatUser(event.GetSender().GetLogin()),
	)
	return FormatMessageWithButton(msg, "View Commit", event.GetCommit().GetHTMLURL())
}

func FormatWorkflowRunEvent(e *github.WorkflowRunEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	workflow := e.GetWorkflow().GetName()
	run := e.GetWorkflowRun()
	repo := e.GetRepo().GetFullName()
	sender := e.GetSender().GetLogin()

	var statusEmoji string
	var statusLabel string
	conclusion := run.GetConclusion()
	status := run.GetStatus()

	switch status {
	case "completed":
		switch conclusion {
		case "success":
			statusEmoji = "✅"
			statusLabel = "Success"
		case "failure":
			statusEmoji = "❌"
			statusLabel = "Failed"
		case "neutral":
			statusEmoji = "⚖️"
			statusLabel = "Neutral"
		case "cancelled":
			statusEmoji = "⛔"
			statusLabel = "Cancelled"
		default:
			statusEmoji = "🏁"
			statusLabel = "Completed"
		}
	case "in_progress":
		statusEmoji = "⏳"
		statusLabel = "Running"
	case "queued":
		statusEmoji = "🔄"
		statusLabel = "Queued"
	default:
		statusEmoji = "⚠️"
		statusLabel = "Unknown status"
	}

	msg := fmt.Sprintf(
		"%s <b>Workflow Run %s in</b> %s\n\n"+
			"<b>Workflow:</b> <code>%s</code>\n"+
			"<b>Status:</b> <code>%s</code>\n"+
			"<b>By:</b> %s",
		statusEmoji,
		EscapeHTML(titleText(conclusion)),
		FormatRepo(repo),
		EscapeHTML(workflow),
		EscapeHTML(statusLabel),
		FormatUser(sender),
	)
	if conclusion == "" {
		msg = fmt.Sprintf(
			"%s <b>Workflow Run %s in</b> %s\n\n"+
				"<b>Workflow:</b> <code>%s</code>\n"+
				"<b>By:</b> %s",
			statusEmoji,
			EscapeHTML(statusLabel),
			FormatRepo(repo),
			EscapeHTML(workflow),
			FormatUser(sender),
		)
	}
	return FormatMessageWithButton(msg, "View Workflow Run", run.GetHTMLURL())
}

func FormatWorkflowJobEvent(e *github.WorkflowJobEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "⚙️ <b>No workflow job data</b>", nil
	}

	job := e.GetWorkflowJob()
	if job == nil {
		return "⚙️ <b>Invalid workflow job</b>", nil
	}

	status := job.GetStatus()
	conclusion := job.GetConclusion()
	statusEmoji := "⚙️"
	statusText := titleText(status)

	switch {
	case status == "completed" && conclusion == "success":
		statusEmoji = "✅"
		statusText = "Success"
	case status == "completed" && conclusion == "failure":
		statusEmoji = "❌"
		statusText = "Failed"
	case status == "in_progress":
		statusEmoji = "⏳"
	case status == "queued":
		statusEmoji = "🔄"
	case conclusion == "cancelled":
		statusEmoji = "⛔"
		statusText = "Cancelled"
	}

	msg := fmt.Sprintf("%s <b>Workflow Job %s</b>\n\n", statusEmoji, EscapeHTML(statusText))
	msg += fmt.Sprintf("<b>Name:</b> %s\n", EscapeHTML(job.GetName()))
	msg += fmt.Sprintf("<b>Repository:</b> %s\n", FormatRepo(e.GetRepo().GetFullName()))

	if !job.GetStartedAt().IsZero() {
		msg += fmt.Sprintf("<b>Started:</b> %s\n", EscapeHTML(job.GetStartedAt().Format("2006-01-02 15:04")))
	}
	if !job.GetCompletedAt().IsZero() {
		msg += fmt.Sprintf("<b>Completed:</b> %s\n", EscapeHTML(job.GetCompletedAt().Format("2006-01-02 15:04")))
	}

	if runner := job.GetRunnerName(); runner != "" {
		msg += fmt.Sprintf("<b>Runner:</b> %s\n", EscapeHTML(runner))
	}

	msg += fmt.Sprintf("<b>By:</b> %s\n", FormatUser(e.GetSender().GetLogin()))
	return FormatMessageWithButton(msg, "View Job", job.GetHTMLURL())
}

func FormatWorkflowDispatchEvent(e *github.WorkflowDispatchEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := e.GetRepo().GetFullName()
	workflow := e.GetWorkflow()
	if workflow == "" {
		workflow = "Unnamed Workflow"
	}

	inputs := "No inputs"
	if e.Inputs != nil {
		var inputsMap map[string]interface{}
		if err := json.Unmarshal(e.Inputs, &inputsMap); err == nil && len(inputsMap) > 0 {
			var inputPairs []string
			for k, v := range inputsMap {
				inputPairs = append(inputPairs, fmt.Sprintf("%s: %v", k, v))
			}
			inputs = strings.Join(inputPairs, ", ")
		}
	}

	msg := fmt.Sprintf(
		"🚀 <b>%s manually triggered</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Branch:</b> %s\n"+
			"<b>Inputs:</b> %s\n"+
			"<b>By:</b> %s",
		EscapeHTML(workflow),
		FormatRepo(repo),
		EscapeHTML(e.GetRef()),
		EscapeHTML(inputs),
		FormatUser(e.GetSender().GetLogin()),
	)
	return FormatMessageWithButton(msg, "View Repository", e.GetRepo().GetHTMLURL())
}

func FormatTeamAddEvent(e *github.TeamAddEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	msg := fmt.Sprintf(
		"👥 <b>Team added</b>\n\n"+
			"<b>Team:</b> %s\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Org:</b> %s\n"+
			"<b>By:</b> %s",
		EscapeHTML(e.GetTeam().GetName()),
		FormatRepo(e.GetRepo().GetFullName()),
		EscapeHTML(e.GetOrg().GetLogin()),
		FormatUser(e.GetSender().GetLogin()),
	)
	return FormatMessageWithButton(msg, "View Team", e.GetTeam().GetHTMLURL())
}

func FormatTeamEvent(e *github.TeamEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	team := e.GetTeam().GetName()
	org := e.GetOrg().GetLogin()
	sender := e.GetSender().GetLogin()

	actionInfo := map[string]struct {
		emoji string
		verb  string
	}{
		"created": {"🆕", "created"},
		"edited":  {"✏️", "modified"},
		"deleted": {"🗑️", "deleted"},
	}[action]

	if actionInfo.emoji == "" {
		actionInfo = struct {
			emoji string
			verb  string
		}{"⚙️", action}
	}

	msg := fmt.Sprintf(
		"%s <b>Team %s</b>\n\n"+
			"<b>Name:</b> %s\n"+
			"<b>Org:</b> %s\n"+
			"<b>By:</b> %s",
		actionInfo.emoji,
		EscapeHTML(actionInfo.verb),
		EscapeHTML(team),
		EscapeHTML(org),
		FormatUser(sender),
	)
	return FormatMessageWithButton(msg, "View Team", e.GetTeam().GetHTMLURL())
}

func FormatStarEvent(e *github.StarEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	emoji := "⭐️"
	actionText := "starred"
	if action == "deleted" {
		emoji = "❌"
		actionText = "unstarred"
	}

	user := e.GetSender().GetLogin()
	repo := e.GetRepo().GetFullName()
	repoURL := e.GetRepo().GetHTMLURL()
	stars := e.GetRepo().GetStargazersCount()
	forks := e.GetRepo().GetForksCount()

	msg := fmt.Sprintf(
		"%s %s %s %s\n\n✨ Stars: %d | 🍴 Forks: %d",
		emoji,
		FormatUser(user),
		EscapeHTML(actionText),
		FormatRepo(repo),
		stars,
		forks,
	)

	return FormatMessageWithButton(msg, "View Repository", repoURL)
}

func FormatRepositoryDispatchEvent(e *github.RepositoryDispatchEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := e.GetRepo().GetFullName()
	sender := e.GetSender().GetLogin()
	action := e.GetAction()
	branch := e.Branch
	if branch == nil {
		// e.Repo may be nil in malformed payloads; GetMasterBranch is nil-safe.
		if mb := e.GetRepo().GetMasterBranch(); mb != "" {
			branch = &mb
		}
	}

	var payloadStr string
	if e.ClientPayload != nil {
		var payload map[string]interface{}
		if err := json.Unmarshal(e.ClientPayload, &payload); err == nil {
			if len(payload) > 0 {
				payloadBytes, _ := json.Marshal(payload)
				payloadStr = fmt.Sprintf("\n<b>Payload:</b> <code>%s</code>", EscapeHTML(string(payloadBytes)))
			}
		}
	}

	msg := fmt.Sprintf(
		"🚀 <b>Repository Dispatch</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Action:</b> %s\n"+
			"<b>Branch:</b> %s\n"+
			"<b>By:</b> %s%s",
		FormatRepo(repo),
		EscapeHTML(action),
		EscapeHTML(branchOrDefault(branch)),
		FormatUser(sender),
		payloadStr,
	)
	return FormatMessageWithButton(msg, "View Repository", e.GetRepo().GetHTMLURL())
}

// Helper function to handle branch field
func branchOrDefault(branch *string) string {
	if branch != nil {
		return *branch
	}
	return "default branch"
}

func FormatPullRequestReviewCommentEvent(e *github.PullRequestReviewCommentEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo().GetFullName()
	comment := e.GetComment()
	pr := e.GetPullRequest()

	actionEmoji := emojiOr(commentActionEmoji, action, "⚠️")

	msg := fmt.Sprintf(
		"%s <b>PR Review Comment %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>PR:</b> <a href=\"%s\">%s#%d</a>\n"+
			"<b>Comment:</b> %s\n",
		actionEmoji,
		EscapeHTML(action),
		FormatRepo(repo),
		EscapeHTMLURL(pr.GetHTMLURL()),
		EscapeHTML(pr.GetTitle()),
		pr.GetNumber(),
		FormatTextWithMarkdown(comment.GetBody()),
	)
	return FormatMessageWithButton(msg, "View Comment", comment.GetHTMLURL())
}

func FormatPullRequestReviewEvent(e *github.PullRequestReviewEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	review := e.GetReview()
	pr := e.GetPullRequest()

	stateEmoji := emojiOr(map[string]string{
		"approved":          "✅",
		"changes_requested": "✏️",
		"commented":         "💬",
		"dismissed":         "❌",
	}, review.GetState(), "🔍")

	msg := fmt.Sprintf(
		"%s <b>PR Review %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>PR:</b> <a href=\"%s\">%s#%d</a>\n"+
			"<b>State:</b> %s\n"+
			"<b>By:</b> %s\n",
		stateEmoji,
		EscapeHTML(action),
		FormatRepo(e.GetRepo().GetFullName()),
		EscapeHTMLURL(pr.GetHTMLURL()),
		EscapeHTML(pr.GetTitle()),
		pr.GetNumber(),
		EscapeHTML(review.GetState()),
		FormatUser(e.GetSender().GetLogin()),
	)
	return FormatMessageWithButton(msg, "View Review", review.GetHTMLURL())
}

func FormatPingEvent(e *github.PingEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	msg := "🏓 <b>Webhook Ping Received</b>\n\n"

	if e.GetZen() != "" {
		msg += fmt.Sprintf("🧘 <i>%s</i>\n", EscapeHTML(e.GetZen()))
	}

	if e.GetRepo() != nil {
		msg += fmt.Sprintf(
			"📦 %s\n",
			FormatRepo(e.GetRepo().GetFullName()),
		)
	}

	if e.GetSender() != nil {
		msg += fmt.Sprintf("👤 <b>By:</b> %s\n", FormatUser(e.GetSender().GetLogin()))
	}

	if e.GetOrg() != nil {
		msg += fmt.Sprintf("🏢 <b>Org:</b> %s", EscapeHTML(e.GetOrg().GetLogin()))
	}

	buttonURL := e.GetRepo().GetHTMLURL()
	if buttonURL == "" {
		buttonURL = "https://github.com"
	}

	return FormatMessageWithButton(msg, "View GitHub", buttonURL)
}

func FormatSponsorshipEvent(e *github.SponsorshipEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	sender := e.GetSender()
	sponsorship := e.GetChanges()

	msg := fmt.Sprintf(
		"💖 <b>Sponsorship %s</b>\n\n"+
			"<b>Sponsor:</b> %s\n",
		EscapeHTML(action),
		FormatUser(sender.GetLogin()),
	)
	if sponsorship != nil && sponsorship.Tier != nil {
		msg += fmt.Sprintf("<b>Tier:</b> <code>%s</code> -> <code>%s</code>\n", EscapeHTML(sponsorship.Tier.GetFrom()), "new_tier")
	}

	return FormatMessageWithButton(msg, "View Sponsorship", sender.GetHTMLURL())
}

func FormatUserEvent(e *github.UserEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	user := e.GetUser()

	msg := fmt.Sprintf(
		"👤 <b>User %s</b>\n\n"+
			"<b>User:</b> %s\n",
		EscapeHTML(action),
		FormatUser(user.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View User", user.GetHTMLURL())
}

func FormatRepositoryImportEvent(e *github.RepositoryImportEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	status := e.GetStatus()
	repo := e.GetRepo()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"📥 <b>Repository Import %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(status),
		FormatRepo(repo.GetFullName()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Repository", repo.GetHTMLURL())
}

func FormatRepositoryRulesetEvent(e *github.RepositoryRulesetEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepository()
	sender := e.GetSender()
	ruleset := e.GetRepositoryRuleset()

	msg := fmt.Sprintf(
		"📜 <b>Repository Ruleset %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Ruleset:</b> <code>%s</code>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(ruleset.GetName()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Ruleset", fmt.Sprintf("%s/settings/rules/%d", repo.GetHTMLURL(), ruleset.GetID()))
}

func FormatSecretScanningAlertEvent(e *github.SecretScanningAlertEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	alert := e.GetAlert()
	repo := e.GetRepo()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🤫 <b>Secret Scanning Alert %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Secret Type:</b> <code>%s</code>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(alert.GetSecretType()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Alert", alert.GetHTMLURL())
}

func FormatSecretScanningAlertLocationEvent(e *github.SecretScanningAlertLocationEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"📍 <b>Secret Scanning Alert Location %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Alert", e.GetAlert().GetHTMLURL())
}

func FormatSecurityAndAnalysisEvent(e *github.SecurityAndAnalysisEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := e.GetRepository()
	sender := e.GetSender()
	changes := e.Changes

	var fromStatus string
	if changes.From != nil && changes.From.SecurityAndAnalysis != nil && changes.From.GetSecurityAndAnalysis() != nil && changes.From.GetSecurityAndAnalysis().AdvancedSecurity != nil {
		fromStatus = changes.From.GetSecurityAndAnalysis().AdvancedSecurity.GetStatus()
	}

	msg := fmt.Sprintf(
		"🔒 <b>Security & Analysis Settings Updated</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>From Status:</b> <code>%s</code>\n"+
			"<b>By:</b> %s\n",
		FormatRepo(repo.GetFullName()),
		EscapeHTML(fromStatus),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Security Settings", fmt.Sprintf("%s/settings/security_analysis", repo.GetHTMLURL()))
}

func FormatPullRequestReviewThreadEvent(e *github.PullRequestReviewThreadEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()
	pr := e.GetPullRequest()

	msg := fmt.Sprintf(
		"🧵 <b>PR Review Thread %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Pull Request:</b> <a href=\"%s\">%s</a>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(pr.GetTitle()),
		EscapeHTMLURL(pr.GetHTMLURL()),
		FormatUser(sender.GetLogin()),
	)

	if thread := e.GetThread(); thread != nil && len(thread.Comments) > 0 {
		return FormatMessageWithButton(msg, "View Thread", thread.Comments[0].GetHTMLURL())
	}
	return msg, nil
}

func FormatPullRequestTargetEvent(e *github.PullRequestTargetEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()
	pr := e.GetPullRequest()

	msg := fmt.Sprintf(
		"🎯 <b>PR Target %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Pull Request:</b> <a href=\"%s\">%s</a>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(pr.GetTitle()),
		EscapeHTMLURL(pr.GetHTMLURL()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View PR", pr.GetHTMLURL())
}

func FormatRegistryPackageEvent(e *github.RegistryPackageEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepository()
	sender := e.GetSender()
	pkg := e.RegistryPackage

	msg := fmt.Sprintf(
		"📦 <b>Registry Package %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Package:</b> <code>%s</code>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(pkg.GetName()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Package", pkg.GetHTMLURL())
}

func FormatMergeGroupEvent(e *github.MergeGroupEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🔄 <b>Merge Group %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Repository", repo.GetHTMLURL())
}

func FormatPersonalAccessTokenRequestEvent(e *github.PersonalAccessTokenRequestEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	org := e.GetOrg()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🔑 <b>Personal Access Token Request %s</b>\n\n"+
			"<b>Organization:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		EscapeHTML(org.GetLogin()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Organization Settings", fmt.Sprintf("https://github.com/organizations/%s/settings/personal-access-tokens", org.GetLogin()))
}

func FormatProjectV2Event(e *github.ProjectV2Event) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	org := e.GetOrg()
	sender := e.GetSender()
	project := e.ProjectsV2

	msg := fmt.Sprintf(
		"📋 <b>Project %s</b>\n\n"+
			"<b>Organization:</b> %s\n"+
			"<b>Project:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		EscapeHTML(org.GetLogin()),
		EscapeHTML(project.GetTitle()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Project", project.GetHTMLURL())
}

func FormatProjectV2ItemEvent(e *github.ProjectV2ItemEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	org := e.GetOrg()
	sender := e.GetSender()
	item := e.ProjectV2Item

	msg := fmt.Sprintf(
		"📄 <b>Project Item %s</b>\n\n"+
			"<b>Organization:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		EscapeHTML(org.GetLogin()),
		FormatUser(sender.GetLogin()),
	)
	contentType := item.GetContentType()
	if contentType != nil && *contentType == github.ProjectV2ItemContentTypePullRequest {
		msg += fmt.Sprintf("<b>Pull Request:</b> %s\n", item.GetContentNodeID())
	} else if contentType != nil && *contentType == github.ProjectV2ItemContentTypeIssue {
		msg += fmt.Sprintf("<b>Issue:</b> %s\n", item.GetContentNodeID())
	} else if contentType != nil && *contentType == github.ProjectV2ItemContentTypeDraftIssue {
		msg += fmt.Sprintf("<b>Draft Issue:</b> %s\n", item.GetContentNodeID())
	}

	return FormatMessageWithButton(msg, "View Item", item.GetProjectURL())
}

func FormatGitHubAppAuthorizationEvent(e *github.GitHubAppAuthorizationEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🔒 <b>GitHub App Authorization %s</b>\n\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatUser(sender.GetLogin()),
	)

	return msg, nil
}

func FormatInstallationRepositoriesEvent(e *github.InstallationRepositoriesEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	sender := e.GetSender()
	reposAdded := e.RepositoriesAdded
	reposRemoved := e.RepositoriesRemoved

	msg := fmt.Sprintf(
		"📦 <b>Installation Repositories %s</b>\n\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatUser(sender.GetLogin()),
	)
	if len(reposAdded) > 0 {
		var repoNames []string
		for _, r := range reposAdded {
			repoNames = append(repoNames, FormatRepo(r.GetFullName()))
		}
		msg += fmt.Sprintf("<b>Repositories Added:</b>\n%s\n", strings.Join(repoNames, "\n"))
	}
	if len(reposRemoved) > 0 {
		var repoNames []string
		for _, r := range reposRemoved {
			repoNames = append(repoNames, FormatRepo(r.GetFullName()))
		}
		msg += fmt.Sprintf("<b>Repositories Removed:</b>\n%s\n", strings.Join(repoNames, "\n"))
	}

	return FormatMessageWithButton(msg, "View Installation", e.GetInstallation().GetHTMLURL())
}

func FormatInstallationTargetEvent(e *github.InstallationTargetEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	sender := e.GetSender()
	target := e.GetAccount()

	msg := fmt.Sprintf(
		"🎯 <b>Installation Target %s</b>\n\n"+
			"<b>Target:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatUser(target.GetLogin()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Installation", e.GetInstallation().GetHTMLURL())
}

func FormatDiscussionCommentEvent(e *github.DiscussionCommentEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()
	discussion := e.GetDiscussion()
	comment := e.GetComment()

	msg := fmt.Sprintf(
		"💬 <b>Discussion Comment %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Discussion:</b> <a href=\"%s\">%s</a>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(discussion.GetTitle()),
		EscapeHTMLURL(discussion.GetHTMLURL()),
		FormatUser(sender.GetLogin()),
	)
	if action != "deleted" {
		msg += fmt.Sprintf("<b>Comment:</b> %s\n", FormatTextWithMarkdown(comment.GetBody()))
	}

	return FormatMessageWithButton(msg, "View Comment", comment.GetHTMLURL())
}

func FormatDiscussionEvent(e *github.DiscussionEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()
	discussion := e.GetDiscussion()

	msg := fmt.Sprintf(
		"📣 <b>Discussion %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Title:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(discussion.GetTitle()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Discussion", discussion.GetHTMLURL())
}

func FormatCodeScanningAlertEvent(e *github.CodeScanningAlertEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	alert := e.GetAlert()
	repo := e.GetRepo()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🛡️ <b>Code Scanning Alert %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Rule:</b> %s\n"+
			"<b>Severity:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(alert.GetRule().GetDescription()),
		EscapeHTML(alert.GetRuleSeverity()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Alert", alert.GetHTMLURL())
}

func FormatDependabotAlertEvent(e *github.DependabotAlertEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	alert := e.GetAlert()
	repo := e.GetRepo()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🤖 <b>Dependabot Alert %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Package:</b> <code>%s</code>\n"+
			"<b>Severity:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(alert.GetSecurityVulnerability().Package.GetName()),
		EscapeHTML(alert.GetSecurityVulnerability().GetSeverity()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Alert", alert.GetHTMLURL())
}

func FormatDeploymentProtectionRuleEvent(e *github.DeploymentProtectionRuleEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🛡️ <b>Deployment Protection Rule %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Environment:</b> <code>%s</code>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(e.GetEnvironment()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Deployment", e.GetDeployment().GetURL())
}

func FormatDeploymentReviewEvent(e *github.DeploymentReviewEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🔎 <b>Deployment Review %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Environment:</b> <code>%s</code>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(e.GetEnvironment()),
		FormatUser(sender.GetLogin()),
	)
	if e.Comment != nil {
		msg += fmt.Sprintf("<b>Comment:</b> %s\n", EscapeHTML(e.GetComment()))
	}

	return FormatMessageWithButton(msg, "View Workflow Run", e.GetWorkflowRun().GetHTMLURL())
}

func FormatContentReferenceEvent(e *github.ContentReferenceEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.GetRepo()
	sender := e.GetSender()
	ref := e.GetContentReference()

	msg := fmt.Sprintf(
		"🔗 <b>Content Reference %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Reference:</b> <code>%s</code>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(ref.GetReference()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Repository", repo.GetHTMLURL())
}

func FormatCustomPropertyEvent(e *github.CustomPropertyEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	org := e.GetOrg()
	sender := e.GetSender()
	prop := e.Definition

	msg := fmt.Sprintf(
		"📝 <b>Custom Property %s</b>\n\n"+
			"<b>Organization:</b> %s\n"+
			"<b>Property Name:</b> <code>%s</code>\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		EscapeHTML(org.GetLogin()),
		EscapeHTML(prop.GetPropertyName()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Organization Settings", fmt.Sprintf("https://github.com/organizations/%s/settings/custom-properties", org.GetLogin()))
}

func FormatCustomPropertyValuesEvent(e *github.CustomPropertyValuesEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	repo := e.GetRepo()
	sender := e.GetSender()

	var props []string
	for _, p := range e.NewPropertyValues {
		// p.Value is `any`; render via fmt and escape so arbitrary values cannot
		// break HTML parsing.
		props = append(props, fmt.Sprintf("<code>%s</code>: <code>%s</code>", EscapeHTML(p.PropertyName), EscapeHTML(fmt.Sprintf("%v", p.Value))))
	}

	msg := fmt.Sprintf(
		"🔄 <b>Custom Property Values Updated</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s\n"+
			"<b>New Values:</b>\n%s",
		FormatRepo(repo.GetFullName()),
		FormatUser(sender.GetLogin()),
		strings.Join(props, "\n"),
	)

	return FormatMessageWithButton(msg, "View Repository Settings", fmt.Sprintf("%s/settings/custom-properties", repo.GetHTMLURL()))
}

func FormatBranchProtectionRuleEvent(e *github.BranchProtectionRuleEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.Repo
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🛡️ <b>Branch Protection Rule %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		FormatUser(sender.GetLogin()),
	)
	if e.Rule != nil {
		msg += fmt.Sprintf("<b>Rule Name:</b> %s\n", EscapeHTML(e.Rule.GetName()))
	}

	return FormatMessageWithButton(msg, "View Branch Settings", fmt.Sprintf("%s/settings/branches", repo.GetHTMLURL()))
}

func FormatBranchProtectionConfigurationEvent(e *github.BranchProtectionConfigurationEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	repo := e.Repo
	sender := e.GetSender()

	msg := fmt.Sprintf(
		"🛡️ <b>Branch Protection Configuration %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>By:</b> %s\n",
		EscapeHTML(action),
		FormatRepo(repo.GetFullName()),
		FormatUser(sender.GetLogin()),
	)

	return FormatMessageWithButton(msg, "View Repository", repo.GetHTMLURL())
}

func FormatRepositoryVulnerabilityAlertEvent(e *github.RepositoryVulnerabilityAlertEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	alert := e.GetAlert()
	repo := e.GetRepository()

	msg := fmt.Sprintf(
		"🚨 <b>Vulnerability Alert: %s</b>\n\n"+
			"<b>Repository:</b> %s\n"+
			"<b>Severity:</b> %s\n"+
			"<b>Package:</b> %s\n",
		EscapeHTML(alert.GetAffectedPackageName()),
		FormatRepo(repo.GetFullName()),
		EscapeHTML(alert.GetSeverity()),
		EscapeHTML(alert.GetAffectedPackageName()),
	)

	return FormatMessageWithButton(msg, "View Alert", fmt.Sprintf("%s/security/advisories/%s", repo.GetHTMLURL(), alert.GetGitHubSecurityAdvisoryID()))
}

func FormatPageBuildEvent(e *github.PageBuildEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	msg := "🏗️ <b>GitHub Pages Build</b>\n\n"

	if e.Build != nil {
		msg += fmt.Sprintf("<b>Status:</b> %s\n", EscapeHTML(e.Build.GetStatus()))

		if e.Build.GetError() != nil {
			msg += fmt.Sprintf("<b>Error:</b> %s\n", EscapeHTML(e.Build.Error.GetMessage()))
		}
	}

	if e.GetRepo() != nil {
		msg += fmt.Sprintf(
			"📦 %s\n",
			FormatRepo(e.GetRepo().GetFullName()),
		)
	}

	if e.GetSender() != nil {
		msg += fmt.Sprintf("👤 <b>By:</b> %s", FormatUser(e.GetSender().GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Repository", e.GetRepo().GetHTMLURL())
}

func FormatPackageEvent(e *github.PackageEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	msg := "📦 <b>Package Event</b>\n\n"

	if name := e.GetPackage().GetName(); name != "" {
		msg += fmt.Sprintf("<b>Package:</b> %s\n", EscapeHTML(name))
	}

	if e.GetRepo() != nil {
		msg += fmt.Sprintf(
			"<b>Repository:</b> %s\n",
			FormatRepo(e.GetRepo().GetFullName()),
		)
	}

	if e.GetSender() != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(e.GetSender().GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Package", e.GetPackage().GetHTMLURL())
}

func FormatOrgBlockEvent(e *github.OrgBlockEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	msg := "🚫 <b>Organization Block</b>\n\n"
	if user := e.GetBlockedUser(); user != nil {
		msg += fmt.Sprintf("<b>Blocked:</b> %s\n", FormatUser(user.GetLogin()))
	}

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Organization", e.GetOrganization().GetHTMLURL())
}

func FormatOrganizationEvent(e *github.OrganizationEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	sender := e.GetSender()

	msg := fmt.Sprintf("🏢 <b>Organization Event</b>\n<b>Action:</b> %s", EscapeHTML(action))

	if sender != nil {
		msg += fmt.Sprintf("\n<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Organization", e.GetOrganization().GetHTMLURL())
}

func FormatMilestoneEvent(e *github.MilestoneEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	milestone := e.GetMilestone()
	action := e.GetAction()

	msg := fmt.Sprintf("🏁 <b>Milestone %s</b>\n\n", EscapeHTML(action))

	if milestone != nil {
		msg += fmt.Sprintf("<b>Title:</b> %s\n", EscapeHTML(milestone.GetTitle()))
		if desc := milestone.GetDescription(); desc != "" {
			msg += fmt.Sprintf("<b>Description:</b> %s\n", FormatTextWithMarkdown(desc))
		}
	}

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Milestone", e.GetMilestone().GetHTMLURL())
}

func FormatMetaEvent(e *github.MetaEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	msg := "⚙️ <b>Meta Event</b>\n\n"

	if id := e.GetHookID(); id != 0 {
		msg += fmt.Sprintf("<b>Hook ID:</b> %d\n", id)
	}

	if repo := e.GetRepo(); repo != nil {
		msg += fmt.Sprintf("<b>Repository:</b> %s\n", FormatRepo(repo.GetFullName()))
	}

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s\n", FormatUser(sender.GetLogin()))
	}

	if org := e.GetOrg(); org != nil {
		msg += fmt.Sprintf("<b>Org:</b> %s\n", EscapeHTML(org.GetLogin()))
	}

	if install := e.GetInstallation(); install != nil {
		msg += fmt.Sprintf("<b>Install ID:</b> %d", install.GetID())
	}

	return msg, nil
}

func FormatMembershipEvent(e *github.MembershipEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "🚫 <b>No membership event data</b>", nil
	}

	msg := fmt.Sprintf("👥 <b>Membership %s</b>\n\n", EscapeHTML(e.GetAction()))

	if scope := e.GetScope(); scope != "" {
		msg += fmt.Sprintf("<b>Scope:</b> %s\n", EscapeHTML(scope))
	}

	if member := e.GetMember(); member != nil {
		msg += fmt.Sprintf("<b>Member:</b> %s\n", FormatUser(member.GetLogin()))
	}

	if team := e.GetTeam(); team != nil {
		msg += fmt.Sprintf("<b>Team:</b> %s\n", EscapeHTML(team.GetName()))
		if desc := team.GetDescription(); desc != "" {
			msg += fmt.Sprintf("<b>Description:</b> %s\n", FormatTextWithMarkdown(desc))
		}
	}

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Team", e.GetTeam().GetHTMLURL())
}

func FormatDeploymentEvent(e *github.DeploymentEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	msg := "🚀 <b>Deployment Event</b>\n\n"

	if deploy := e.GetDeployment(); deploy != nil {
		msg += fmt.Sprintf("<b>ID:</b> %d\n", deploy.GetID())
		if desc := deploy.GetDescription(); desc != "" {
			msg += fmt.Sprintf("<b>Description:</b> %s\n", FormatTextWithMarkdown(desc))
		}
	}

	if repo := e.GetRepo(); repo != nil {
		msg += fmt.Sprintf("<b>Repository:</b> %s\n", FormatRepo(repo.GetName()))
	}

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Deployment", e.GetDeployment().GetURL())
}

func FormatLabelEvent(e *github.LabelEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "🏷️ <b>No label event data</b>", nil
	}

	msg := fmt.Sprintf("🏷️ <b>Label %s</b>\n\n", EscapeHTML(e.GetAction()))

	if label := e.GetLabel(); label != nil {
		msg += fmt.Sprintf("<b>Name:</b> %s\n", EscapeHTML(label.GetName()))
		msg += fmt.Sprintf("<b>Color:</b> <code>#%s</code>\n", EscapeHTML(label.GetColor()))
		if desc := label.GetDescription(); desc != "" {
			msg += fmt.Sprintf("<b>Description:</b> %s\n", FormatTextWithMarkdown(desc))
		}
	}

	if changes := e.GetChanges(); changes != nil {
		if title := changes.GetTitle(); title != nil && title.GetFrom() != "" {
			msg += fmt.Sprintf("<b>Previous Name:</b> %s\n", EscapeHTML(title.GetFrom()))
		}
		if body := changes.GetBody(); body != nil && body.GetFrom() != "" {
			msg += fmt.Sprintf("<b>Previous Desc:</b> %s\n", FormatTextWithMarkdown(body.GetFrom()))
		}
	}

	return FormatMessageWithButton(msg, "View Repository", e.GetRepo().GetHTMLURL())
}

func FormatMarketplacePurchaseEvent(e *github.MarketplacePurchaseEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "🛒 <b>No marketplace data</b>", nil
	}

	msg := fmt.Sprintf("🛒 <b>Marketplace %s</b>\n\n", EscapeHTML(e.GetAction()))

	if purchase := e.GetMarketplacePurchase(); purchase != nil {
		if plan := purchase.GetPlan(); plan != nil {
			msg += fmt.Sprintf("<b>Plan:</b> %s\n", EscapeHTML(plan.GetName()))
		}
		msg += fmt.Sprintf("<b>Billing:</b> %s\n", EscapeHTML(purchase.GetBillingCycle()))
		msg += fmt.Sprintf("<b>Units:</b> %d\n", purchase.GetUnitCount())
		if nextBill := purchase.GetNextBillingDate(); !nextBill.IsZero() {
			msg += fmt.Sprintf("<b>Next Bill:</b> %s\n", EscapeHTML(nextBill.Format("2006-01-02")))
		}

		if account := purchase.GetAccount(); account != nil {
			msg += fmt.Sprintf("<b>Account:</b> %s (%s)\n",
				FormatUser(account.GetLogin()),
				EscapeHTML(account.GetType()))
		}
	}

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return msg, nil
}

func FormatGollumEvent(e *github.GollumEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "📚 <b>No wiki update data available</b>", nil
	}

	var msg strings.Builder
	msg.WriteString("📚 <b>Wiki Update</b>\n\n")
	if repo := e.GetRepo(); repo != nil {
		msg.WriteString(fmt.Sprintf("<b>Repository:</b> %s\n",
			FormatRepo(repo.GetFullName())))
	}

	if org := e.GetOrg(); org != nil {
		msg.WriteString(fmt.Sprintf("<b>Organization:</b> %s\n", EscapeHTML(org.GetLogin())))
	}

	if sender := e.GetSender(); sender != nil {
		msg.WriteString(fmt.Sprintf("<b>Edited by:</b> %s\n", FormatUser(sender.GetLogin())))
	}

	if len(e.Pages) > 0 {
		msg.WriteString("\n<b>Page Changes:</b>\n")
		for _, page := range e.Pages {
			if page == nil {
				continue
			}
			action := "unknown"
			if page.Action != nil {
				action = *page.Action
			}
			emoji := getActionEmoji(action)
			pageTitle := ""
			if page.Title != nil {
				pageTitle = *page.Title
			} else if page.PageName != nil {
				pageTitle = *page.PageName
			}

			if pageTitle != "" {
				msg.WriteString(fmt.Sprintf("%s <b>%s</b> (%s)\n",
					emoji,
					EscapeHTML(pageTitle),
					EscapeHTML(action)))
			}
			if page.Summary != nil && *page.Summary != "" {
				msg.WriteString(fmt.Sprintf("<i>Summary:</i> %s\n", FormatTextWithMarkdown(*page.Summary)))
			}

			if page.SHA != nil && *page.SHA != "" {
				msg.WriteString(fmt.Sprintf("<i>Revision:</i> %s\n", EscapeHTML(ShortSHA(*page.SHA))))
			}
			if page.HTMLURL != nil && *page.HTMLURL != "" {
				msg.WriteString(fmt.Sprintf("<a href=\"%s\">View Page</a>\n", EscapeHTMLURL(*page.HTMLURL)))
			}

			msg.WriteString("\n")
		}
	}

	return msg.String(), nil
}

func getActionEmoji(action string) string {
	switch action {
	case "created":
		return "🆕"
	case "edited":
		return "✏️"
	case "deleted":
		return "🗑️"
	default:
		return "🔹"
	}
}

func FormatDeployKeyEvent(e *github.DeployKeyEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "🔑 <b>No deploy key data</b>", nil
	}

	msg := fmt.Sprintf("🔑 <b>Deploy Key %s</b>\n\n", EscapeHTML(e.GetAction()))

	if key := e.GetKey(); key != nil {
		msg += fmt.Sprintf("<b>Title:</b> %s\n", EscapeHTML(key.GetTitle()))
		if url := key.GetURL(); url != "" {
			msg += fmt.Sprintf("<a href=\"%s\">View Key</a>\n", EscapeHTMLURL(url))
		}
	}

	msg += fmt.Sprintf("<b>Repository:</b> %s\n", FormatRepo(e.GetRepo().GetName()))

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Repository", e.GetRepo().GetHTMLURL())
}

func FormatCheckSuiteEvent(e *github.CheckSuiteEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "✅ <b>No check suite data</b>", nil
	}

	suite := e.GetCheckSuite()
	var msg strings.Builder

	action := titleText(e.GetAction())
	msg.WriteString(fmt.Sprintf("✅ <b>Check Suite: %s</b>\n\n", EscapeHTML(action)))

	if suite != nil {
		status := suite.GetStatus()
		msg.WriteString(fmt.Sprintf("• <b>Status:</b> %s\n", EscapeHTML(status)))

		if conclusion := suite.GetConclusion(); conclusion != "" {
			msg.WriteString(fmt.Sprintf("• <b>Result:</b> %s\n", EscapeHTML(conclusion)))
		}
	}

	msg.WriteString(fmt.Sprintf("\n<b>Repository:</b> %s\n", FormatRepo(e.GetRepo().GetFullName())))

	if sender := e.GetSender(); sender != nil {
		username := sender.GetLogin()
		msg.WriteString(fmt.Sprintf("<b>Triggered by:</b> %s", EscapeHTML(username)))
	}

	return FormatMessageWithButton(msg.String(), "View Details", e.GetCheckSuite().GetURL())
}

func FormatCheckRunEvent(e *github.CheckRunEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "⚙️ <b>No check run data</b>", nil
	}

	check := e.GetCheckRun()
	var msg strings.Builder

	action := titleText(e.GetAction())
	msg.WriteString(fmt.Sprintf("⚙️ <b>Check Run: %s</b>\n\n", EscapeHTML(action)))

	if check != nil {
		name := check.GetName()
		status := check.GetStatus()
		msg.WriteString(fmt.Sprintf("• <b>Name:</b> %s\n", EscapeHTML(name)))
		msg.WriteString(fmt.Sprintf("• <b>Status:</b> %s\n", EscapeHTML(status)))

		if conclusion := check.GetConclusion(); conclusion != "" {
			msg.WriteString(fmt.Sprintf("• <b>Result:</b> %s\n", EscapeHTML(conclusion)))
		}

		if !check.GetStartedAt().IsZero() {
			msg.WriteString(fmt.Sprintf("• <b>Started:</b> %s\n", EscapeHTML(check.GetStartedAt().Format("2006-01-02 15:04"))))
		}

		if !check.GetCompletedAt().IsZero() {
			msg.WriteString(fmt.Sprintf("• <b>Completed:</b> %s\n", EscapeHTML(check.GetCompletedAt().Format("2006-01-02 15:04"))))
		}
	}

	msg.WriteString(fmt.Sprintf("\n<b>Repository:</b> %s\n", FormatRepo(e.GetRepo().GetFullName())))

	if sender := e.GetSender(); sender != nil {
		username := sender.GetLogin()
		msg.WriteString(fmt.Sprintf("<b>Triggered by:</b> %s", EscapeHTML(username)))
	}

	return FormatMessageWithButton(msg.String(), "View Details", e.GetCheckRun().GetHTMLURL())
}

func FormatDeploymentStatusEvent(e *github.DeploymentStatusEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "🚦 <b>No deployment status data</b>", nil
	}

	status := e.GetDeploymentStatus()
	msg := fmt.Sprintf("🚦 <b>Deployment %s</b>\n\n", EscapeHTML(status.GetState()))

	if desc := status.GetDescription(); desc != "" {
		msg += fmt.Sprintf("<b>Status:</b> %s\n", FormatTextWithMarkdown(desc))
	}

	msg += fmt.Sprintf("<b>Repository:</b> %s\n", FormatRepo(e.GetRepo().GetName()))

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Deployment", e.GetDeploymentStatus().GetDeploymentURL())
}

func FormatSecurityAdvisoryEvent(e *github.SecurityAdvisoryEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	if e == nil {
		return "⚠️ <b>No security advisory data</b>", nil
	}

	adv := e.GetSecurityAdvisory()
	msg := fmt.Sprintf("⚠️ <b>Security Advisory %s</b>\n\n", EscapeHTML(e.GetAction()))

	if adv != nil {
		msg += fmt.Sprintf("<b>Summary:</b> %s\n", FormatTextWithMarkdown(adv.GetSummary()))
		if sev := adv.GetSeverity(); sev != "" {
			msg += fmt.Sprintf("<b>Severity:</b> %s\n", EscapeHTML(sev))
		}
		if cve := adv.GetCVEID(); cve != "" {
			msg += fmt.Sprintf("<b>CVE:</b> %s\n", EscapeHTML(cve))
		}
		if url := adv.GetURL(); url != "" {
			msg += fmt.Sprintf("<a href=\"%s\">View Advisory</a>\n", EscapeHTMLURL(url))
		}
		if author := adv.GetAuthor(); author != nil {
			msg += fmt.Sprintf("<b>Reported by:</b> %s\n", FormatUser(author.GetLogin()))
		}
	}

	if repo := e.GetRepository(); repo != nil {
		msg += fmt.Sprintf("<b>Repository:</b> %s\n", FormatRepo(repo.GetFullName()))
	}

	if org := e.GetOrganization(); org != nil {
		msg += fmt.Sprintf("<b>Org:</b> %s\n", EscapeHTML(org.GetLogin()))
	}

	if sender := e.GetSender(); sender != nil {
		msg += fmt.Sprintf("<b>By:</b> %s", FormatUser(sender.GetLogin()))
	}

	return FormatMessageWithButton(msg, "View Advisory", e.GetSecurityAdvisory().GetHTMLURL())
}

func FormatInstallationEvent(e *github.InstallationEvent) (string, *gotgbot.InlineKeyboardMarkup) {
	action := e.GetAction()
	sender := e.GetSender().GetLogin()

	var msg string
	switch action {
	case "created":
		msg = "🎉 <b>Bot Installed Successfully</b>!\n\n"
		msg += "I am now linked to your account and will monitor your repositories for updates.\n\n"
		msg += fmt.Sprintf("👤 <b>By:</b> %s", FormatUser(sender))
	case "deleted":
		msg = "🗑️ <b>Bot Uninstalled</b>\n\n"
		msg += "I have been removed from your account and will no longer send notifications.\n\n"
		msg += fmt.Sprintf("👤 <b>By:</b> %s", FormatUser(sender))
	default:
		msg = fmt.Sprintf("🤖 <b>Installation Update:</b> <code>%s</code>", EscapeHTML(action))
	}

	return msg, nil
}

func FormatGenericEvent(event interface{}) (string, *gotgbot.InlineKeyboardMarkup) {
	if event == nil {
		return "", nil
	}

	eventType := fmt.Sprintf("%T", event)
	eventType = strings.TrimPrefix(eventType, "*github.")
	msg := fmt.Sprintf(
		"⚙️ <b>GitHub event received</b>\n\n"+
			"<b>Type:</b> <code>%s</code>\n\n"+
			"<i>This event type is supported by the webhook parser but does not have a specialized formatter yet.</i>",
		EscapeHTML(eventType),
	)
	return msg, nil
}
