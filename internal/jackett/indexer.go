package jackett

import (
	"sort"
	"strings"
)

// Indexer is one tracker configured in Jackett, described in one place so a new
// one is a single small file: see ptp.go for the shape to copy.
type Indexer struct {
	// ID is Jackett's own indexer ID, which is also the path segment that
	// scopes a torznab call to this tracker alone.
	ID string
	// Flag is the /r flag that selects it, such as "--ptp".
	Flag string
	// Label names it in a results header, kept short since several can appear.
	Label string
	// Name is the tracker's full name, shown in /help.
	Name string
	// Order sorts the tracker in /help. Without it the listing would follow
	// Go's file initialization order, which is alphabetical by file name and so
	// changes as trackers are added.
	Order int
	// SplitTitle separates a release name from whatever the tracker appends to
	// it, so the two can be rendered differently. Trackers that publish plain
	// release names leave it nil.
	SplitTitle func(title string) (name, tags string)
}

// Filter narrows results within one indexer, for a tag only that tracker uses.
type Filter struct {
	// Flag is the /r flag that turns it on.
	Flag string
	// Label names it in a results header.
	Label string
	// Help describes it in /help.
	Help string
	// Indexer is the tracker the flag belongs to. The flag is rejected unless
	// that indexer was asked for too, since it means nothing elsewhere.
	Indexer *Indexer
	// keep reports whether a release title survives the filter.
	keep func(title string) bool
}

// The registries. Each indexer and filter adds itself here from its own file,
// so nothing central has to be edited to add a tracker.
var (
	indexers []*Indexer
	filters  []*Filter
)

// registerIndexer adds an indexer to the registry and returns it, so a file can
// declare and register it in one statement.
func registerIndexer(indexer *Indexer) *Indexer {
	indexers = append(indexers, indexer)
	return indexer
}

// registerFilter adds a filter to the registry and returns it.
func registerFilter(filter *Filter) *Filter {
	filters = append(filters, filter)
	return filter
}

// Indexers returns every registered indexer, in listing order.
func Indexers() []*Indexer {
	sorted := append([]*Indexer(nil), indexers...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Order < sorted[j].Order })
	return sorted
}

// Filters returns every registered filter, ordered by the indexer each belongs
// to, so a tracker's flags stay together under it.
func Filters() []*Filter {
	sorted := append([]*Filter(nil), filters...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Indexer.Order < sorted[j].Indexer.Order
	})
	return sorted
}

// IndexerByFlag resolves a command flag to its indexer, ignoring case.
func IndexerByFlag(flag string) (*Indexer, bool) {
	for _, indexer := range indexers {
		if strings.EqualFold(flag, indexer.Flag) {
			return indexer, true
		}
	}
	return nil, false
}

// FilterByFlag resolves a command flag to its filter, ignoring case.
func FilterByFlag(flag string) (*Filter, bool) {
	for _, filter := range filters {
		if strings.EqualFold(flag, filter.Flag) {
			return filter, true
		}
	}
	return nil, false
}

// IndexerByID resolves a feed item's jackettindexer ID to its indexer. An
// indexer configured in Jackett but not registered here is unknown, which is
// fine: it is searched, just without any tracker-specific handling.
func IndexerByID(id string) (*Indexer, bool) {
	for _, indexer := range indexers {
		if indexer.ID == id {
			return indexer, true
		}
	}
	return nil, false
}

// Options narrows a search. An empty Indexers searches every tracker Jackett
// has configured.
type Options struct {
	Indexers []*Indexer
	Filters  []*Filter
}

// Labels names the active flags, for a results header.
func (o Options) Labels() []string {
	var labels []string
	for _, indexer := range o.Indexers {
		labels = append(labels, indexer.Label)
	}
	for _, filter := range o.Filters {
		labels = append(labels, filter.Label)
	}
	return labels
}

// endpointIndexer is the path segment a search runs against. A single requested
// indexer is queried directly, so the other trackers are never hit; anything
// else fans out and is filtered from the feed.
func (o Options) endpointIndexer() string {
	if len(o.Indexers) == 1 {
		return o.Indexers[0].ID
	}
	return "all"
}

// wantsIndexer reports whether a feed item from indexerID belongs in the
// results. One indexer needs no check, since the endpoint already scoped it.
func (o Options) wantsIndexer(indexerID string) bool {
	if len(o.Indexers) < 2 {
		return true
	}
	for _, indexer := range o.Indexers {
		if indexer.ID == indexerID {
			return true
		}
	}
	return false
}

// wantsTitle reports whether a release survives every active filter.
func (o Options) wantsTitle(title string) bool {
	for _, filter := range o.Filters {
		if !filter.keep(title) {
			return false
		}
	}
	return true
}
