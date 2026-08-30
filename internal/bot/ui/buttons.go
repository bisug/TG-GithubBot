package ui

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
	gh "github.com/google/go-github/v90/github"
)

const (
	StyleDanger  = gotgbot.KeyboardButtonStyleDanger
	StyleSuccess = gotgbot.KeyboardButtonStyleSuccess
	StylePrimary = gotgbot.KeyboardButtonStylePrimary

	IconAdd      = "TG_BUTTON_ICON_ADD"
	IconAll      = "TG_BUTTON_ICON_ALL"
	IconBack     = "TG_BUTTON_ICON_BACK"
	IconCancel   = "TG_BUTTON_ICON_CANCEL"
	IconChoose   = "TG_BUTTON_ICON_CHOOSE"
	IconConfirm  = "TG_BUTTON_ICON_CONFIRM"
	IconConnect  = "TG_BUTTON_ICON_CONNECT"
	IconGitHub   = "TG_BUTTON_ICON_GITHUB"
	IconNext     = "TG_BUTTON_ICON_NEXT"
	IconPrevious = "TG_BUTTON_ICON_PREVIOUS"
	IconPush     = "TG_BUTTON_ICON_PUSH"
	IconSettings = "TG_BUTTON_ICON_SETTINGS"
	IconStop     = "TG_BUTTON_ICON_STOP"
)

type ButtonOption func(*gotgbot.InlineKeyboardButton)

func Callback(text string, data string, opts ...ButtonOption) gotgbot.InlineKeyboardButton {
	btn := gotgbot.InlineKeyboardButton{
		Text:         text,
		CallbackData: data,
	}
	applyOptions(&btn, opts...)
	validateCallback(btn.CallbackData)
	return btn
}

func URL(text string, url string, opts ...ButtonOption) gotgbot.InlineKeyboardButton {
	btn := gotgbot.InlineKeyboardButton{
		Text: text,
		Url:  url,
	}
	applyOptions(&btn, opts...)
	return btn
}

func WithStyle(style string) ButtonOption {
	return func(btn *gotgbot.InlineKeyboardButton) {
		switch style {
		case StyleDanger, StyleSuccess, StylePrimary:
			btn.Style = style
		case "":
		default:
			log.Printf("Ignoring unsupported Telegram button style %q", style)
		}
	}
}

func WithCustomEmojiEnv(envKey string) ButtonOption {
	return func(btn *gotgbot.InlineKeyboardButton) {
		if envKey == "" {
			return
		}
		if id := strings.TrimSpace(os.Getenv(envKey)); id != "" {
			btn.IconCustomEmojiId = id
		}
	}
}

func applyOptions(btn *gotgbot.InlineKeyboardButton, opts ...ButtonOption) {
	for _, opt := range opts {
		if opt != nil {
			opt(btn)
		}
	}
}

func validateCallback(data string) {
	if len(data) > 64 {
		log.Printf("Telegram callback_data exceeds 64 bytes: length=%d data=%q", len(data), data)
	}
}

// Row wraps buttons into a single keyboard row.
func Row(buttons ...gotgbot.InlineKeyboardButton) []gotgbot.InlineKeyboardButton {
	return buttons
}

// Markup wraps keyboard rows into an inline keyboard markup.
func Markup(rows ...[]gotgbot.InlineKeyboardButton) gotgbot.InlineKeyboardMarkup {
	return gotgbot.InlineKeyboardMarkup{InlineKeyboard: rows}
}

// BackButton returns the standard "Back" button for the given callback data.
func BackButton(data string) gotgbot.InlineKeyboardButton {
	return Callback("Back", data, WithStyle(StylePrimary), WithCustomEmojiEnv(IconBack))
}

// RepoSettingsButton returns the row button that opens a repository's settings menu.
func RepoSettingsButton(fullName, data string) gotgbot.InlineKeyboardButton {
	return Callback(CompactButtonText(fullName), data, WithStyle(StylePrimary), WithCustomEmojiEnv(IconSettings))
}

// AddRepoButton returns the row button that links a repository to the chat.
func AddRepoButton(fullName, data string) gotgbot.InlineKeyboardButton {
	return Callback(CompactButtonText(fullName), data, WithStyle(StylePrimary), WithCustomEmojiEnv(IconAdd))
}

// RepoPageNav builds the previous / numbered / next pagination row for the add-repo
// repository picker. pageData maps a page number to its callback data string; commands
// and callbacks pass their own builder so ui stays unaware of the callback protocol.
func RepoPageNav(page int, resp *gh.Response, pageData func(int) string) []gotgbot.InlineKeyboardButton {
	var navRow []gotgbot.InlineKeyboardButton

	if resp.PrevPage == 0 && resp.NextPage == 0 {
		return nil // single page: nothing to navigate
	}

	if resp.FirstPage != 0 && resp.PrevPage != 0 {
		navRow = append(navRow, Callback("<", pageData(resp.PrevPage),
			WithStyle(StylePrimary),
			WithCustomEmojiEnv(IconPrevious),
		))
	}

	startPage := page - 1
	if startPage < 1 {
		startPage = 1
	}

	endPage := page + 1
	if resp.LastPage != 0 {
		if endPage > resp.LastPage {
			endPage = resp.LastPage
		}
	} else if resp.NextPage != 0 {
		endPage = resp.NextPage
	} else {
		endPage = page // no next/last page known: don't render a phantom page+1 button
	}

	for i := startPage; i <= endPage; i++ {
		text := strconv.Itoa(i)
		if i == page {
			text = "· " + text + " ·"
		}
		navRow = append(navRow, Callback(text, pageData(i), WithStyle(StylePrimary)))
	}

	if resp.NextPage != 0 {
		navRow = append(navRow, Callback(">", pageData(resp.NextPage),
			WithStyle(StylePrimary),
			WithCustomEmojiEnv(IconNext),
		))
	}

	return navRow
}

// CompactButtonText truncates a button label to Telegram-friendly length with an ellipsis.
func CompactButtonText(name string) string {
	const max = 42
	if len(name) <= max {
		return name
	}
	return name[:max-1] + "…"
}
