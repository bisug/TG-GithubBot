package main

import (
	"context"
	"errors"
	"fmt"
	"github-webhook/internal/bot/middleware"
	"log"
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
	"github-webhook/internal/models"
	"github-webhook/internal/utils"

	"github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/callbackquery"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("Application stopped: %v", err)
	}
}

func run() (runErr error) {
	cfg := config.Load()
	database, err := db.Connect(cfg)
	if err != nil {
		return fmt.Errorf("connect to DB: %w", err)
	}
	databaseClosed := false
	defer func() {
		if databaseClosed {
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
	adminCache := cache.New[int64, []int64]()
	reloadRateLimit := cache.New[int64, time.Time]()

	b, err := gotgbot.NewBot(cfg.TelegramToken, nil)
	if err != nil {
		return fmt.Errorf("create bot: %w", err)
	}

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(b *gotgbot.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Printf("Error processing update: %v", err)
			return ext.DispatcherActionNoop
		},
	})
	updater := ext.NewUpdater(dispatcher, nil)
	dispatcher.AddHandlerToGroup(handlers.NewMessage(nil, middleware.TrackUserAndChat(database)), -1)

	// Commands
	cmdHandler := commands.NewCommandHandler(cfg, database, oauth, oauthStateCache, clientFactory, cfg.EncryptionKey, contextCache, adminCache, reloadRateLimit)
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
	dispatcher.AddHandler(handlers.NewCommand("reload", cmdHandler.Reload))
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

	cbHandler := callbacks.NewCallbackHandler(cfg, database, clientFactory, cfg.EncryptionKey, actionCache, adminCache)
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("c:"), cbHandler.HandleSettings))
	dispatcher.AddHandler(handlers.NewCallback(callbackquery.Prefix("act:"), cbHandler.HandlePRAction))

	mux := http.NewServeMux()
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

	webhookHandler := github.NewWebhookServer(cfg, database, b, contextCache, actionCache).Handler
	mux.HandleFunc("/webhook/", webhookHandler)
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		if code == "" {
			http.Error(w, "Missing code", http.StatusBadRequest)
			return
		}

		telegramID, err := resolveOAuthState(state, oauthStateCache, cfg.EncryptionKey)
		if err != nil {
			log.Printf("OAuth callback rejected: %v", err)
			http.Error(w, "Invalid or expired state. Please return to Telegram and run /connect again.", http.StatusBadRequest)
			return
		}

		oauthStateCache.Delete(state)
		token, err := oauth.ExchangeCode(context.Background(), code)
		if err != nil {
			http.Error(w, "Failed to exchange code", http.StatusInternalServerError)
			return
		}

		encToken, err := utils.Encrypt(token.AccessToken, cfg.EncryptionKey)
		if err != nil {
			http.Error(w, "Encryption failed", http.StatusInternalServerError)
			return
		}

		ghClient := clientFactory.GetUserClient(context.Background(), token.AccessToken)
		u, _, err := ghClient.Users.Get(context.Background(), "")
		if err != nil {
			http.Error(w, "Failed to fetch user", http.StatusInternalServerError)
			return
		}

		user := &models.User{
			ID:                  telegramID,
			GitHubUserID:        u.GetID(),
			GitHubUsername:      u.GetLogin(),
			EncryptedOAuthToken: encToken,
		}
		if err := database.UpsertUser(context.Background(), user); err != nil {
			http.Error(w, "DB Error", http.StatusInternalServerError)
			return
		}

		_, _ = b.SendMessage(telegramID, fmt.Sprintf("✅ GitHub account <b>%s</b> connected successfully!", u.GetLogin()), &gotgbot.SendMessageOpts{ParseMode: "HTML"})

		html := fmt.Sprintf(`
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
		_, _ = w.Write([]byte(html))
	})

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("listen on port %s: %w", cfg.Port, err)
	}

	server := &http.Server{
		Handler: mux,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Printf("Server listening on port %s", cfg.Port)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	// Start Bot (Polling or Webhook)
	if cfg.UsePolling {
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
						log.Printf("Polling conflict detected (expected during deploy), retrying in 2s...")
						time.Sleep(2 * time.Second)
						continue
					}
					log.Printf("Polling failed: %v. Retrying in 5s...", err)
					time.Sleep(5 * time.Second)
					continue
				}
				break
			}
		}()
		log.Printf("Bot started using Polling: @%s", b.User.Username)
	} else {
		webhookBase := strings.TrimRight(cfg.TelegramWebhookURL, "/")
		webhookPath := "/tg-webhook/" + cfg.TelegramToken
		fullWebhookURL := webhookBase + webhookPath
		
		mux.HandleFunc(webhookPath, updater.GetHandlerFunc(webhookPath))
		log.Printf("Registered local Telegram webhook handler for %s", webhookPath)

		err = updater.SetAllBotWebhooks(webhookBase, &gotgbot.SetWebhookOpts{
			MaxConnections:     100,
			DropPendingUpdates: true,
		})
		if err != nil {
			log.Printf("ERROR: Failed to set Telegram webhook: %v. Falling back to polling...", err)
			go func() {
				if err := updater.StartPolling(b, &ext.PollingOpts{DropPendingUpdates: true}); err != nil {
					log.Printf("FATAL: Polling fallback failed: %v", err)
				}
			}()
		} else {
			log.Printf("✅ Bot successfully registered Webhook at Telegram: %s", fullWebhookURL)
		}
	}

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		log.Printf("Shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			shutdownErr := shutdown(shutdownCtx, server, updater, database)
			databaseClosed = true
			if shutdownErr != nil {
				return errors.Join(fmt.Errorf("server failed: %w", err), fmt.Errorf("shutdown after server failure: %w", shutdownErr))
			}
			return fmt.Errorf("server failed: %w", err)
		}
		log.Printf("Server stopped")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	err = shutdown(shutdownCtx, server, updater, database)
	databaseClosed = true
	if err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}

	log.Printf("Shutdown complete")
	return nil
}

func shutdown(ctx context.Context, server *http.Server, updater *ext.Updater, database *db.DB) error {
	var errs []error

	if err := updater.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("stop updater: %w", err))
	}

	if err := server.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("shutdown server: %w", err))
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
