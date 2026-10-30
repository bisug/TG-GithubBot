package utils

import (
	"errors"
	"fmt"
	"testing"

	"github.com/AshokShau/gotdbot"
)

func TestIsIgnorableTelegramError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "pointer gotdbot.Error with MESSAGE_NOT_MODIFIED",
			err:  &gotdbot.Error{Code: 400, Message: "MESSAGE_NOT_MODIFIED"},
			want: true,
		},
		{
			name: "value gotdbot.Error with MESSAGE_NOT_MODIFIED",
			err:  gotdbot.Error{Code: 400, Message: "MESSAGE_NOT_MODIFIED"},
			want: true,
		},
		{
			name: "pointer gotdbot.Error with Canceled by new edit message request",
			err:  &gotdbot.Error{Code: 400, Message: "Canceled by new edit message request"},
			want: true,
		},
		{
			name: "wrapped value gotdbot.Error",
			err:  fmt.Errorf("edit failed: %w", gotdbot.Error{Code: 400, Message: "MESSAGE_NOT_MODIFIED"}),
			want: true,
		},
		{
			name: "formatted pointer gotdbot.Error",
			err:  fmt.Errorf("edit failed: %v", &gotdbot.Error{Code: 400, Message: "MESSAGE_NOT_MODIFIED"}),
			want: true,
		},
		{
			name: "message is not modified lowercase variation",
			err:  &gotdbot.Error{Code: 400, Message: "Bad Request: message is not modified"},
			want: true,
		},
		{
			name: "query_id_invalid",
			err:  &gotdbot.Error{Code: 400, Message: "QUERY_ID_INVALID"},
			want: true,
		},
		{
			name: "generic error with message_not_modified string",
			err:  errors.New("failed: MESSAGE_NOT_MODIFIED"),
			want: true,
		},
		{
			name: "unrelated telegram error",
			err:  &gotdbot.Error{Code: 400, Message: "CHAT_NOT_FOUND"},
			want: false,
		},
		{
			name: "unrelated standard error",
			err:  errors.New("database connection refused"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsIgnorableTelegramError(tt.err); got != tt.want {
				t.Errorf("IsIgnorableTelegramError() = %v, want %v", got, tt.want)
			}
		})
	}
}
