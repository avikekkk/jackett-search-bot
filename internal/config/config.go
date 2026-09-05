// Package config loads and validates bot configuration from the environment.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

// DefaultSearchRedact is used when SEARCH_REDACT_SECONDS is not set.
const DefaultSearchRedact = 300 * time.Second

// DefaultMaxResults is how many releases one page shows when MAX_RESULTS is not set.
const DefaultMaxResults = 10

// DefaultDatabasePath is used when DATABASE_PATH is not set.
const DefaultDatabasePath = "jackettbot.db"

// Error is returned when required environment configuration is missing or invalid.
type Error struct {
	msg string
}

func (e *Error) Error() string { return e.msg }

func configErrorf(format string, args ...any) error {
	return &Error{msg: fmt.Sprintf(format, args...)}
}

// Bot holds every setting the bot needs to run.
type Bot struct {
	TelegramAPIID     int
	TelegramAPIHash   string
	TelegramBotToken  string
	OwnerID           int64
	AuthorizedChatIDs []int64

	// JackettURL is the base URL of the Jackett instance, without a trailing slash.
	JackettURL    string
	JackettAPIKey string

	// MaxResults is the page size of a /r result message.
	MaxResults int

	// SearchRedact is how long a search result message stays visible before it
	// is redacted automatically. Zero disables auto-redaction.
	SearchRedact time.Duration

	// DatabasePath is the SQLite file holding runtime authorizations.
	DatabasePath string
}

// IsAuthorizedChat reports whether the chat is allowed to run /r.
func (c *Bot) IsAuthorizedChat(chatID int64) bool {
	for _, id := range c.AuthorizedChatIDs {
		if id == chatID {
			return true
		}
	}
	return false
}

func clean(value string) string { return strings.TrimSpace(value) }

func env(name string) string { return clean(os.Getenv(name)) }

// parseSearchRedact reads a whole number of seconds; 0 disables auto-redaction.
func parseSearchRedact(value string) (time.Duration, error) {
	if value == "" {
		return DefaultSearchRedact, nil
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds < 0 {
		return 0, configErrorf("SEARCH_REDACT_SECONDS must be a non-negative number of seconds")
	}
	return time.Duration(seconds) * time.Second, nil
}

func parseMaxResults(value string) (int, error) {
	if value == "" {
		return DefaultMaxResults, nil
	}
	count, err := strconv.Atoi(value)
	if err != nil || count < 1 {
		return 0, configErrorf("MAX_RESULTS must be a whole number of results per page, at least 1")
	}
	return count, nil
}

func parseAuthorizedChatIDs(value string) ([]int64, error) {
	if value == "" {
		return nil, nil
	}

	var chatIDs []int64
	for _, part := range strings.Split(strings.ReplaceAll(value, "\n", ","), ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, configErrorf("AUTHORIZED_CHAT_IDS must be a comma-separated list of chat IDs")
		}
		chatIDs = append(chatIDs, id)
	}
	return chatIDs, nil
}

// parseJackettURL keeps only the scheme and host part the torznab endpoint is
// appended to, so a trailing slash or a pasted API path does not break lookups.
func parseJackettURL(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", configErrorf("JACKETT_URL must be an absolute URL, such as http://localhost:9117")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

// Load reads .env plus the process environment and returns a validated config.
func Load() (*Bot, error) {
	// A missing .env is fine when the environment is already populated.
	_ = godotenv.Load()

	telegramAPIIDRaw := env("TELEGRAM_API_ID")
	telegramAPIHash := env("TELEGRAM_API_HASH")
	telegramBotToken := env("TELEGRAM_BOT_TOKEN")
	ownerIDRaw := env("OWNER_ID")
	jackettURLRaw := env("JACKETT_URL")
	jackettAPIKey := env("JACKETT_API_KEY")

	if telegramAPIIDRaw == "" {
		return nil, configErrorf("Missing TELEGRAM_API_ID in .env")
	}
	if telegramAPIHash == "" {
		return nil, configErrorf("Missing TELEGRAM_API_HASH in .env")
	}
	if telegramBotToken == "" {
		return nil, configErrorf("Missing TELEGRAM_BOT_TOKEN in .env")
	}
	if ownerIDRaw == "" {
		return nil, configErrorf("Missing OWNER_ID in .env")
	}
	if jackettURLRaw == "" {
		return nil, configErrorf("Missing JACKETT_URL in .env")
	}
	if jackettAPIKey == "" {
		return nil, configErrorf("Missing JACKETT_API_KEY in .env")
	}

	telegramAPIID, err := strconv.Atoi(telegramAPIIDRaw)
	if err != nil {
		return nil, configErrorf("TELEGRAM_API_ID must be an integer")
	}

	ownerID, err := strconv.ParseInt(ownerIDRaw, 10, 64)
	if err != nil {
		return nil, configErrorf("OWNER_ID must be a Telegram numeric user ID")
	}

	jackettURL, err := parseJackettURL(jackettURLRaw)
	if err != nil {
		return nil, err
	}

	authorizedChatIDs, err := parseAuthorizedChatIDs(env("AUTHORIZED_CHAT_IDS"))
	if err != nil {
		return nil, err
	}

	maxResults, err := parseMaxResults(env("MAX_RESULTS"))
	if err != nil {
		return nil, err
	}

	searchRedact, err := parseSearchRedact(env("SEARCH_REDACT_SECONDS"))
	if err != nil {
		return nil, err
	}

	databasePath := env("DATABASE_PATH")
	if databasePath == "" {
		databasePath = DefaultDatabasePath
	}

	return &Bot{
		TelegramAPIID:     telegramAPIID,
		TelegramAPIHash:   telegramAPIHash,
		TelegramBotToken:  telegramBotToken,
		OwnerID:           ownerID,
		AuthorizedChatIDs: authorizedChatIDs,
		JackettURL:        jackettURL,
		JackettAPIKey:     jackettAPIKey,
		MaxResults:        maxResults,
		SearchRedact:      searchRedact,
		DatabasePath:      databasePath,
	}, nil
}
