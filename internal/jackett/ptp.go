package jackett

import (
	"regexp"
	"strings"
)

// PTP is PassThePopcorn. Copy this file to add another tracker: declare the
// indexer, and any filter for a tag only that tracker uses.
var PTP = registerIndexer(&Indexer{
	ID:    "passthepopcorn",
	Flag:  "--ptp",
	Label: "PTP",
	Name:  "PassThePopcorn",
	// PTP appends a tag block to every title, as in
	// "Movie.2003.1080p [1080p / Blu-ray / x264 / MKV / Checked]".
	SplitTitle: splitTagBlock,
})

// tagBlockRe matches PTP's trailing tag block. The slash is what identifies it,
// so a bracketed tag that is not a field list is left alone. Brackets do not
// nest, so the character class keeps the match tight.
var tagBlockRe = regexp.MustCompile(`\s*\[[^\[\]]*/[^\[\]]*\]`)

func splitTagBlock(title string) (name, tags string) {
	blocks := tagBlockRe.FindAllString(title, -1)
	if len(blocks) == 0 {
		return strings.TrimSpace(title), ""
	}

	for i, block := range blocks {
		blocks[i] = strings.TrimSpace(block)
	}
	name = strings.TrimSpace(tagBlockRe.ReplaceAllString(title, ""))
	return name, strings.Join(blocks, " ")
}

// GoldenPopcorn keeps only PTP's Golden Popcorn releases. PTP marks them in the
// tag block it appends to a title, so matching the label is enough.
var GoldenPopcorn = registerFilter(&Filter{
	Flag:    "--gp",
	Label:   "GP",
	Help:    "Golden Popcorn only",
	Indexer: PTP,
	keep: func(title string) bool {
		return strings.Contains(title, "Golden Popcorn")
	},
})
