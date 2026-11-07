package unibox

import (
	"math/rand/v2"
	"strconv"
)

// taskAdjectives and taskNouns make a task with no repository a name to
// say and remember, as a container's is: nothing of it tells two such
// tasks apart otherwise.
var taskAdjectives = []string{
	"amber", "bold", "brave", "bright", "brisk", "calm", "clever", "cosy", "crisp", "curious",
	"daring", "deft", "eager", "fair", "fancy", "fond", "gentle", "glad", "golden", "grand",
	"happy", "hardy", "honest", "jolly", "keen", "kind", "lively", "lucky", "merry", "mellow",
	"misty", "modest", "nimble", "noble", "patient", "plucky", "polite", "proud", "quick", "quiet",
	"rapid", "ready", "rosy", "silent", "snowy", "steady", "sunny", "swift", "tidy", "witty",
}

var taskNouns = []string{
	"badger", "beacon", "birch", "brook", "cedar", "comet", "coral", "crane", "delta", "falcon",
	"fern", "finch", "fjord", "gecko", "glacier", "harbor", "heron", "island", "lantern", "lark",
	"lemur", "lynx", "maple", "meadow", "meteor", "moose", "nebula", "otter", "owl", "panda",
	"pebble", "pine", "plover", "puffin", "quarry", "raven", "reef", "ridge", "river", "robin",
	"salmon", "spruce", "summit", "thistle", "tundra", "valley", "walrus", "willow", "wren", "yak",
}

// FreshTaskName is a name for a task with no repository, as adjective-noun,
// that taken says no task has.
func FreshTaskName(taken func(string) bool) string {
	for i := 0; ; i++ {
		name := taskAdjectives[rand.IntN(len(taskAdjectives))] + "-" + taskNouns[rand.IntN(len(taskNouns))]
		// A number makes it free once the words seldom do.
		if i >= 20 {
			name += "-" + strconv.Itoa(i)
		}
		if !taken(name) {
			return name
		}
	}
}
