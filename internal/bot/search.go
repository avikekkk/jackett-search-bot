package bot

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	tdhtml "github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/markup"
	messagepeer "github.com/gotd/td/telegram/message/peer"
	"github.com/gotd/td/tg"

	"github.com/avisek/jackett-search-bot/internal/jackett"
)

const (
	maxSearchSessions = 100
	// sessionTTL drops result sets nobody is paging through any more, so a busy
	// chat cannot pin every indexer response in memory.
	sessionTTL = time.Hour
)

// searchSession keeps one user's result set alive across pagination taps.
type searchSession struct {
	results   []*jackett.Release
	query     string
	opts      jackett.Options
	userID    int64
	chatID    int64
	createdAt time.Time
}

// sessionStore is a bounded FIFO cache of search sessions.
type sessionStore struct {
	mu    sync.Mutex
	max   int
	order []string
	items map[string]*searchSession
}

func newSessionStore(max int) *sessionStore {
	return &sessionStore{max: max, items: map[string]*searchSession{}}
}

func (s *sessionStore) put(token string, session *searchSession) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpired()
	s.items[token] = session
	s.order = append(s.order, token)
	for len(s.order) > s.max {
		oldest := s.order[0]
		s.order = s.order[1:]
		delete(s.items, oldest)
	}
}

func (s *sessionStore) get(token string) (*searchSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpired()
	session, ok := s.items[token]
	return session, ok
}

func (s *sessionStore) remove(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.delete(token)
}

// pruneExpired drops sessions past their TTL. Callers hold the lock.
func (s *sessionStore) pruneExpired() {
	for _, token := range append([]string(nil), s.order...) {
		session, ok := s.items[token]
		if !ok || time.Since(session.createdAt) > sessionTTL {
			s.delete(token)
		}
	}
}

// delete removes a token from both the map and the FIFO order. Callers hold the lock.
func (s *sessionStore) delete(token string) {
	delete(s.items, token)
	for i, existing := range s.order {
		if existing == token {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func newSessionToken() string {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		return "00000000"
	}
	return hex.EncodeToString(buf)
}

func (r *request) handleRelease(ctx context.Context) {
	if r.rejectUnauthorizedSearch(ctx) {
		return
	}

	query, opts, err := parseReleaseArgs(r.args)
	if err != nil {
		r.replyLogged(ctx, err.Error())
		return
	}
	if query == "" {
		r.replyLogged(ctx, "Provide a query or IMDb ID/link")
		return
	}

	log := r.bot.log.With("context", r.context(), "query", query,
		"flags", strings.Join(opts.Labels(), ","))
	log.Info("Search requested")

	searchingMsgID, err := r.reply(ctx, "Searching")
	if err != nil {
		log.Warn("Failed to post status message", "error", err)
		return
	}

	results, err := r.bot.jackett.Search(ctx, query, opts)
	if err != nil {
		var jackettErr *jackett.Error
		if errors.As(err, &jackettErr) {
			log.Warn("Search failed", "reason", jackettErr.Error())
			r.editLogged(ctx, searchingMsgID, html.EscapeString(jackettErr.Error()))
			return
		}
		if ctx.Err() != nil {
			log.Warn("Search interrupted by shutdown")
			r.editLogged(ctx, searchingMsgID, errInterrupted)
			return
		}
		log.Error("Unhandled search failure", "error", err)
		r.editLogged(ctx, searchingMsgID, errUnexpected)
		return
	}
	sortByResolution(results)

	if len(results) == 0 {
		log.Info("Search completed", "results", 0)
		// Nothing to page through, so the header and the echoed query would only
		// be noise around a one-line answer.
		r.editLogged(ctx, searchingMsgID, "No results")
		return
	}

	token := newSessionToken()
	r.bot.sessions.put(token, &searchSession{
		results:   results,
		query:     query,
		opts:      opts,
		userID:    r.userID(),
		chatID:    r.chatID(),
		createdAt: time.Now(),
	})

	pageSize := r.bot.cfg.MaxResults
	text, totalPages, page := buildSearchPageText(results, query, opts, 0, pageSize)
	log.Info("Search completed", "results", len(results))
	if err := r.edit(ctx, searchingMsgID, text, buildSearchPageMarkup(token, page, totalPages)); err != nil {
		log.Warn("Failed to show search results", "error", err)
		return
	}
	r.scheduleRedact(ctx, searchingMsgID, token)
}

// parseReleaseArgs splits the query from the flags, which may appear anywhere
// in the command. Flags come from the indexer registry, so a new tracker needs
// no change here. A filter flag fails unless its own indexer was asked for too.
func parseReleaseArgs(args []string) (string, jackett.Options, error) {
	var opts jackett.Options
	var terms []string

	for _, arg := range args {
		if indexer, ok := jackett.IndexerByFlag(arg); ok {
			if !containsIndexer(opts.Indexers, indexer) {
				opts.Indexers = append(opts.Indexers, indexer)
			}
			continue
		}
		if filter, ok := jackett.FilterByFlag(arg); ok {
			if !containsFilter(opts.Filters, filter) {
				opts.Filters = append(opts.Filters, filter)
			}
			continue
		}
		terms = append(terms, arg)
	}

	for _, filter := range opts.Filters {
		if !containsIndexer(opts.Indexers, filter.Indexer) {
			return "", jackett.Options{}, fmt.Errorf(
				"%s only works with %s", codeBlock(filter.Flag), codeBlock(filter.Indexer.Flag))
		}
	}

	return strings.TrimSpace(strings.Join(terms, " ")), opts, nil
}

func containsIndexer(list []*jackett.Indexer, want *jackett.Indexer) bool {
	for _, indexer := range list {
		if indexer == want {
			return true
		}
	}
	return false
}

func containsFilter(list []*jackett.Filter, want *jackett.Filter) bool {
	for _, filter := range list {
		if filter == want {
			return true
		}
	}
	return false
}

// sortByResolution puts 1080p releases first and 2160p next, since those are
// the ones worth grabbing, and orders each group smallest first.
func sortByResolution(results []*jackett.Release) {
	sort.SliceStable(results, func(i, j int) bool {
		left, right := resolutionRank(results[i].Title), resolutionRank(results[j].Title)
		if left != right {
			return left < right
		}
		return results[i].SizeBytes < results[j].SizeBytes
	})
}

func resolutionRank(title string) int {
	lowered := strings.ToLower(title)
	switch {
	case strings.Contains(lowered, "1080p"):
		return 0
	case strings.Contains(lowered, "2160p"):
		return 1
	default:
		return 2
	}
}

// redactedMessage is shown once results are hidden, manually or on a timer.
const redactedMessage = "RESULTS REDACTED"

// clearKeyboard removes the inline keyboard from a message. An edit that sends
// no markup drops the old one; sending an empty ReplyInlineMarkup instead is
// rejected outright with REPLY_MARKUP_INVALID.
func clearKeyboard() tg.ReplyMarkupClass { return nil }

// scheduleRedact hides a results message after SEARCH_REDACT_SECONDS so release
// names do not linger in chat history.
func (r *request) scheduleRedact(ctx context.Context, msgID int, token string) {
	lifetime := r.bot.cfg.SearchRedact
	if lifetime <= 0 {
		return
	}

	// Deliberately not tracked by the shutdown WaitGroup: nothing should wait
	// minutes for this timer when the bot is stopping.
	go func() {
		timer := time.NewTimer(lifetime)
		defer timer.Stop()

		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		r.bot.sessions.remove(token)
		// If the requester already closed it, this edit is a no-op.
		if err := r.edit(ctx, msgID, redactedMessage, clearKeyboard()); err != nil && !isNotModified(err) {
			r.bot.log.Warn("Failed to auto-redact search results", "error", err)
		}
	}()
}

// searchHeader names the filters a result set was built with, so a page still
// says what it is once it has scrolled up the chat.
func searchHeader(opts jackett.Options) string {
	labels := opts.Labels()
	if len(labels) == 0 {
		return header("SEARCH RESULTS")
	}
	return header("SEARCH RESULTS (" + strings.Join(labels, ", ") + ")")
}

// buildSearchPageText renders one page of results, clamping the page number.
func buildSearchPageText(
	results []*jackett.Release,
	query string,
	opts jackett.Options,
	page, pageSize int,
) (string, int, int) {
	if pageSize < 1 {
		pageSize = 1
	}

	totalCount := len(results)
	totalPages := (totalCount + pageSize - 1) / pageSize
	if totalPages < 1 {
		totalPages = 1
	}
	if page < 0 {
		page = 0
	}
	if page > totalPages-1 {
		page = totalPages - 1
	}

	start := page * pageSize
	end := start + pageSize
	if end > totalCount {
		end = totalCount
	}

	lines := []string{
		searchHeader(opts),
		codeBlock(query),
		fmt.Sprintf("<b>Page %d/%d | Total: %d</b>\n", page+1, totalPages, totalCount),
	}
	for _, release := range results[start:end] {
		// Only the release name is monospaced; the tracker's tag block and the
		// size read as ordinary text on the same line beside it.
		name, tags := release.SplitTitle()
		line := codeBlock(name)
		if tags != "" {
			line += " " + html.EscapeString(tags)
		}
		lines = append(lines, line+" • "+html.EscapeString(release.Size)+"\n")
	}

	return strings.Join(lines, "\n"), totalPages, page
}

func buildSearchPageMarkup(token string, page, totalPages int) tg.ReplyMarkupClass {
	var nav []tg.KeyboardButtonClass
	if page > 0 {
		nav = append(nav, markup.Callback("PREV", []byte("jks:"+token+":"+strconv.Itoa(page-1))))
	}
	nav = append(nav, markup.Callback(
		fmt.Sprintf("%d/%d", page+1, totalPages),
		[]byte("jksnoop"),
	))
	if page < totalPages-1 {
		nav = append(nav, markup.Callback("NEXT", []byte("jks:"+token+":"+strconv.Itoa(page+1))))
	}

	return markup.InlineKeyboard(
		tg.KeyboardButtonRow{Buttons: nav},
		tg.KeyboardButtonRow{Buttons: []tg.KeyboardButtonClass{
			markup.Callback("CLOSE", []byte("jksc:"+token)),
		}},
	)
}

func (b *Bot) onCallbackQuery(e tg.Entities, u *tg.UpdateBotCallbackQuery) error {
	data := string(u.Data)
	switch {
	case data == "jksnoop":
		b.goHandle("callback", func(ctx context.Context) {
			b.answerCallback(ctx, u.QueryID, "", false)
		})
	case strings.HasPrefix(data, "jksc:"):
		b.goHandle("callback-close", func(ctx context.Context) {
			b.closeSearch(ctx, e, u, strings.TrimPrefix(data, "jksc:"))
		})
	case strings.HasPrefix(data, "jks:"):
		b.goHandle("callback-page", func(ctx context.Context) {
			b.paginateSearch(ctx, e, u, data)
		})
	}
	return nil
}

// callbackSession resolves the session behind a tap and reports whether the
// tapper may act on it. The owner can drive anyone's results; everybody else is
// limited to their own, in the chat the search was run in. A search run by an
// anonymous group admin has no user ID to match, so anyone in that chat may
// drive it: otherwise the requester could never page or close their own results.
func (b *Bot) callbackSession(
	ctx context.Context,
	u *tg.UpdateBotCallbackQuery,
	token, denied string,
) (*searchSession, bool) {
	session, ok := b.sessions.get(token)
	if !ok {
		b.answerCallback(ctx, u.QueryID, "Session expired", true)
		return nil, false
	}
	if session.userID != 0 && u.UserID != session.userID && u.UserID != b.cfg.OwnerID {
		b.answerCallback(ctx, u.QueryID, denied, true)
		return nil, false
	}
	if callbackChatID(u.Peer) != session.chatID {
		b.answerCallback(ctx, u.QueryID, "Wrong chat for this search", true)
		return nil, false
	}
	return session, true
}

// callbackChatID renders a callback's peer in the same Bot API numbering the
// session was stored with.
func callbackChatID(peer tg.PeerClass) int64 {
	switch p := peer.(type) {
	case *tg.PeerUser:
		return p.UserID
	case *tg.PeerChat:
		return -p.ChatID
	case *tg.PeerChannel:
		return -1000000000000 - p.ChannelID
	}
	return 0
}

func (b *Bot) closeSearch(ctx context.Context, e tg.Entities, u *tg.UpdateBotCallbackQuery, token string) {
	if _, ok := b.callbackSession(ctx, u, token, "Only requester can close this."); !ok {
		return
	}

	b.sessions.remove(token)
	if err := b.editCallbackMessage(ctx, e, u, redactedMessage, clearKeyboard()); err != nil && !isNotModified(err) {
		b.log.Warn("Failed to redact search results", "error", err)
	}
	b.answerCallback(ctx, u.QueryID, "Closed", false)
}

func (b *Bot) paginateSearch(ctx context.Context, e tg.Entities, u *tg.UpdateBotCallbackQuery, data string) {
	parts := strings.Split(data, ":")
	if len(parts) != 3 {
		b.answerCallback(ctx, u.QueryID, "", false)
		return
	}

	token, pageRaw := parts[1], parts[2]
	session, ok := b.callbackSession(ctx, u, token, "Only requester can change pages.")
	if !ok {
		return
	}

	page, err := strconv.Atoi(pageRaw)
	if err != nil {
		b.answerCallback(ctx, u.QueryID, "", false)
		return
	}

	text, totalPages, safePage := buildSearchPageText(
		session.results, session.query, session.opts, page, b.cfg.MaxResults)
	if err := b.editCallbackMessage(ctx, e, u, text, buildSearchPageMarkup(token, safePage, totalPages)); err != nil {
		if !isNotModified(err) {
			b.log.Warn("Failed to edit search page", "error", err)
		}
	}
	b.answerCallback(ctx, u.QueryID, "", false)
}

func (b *Bot) editCallbackMessage(
	ctx context.Context,
	e tg.Entities,
	u *tg.UpdateBotCallbackQuery,
	text string,
	replyMarkup tg.ReplyMarkupClass,
) error {
	inputPeer, err := messagepeer.EntitiesFromUpdate(e).ExtractPeer(u.Peer)
	if err != nil {
		return err
	}

	ctx, cancel := b.reporting(ctx)
	defer cancel()
	builder := b.sender.To(inputPeer).NoWebpage()
	if replyMarkup != nil {
		builder = builder.Markup(replyMarkup)
	}
	_, err = builder.Edit(u.MsgID).StyledText(ctx, tdhtml.String(nil, text))
	return err
}

func (b *Bot) answerCallback(ctx context.Context, queryID int64, text string, alert bool) {
	ctx, cancel := b.reporting(ctx)
	defer cancel()
	req := &tg.MessagesSetBotCallbackAnswerRequest{QueryID: queryID}
	if text != "" {
		req.SetMessage(text)
	}
	if alert {
		req.SetAlert(true)
	}
	if _, err := b.api.MessagesSetBotCallbackAnswer(ctx, req); err != nil {
		b.log.Warn("Failed to answer callback query", "error", err)
	}
}
