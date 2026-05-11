package ui

import (
	"log"
	"os"
	"strings"

	"github.com/PaulSonOfLars/gotgbot/v2"
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
