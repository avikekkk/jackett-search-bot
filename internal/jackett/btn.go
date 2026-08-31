package jackett

// BTN is BroadcasTheNet. It has no filters of its own; its titles are plain
// release names with no tag block.
var BTN = registerIndexer(&Indexer{
	ID:    "broadcasthenet",
	Flag:  "--btn",
	Label: "BTN",
	Name:  "BroadcasTheNet",
	Order: 2,
})
