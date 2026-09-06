package main

import (
	"context"
	"errors"
	"fmt"
	"github-webhook/internal/bot/middleware"
	"html"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github-webhook/internal/bot/callbacks"
	"github-webhook/internal/bot/commands"
	"github-webhook/internal/cache"
	"github-webhook/internal/config"
	"github-webhook/internal/db"
	"github-webhook/internal/github"
	"github-webhook/internal/logging"
	"github-webhook/internal/models"
	"github-webhook/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
)

func main() {
	logging.Setup()

	if err := run(); err != nil {
		slog.Error("Application stopped", "error", err)
		os.Exit(1)
	}
}

func run() (runErr error) {
	cfg := config.Load()
	if cfg.GitHubWebhookSecret == "" {
		slog.Warn("GITHUB_WEBHOOK_SECRET is empty — incoming webhook signature verification is DISABLED. Only use this for local development.")
	}
	database, err := db.Connect(cfg)
	if err != nil {
		return fmt.Errorf("connect to DB: %w", err)
	}
	databaseClosed := false
	defer func() {
		if database == nil || databaseClosed {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := database.Client.Disconnect(ctx); err != nil {
			if runErr != nil {
				runErr = errors.Join(runErr, fmt.Errorf("disconnect database: %w", err))
				return
			}
			runErr = fmt.Errorf("disconnect database: %w", err)
		}
	}()

	oauth := github.NewOAuth(cfg)
	clientFactory := github.NewClientFactory()
	oauthStateCache := cache.New[string, int64]()
	contextCache := cache.New[string, models.MessageContext]()
	actionCache := cache.New[string, models.PRActionContext]()

	b, err := gotgbot.NewBot(cfg.TelegramToken, nil)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			slog.Error("Error processing update", "error", err)
			return ext.DispatcherActionNoop
		},
		Logger: slog.Default().With("component", "gotgbot_dispatcher"),
	})
	updater := ext.NewUpdater(dispatcher, &ext.UpdaterOpts{
		Logger: slog.Default().With("component", "gotgbot_updater"),
	})
	dispatcher.AddHandlerToGroup(handlers.NewMessage(nil, middleware.TrackUserAndChat(database)), -1)
	dispatcher.AddHandlerToGroup(handlers.NewCallback(nil, middleware.TrackUserAndChat(database)), -1)

	// Commands
	cmdHandler := commands.NewCommandHandler(cfg, database, oauth, oauthStateCache, clientFactory, cfg.EncryptionKey, contextCache)
	dispatcher.AddHandler(handlers.NewCommand("start", cmdHandler.Start))
	dispatcher.AddHandler(handlers.NewCommand("connect", cmdHandler.Connect))
	dispatcher.AddHandler(handlers.NewCommand("add", cmdHandler.AddRepo))
	dispatcher.AddHandler(handlers.NewCommand("addrepo", cmdHandler.AddRepo))
	dispatcher.AddHandler(handlers.NewCommand("rm", cmdHandler.RemoveRepo))
	dispatcher.AddHandler(handlers.NewCommand("removerepo", cmdHandler.RemoveRepo))
	dispatcher.AddHandler(handlers.NewCommand("repos", cmdHandler.Repos))
	dispatcher.AddHandler(handlers.NewCommand("config", cmdHandler.Settings))
	dispatcher.AddHandler(handlers.NewCommand("settings", cmdHandler.Settings))
	dispatcher.AddHandler(handlers.NewCommand("help", cmdHandler.Help))
	dispatcher.AddHandler(handlers.NewCommand("privacy", cmdHandler.Privacy))
	dispatcher.AddHandler(handlers.NewCommand("logout", cmdHandler.Logout))
	dispatcher.AddHandler(handlers.NewCommand("close", cmdHandler.Close))
	dispatcher.AddHandler(handlers.NewCommand("reopen", cmdHandler.Reopen))
	dispatcher.AddHandler(handlers.NewCommand("approve", cmdHandler.Approve))

	replyHandler := commands.NewReplyHandler(database, clientFactory, cfg.EncryptionKey, contextCache)
	dispatcher.AddHandler(handlers.NewMessage(func(msg *gotgbot.Message) bool {
		if msg.GetText() == "" {
			return false
		}

		ents := msg.GetEntities()
		if len(ents) != 0 && ents[0].Offset == 0 && ents[0].Type == "bot_command" {
			return false
		}

		return msg.ReplyToMessage != nil
	}, replyHandler.HandleReply))

	cbHandler := callbacks.NewCallbackHandler(cfg, database, clientFactory, cfg.EncryptionKey, actionCache)
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("c:"), cbHandler.HandleSettings))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("act:"), cbHandler.HandlePRAction))

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("ok"))
	})
	mux.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		html := fmt.Sprintf(`
		<html>
		<head><title>GitHub Webhook Bot</title></head>
		<body style="font-family: sans-serif; text-align: center; padding: 50px;">
			<h1>GitHub Webhook Bot</h1>
			<p>The bot is running successfully.</p>
			<p><a href="https://t.me/%s" style="text-decoration: none; background-color: #0088cc; color: white; padding: 10px 20px; border-radius: 5px;">Open in Telegram</a></p>
		</body>
		</html>`, b.User.Username)
		writer.Header().Set("Content-Type", "text/html")
		_, _ = writer.Write([]byte(html))
	})

	webhookServer := github.NewWebhookServer(cfg, database, b, contextCache, actionCache)
	mux.HandleFunc("/webhook/", webhookServer.Handler)
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		// GitHub redirects back with ?error=access_denied when the user cancels
		// the authorization screen. Show a friendly page instead of a raw 400.
		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			slog.Info("OAuth authorization denied", "error", oauthErr)
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`
			<html>
			<head><title>Cancelled</title></head>
			<body style="font-family: sans-serif; text-align: center; padding: 50px;">
				<h1>Connection cancelled</h1>
				<p>You declined to connect your GitHub account. Nothing was saved.</p>
				<p><a href="https://t.me/` + b.User.Username + `">Return to Telegram</a> and run /connect again if you change your mind.</p>
			</body>
			</html>`))
			return
		}

		if code == "" {
			http.Error(w, "Missing code", http.StatusBadRequest)
			return
		}

		telegramID, err := resolveOAuthState(state, oauthStateCache, cfg.EncryptionKey)
		if err != nil {
			slog.Warn("OAuth callback rejected", "error", err)
			http.Error(w, "Invalid or expired state. Please return to Telegram and run /connect again.", http.StatusBadRequest)
			return
		}

		oauthStateCache.Delete(state)

		// Do the (slow) GitHub exchange + DB write in the background so we return a
		// fast 200 to the browser. The request context is cancelled when we return,
		// so the goroutine uses its own context with a generous timeout.
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			token, err := oauth.ExchangeCode(ctx, code)
			if err != nil {
				slog.Error("OAuth exchange failed", "telegram_id", telegramID, "error", err)
				_, _ = b.SendMessage(telegramID, "⚠️ GitHub connection failed during token exchange. Please run /connect again.", nil)
				return
			}

			encToken, err := utils.Encrypt(token.AccessToken, cfg.EncryptionKey)
			if err != nil {
				slog.Error("OAuth encrypt failed", "telegram_id", telegramID, "error", err)
				_, _ = b.SendMessage(telegramID, "⚠️ GitHub connection failed while securing your token. Please run /connect again.", nil)
				return
			}

			ghClient := clientFactory.GetUserClient(ctx, token.AccessToken)
			u, _, err := ghClient.Users.Get(ctx, "")
			if err != nil {
				slog.Error("OAuth fetch user failed", "telegram_id", telegramID, "error", err)
				_, _ = b.SendMessage(telegramID, "⚠️ GitHub connection failed while fetching your profile. Please run /connect again.", nil)
				return
			}

			user := &models.User{
				ID:                  telegramID,
				GitHubUserID:        u.GetID(),
				GitHubUsername:      u.GetLogin(),
				EncryptedOAuthToken: encToken,
			}
			if err := database.UpsertUser(ctx, user); err != nil {
				slog.Error("OAuth DB upsert failed", "telegram_id", telegramID, "error", err)
				_, _ = b.SendMessage(telegramID, "⚠️ Connected to GitHub but failed to save your token. Please run /connect again.", &gotgbot.SendMessageOpts{ParseMode: "HTML"})
				return
			}

			_, _ = b.SendMessage(telegramID, fmt.Sprintf("✅ GitHub account <b>%s</b> connected successfully!", html.EscapeString(u.GetLogin())), &gotgbot.SendMessageOpts{ParseMode: "HTML"})
		}()

		htmlBody := fmt.Sprintf(`
		<html>
		<head><title>Connected</title></head>
		<body style="font-family: sans-serif; text-align: center; padding: 50px;">
			<h1>Authentication Successful</h1>
			<p>Your GitHub account has been connected.</p>
			<script>
				window.opener = null;
				setTimeout(function() { window.close(); }, 1000);
				setTimeout(function() { window.location.href = "https://t.me/%s"; }, 2000);
			</script>
			<p>If the window does not close automatically, you can <a href="https://t.me/%s">return to Telegram</a>.</p>
		</body>
		</html>`, b.User.Username, b.User.Username)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(htmlBody))
	})

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", cfg.Port, err)
	}

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		slog.Info("Server listening", "port", cfg.Port)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	// Start Bot (Polling or Webhook)
	if cfg.UsePolling {
		slog.Warn("Running in POLLING mode — not recommended for production. " +
			"Polling breaks if two instances ever run with the same token (Telegram getUpdates conflict) and adds latency. " +
			"Set TELEGRAM_WEBHOOK_URL to your public HTTPS URL to use webhooks instead.")
		// Clear webhook before polling to avoid conflicts
		if ok, err := b.DeleteWebhook(nil); err != nil {
			slog.Warn("Failed to delete webhook before polling", "error", err)
		} else if ok {
			slog.Info("Successfully deleted existing webhook before starting polling")
		}

		go func() {
			for {
				err := updater.StartPolling(b, &ext.PollingOpts{
					DropPendingUpdates: true,
					GetUpdatesOpts: &gotgbot.GetUpdatesOpts{
						Timeout: 9,
						RequestOpts: &gotgbot.RequestOpts{
							Timeout: time.Second * 10,
						},
					},
				})
				if err != nil {
					if strings.Contains(err.Error(), "terminated by other getUpdates request") {
						slog.Warn("Polling conflict detected (expected during deploy); retrying in 2s")
						time.Sleep(2 * time.Second)
						continue
					}
					slog.Error("Polling failed; retrying in 5s", "error", err)
					time.Sleep(5 * time.Second)
					continue
				}
				break
			}
		}()
		slog.Info("Bot started using polling", "bot", b.User.Username)
	} else {
		webhookBase := strings.TrimRight(cfg.TelegramWebhookURL, "/")
		webhookPath := "/bot" + cfg.TelegramToken

		mux.HandleFunc(webhookPath, updater.GetHandlerFunc(""))
		slog.Info("Registered local Telegram webhook handler for /bot<redacted>")

		err = updater.SetAllBotWebhooks(webhookBase, &gotgbot.SetWebhookOpts{
			MaxConnections: 100,
			// Keep updates that arrive while the bot is down (e.g. during a
			// deploy) so commands are not silently lost.
			DropPendingUpdates: false,
		})
		if err != nil {
			slog.Error("Failed to set Telegram webhook; falling back to polling", "error", err)
			go func() {
				for {
					err := updater.StartPolling(b, &ext.PollingOpts{DropPendingUpdates: true})
					if err == nil {
						return
					}
					if strings.Contains(err.Error(), "terminated by other getUpdates request") {
						slog.Warn("Polling conflict detected (expected during deploy); retrying in 2s")
						time.Sleep(2 * time.Second)
						continue
					}
					slog.Error("Polling fallback failed; retrying in 5s", "error", err)
					time.Sleep(5 * time.Second)
				}
			}()
		} else {
			slog.Info("Bot successfully registered webhook at Telegram", "url", webhookBase+"/bot<redacted>")
		}
	}

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// Periodically sweep TTL caches so they do not grow unbounded (entries otherwise
	// only evict on re-access, and most are never read again).
	go func() {
		ticker := time.NewTicker(10 * time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-signalCtx.Done():
				return
			case <-ticker.C:
				database.ChatReposCache.Cleanup()
				oauthStateCache.Cleanup()
				contextCache.Cleanup()
				actionCache.Cleanup()
				webhookServer.DeliverySeen.Cleanup()
				webhookServer.Pacer.Cleanup()
				clientFactory.Cleanup()
				middleware.CleanupChatUpsertSeen()
				utils.CleanupAdminCache()
			}
		}
	}()

	select {
	case <-signalCtx.Done():
		slog.Info("Shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			shutdownErr := shutdown(shutdownCtx, server, updater, database, webhookServer, b)
			databaseClosed = true
			if shutdownErr != nil {
				return errors.Join(fmt.Errorf("server failed: %w", err), fmt.Errorf("shutdown after server failure: %w", shutdownErr))
			}
			return fmt.Errorf("server failed: %w", err)
		}
		slog.Info("Server stopped")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = shutdown(shutdownCtx, server, updater, database, webhookServer, b)
	databaseClosed = true
	if err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	slog.Info("Shutdown complete")
	return nil
}

func shutdown(ctx context.Context, server *http.Server, updater *ext.Updater, database *db.DB, webhookServer *github.WebhookServer, bot *gotgbot.Bot) error {
	var errs []error

	if err := updater.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop updater: %w", err))
	}

	if err := server.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown server: %w", err))
	}

	// Ask Telegram to close the bot's session so pending HTTP requests are
	// released promptly instead of lingering until timeout.
	if _, err := bot.Close(nil); err != nil {
		slog.Debug("Bot.Close failed", "error", err)
	}

	webhookWaitCh := make(chan struct{})
	go func() {
		webhookServer.Wg.Wait()
		close(webhookWaitCh)
	}()

	select {
	case <-ctx.Done():
		errs = append(errs, fmt.Errorf("webhook wait: %w", ctx.Err()))
	case <-webhookWaitCh:
	}

	if err := database.Client.Disconnect(ctx); err != nil {
		errs = append(errs, fmt.Errorf("disconnect database: %w", err))
	}

	return errors.Join(errs...)
}

func resolveOAuthState(state string, stateCache *cache.Cache[string, int64], encryptionKey string) (int64, error) {
	if state == "" {
		return 0, fmt.Errorf("missing state")
	}

	if telegramID, ok := stateCache.Get(state); ok {
		return telegramID, nil
	}

	decrypted, err := utils.Decrypt(state, encryptionKey)
	if err != nil {
		return 0, fmt.Errorf("decrypt state: %w", err)
	}

	parts := strings.Split(decrypted, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid state format")
	}

	telegramID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || telegramID == 0 {
		return 0, fmt.Errorf("invalid telegram id")
	}

	createdAt, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid state timestamp")
	}

	if time.Since(time.Unix(createdAt, 0)) > 10*time.Minute {
		return 0, fmt.Errorf("state expired")
	}

	return telegramID, nil
}
