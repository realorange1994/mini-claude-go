package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
)

// ═══════════════════════════════════════════════════════════
// Section: Hash Functions (from hash.go)
// ═══════════════════════════════════════════════════════════

// djb2Hash computes a DJB2 hash of a string, returning a signed 32-bit integer.
// Deterministic across runtimes. Ported from upstream TypeScript hash.ts.
func djb2Hash(str string) int32 {
	var hash int32 = 0
	for i := 0; i < len(str); i++ {
		hash = (hash<<5 - hash) + int32(str[i])
	}
	return hash
}

// hashContent hashes arbitrary content for change detection using SHA-256.
// Returns a hex string. Ported from upstream hashContent().
func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", h)
}

// hashPair hashes two strings disambiguating ("ts","code") vs ("tsc","ode").
// Uses a null separator to ensure different splits produce different hashes.
// Ported from upstream hashPair().
func hashPair(a string, b string) string {
	h := sha256.New()
	h.Write([]byte(a))
	h.Write([]byte{0}) // null separator
	h.Write([]byte(b))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// ═══════════════════════════════════════════════════════════
// Section: Fingerprint (from fingerprint.go)
// ═══════════════════════════════════════════════════════════

// FingerprintSalt is the hardcoded salt for fingerprint validation.
// Must match the backend's expected value.
const FingerprintSalt = "59cf53e54c78"

// computeFingerprint computes a 3-character fingerprint for Claude Code attribution.
// Algorithm: SHA256(SALT + msg[4] + msg[7] + msg[20] + version)[:3]
// IMPORTANT: Do not change this without careful coordination with API providers.
// Ported from upstream TypeScript fingerprint.ts.
func computeFingerprint(messageText string, version string) string {
	// Extract chars at indices [4, 7, 20], use "0" if index not found
	indices := []int{4, 7, 20}
	chars := ""
	for _, i := range indices {
		if i < len(messageText) {
			chars += string(messageText[i])
		} else {
			chars += "0"
		}
	}

	input := FingerprintSalt + chars + version

	// SHA256 hash, return first 3 hex chars
	hash := sha256.Sum256([]byte(input))
	return fmt.Sprintf("%x", hash)[:3]
}

// ═══════════════════════════════════════════════════════════
// Section: UUID & Agent ID (from uuid.go)
// ═══════════════════════════════════════════════════════════

// uuidRegex matches standard UUID format: 8-4-4-4-12 hex digits.
var uuidRegex = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// validateUUID checks if a string is a valid UUID and returns the string if valid,
// or an empty string if not. Ported from upstream TypeScript validateUuid().
func validateUUID(maybeUUID string) (string, bool) {
	if maybeUUID == "" {
		return "", false
	}
	if uuidRegex.MatchString(maybeUUID) {
		return maybeUUID, true
	}
	return "", false
}

// createAgentId generates a new agent ID with prefix for consistency with task IDs.
// Format: a{label-}{16 hex chars}
// Example: acompact-a3f2c1b4d5e6f7a8, aa3f2c1b4d5e6f7a8
// Ported from upstream TypeScript createAgentId().
func createAgentId(label string) string {
	suffix := randomHex(8) // 8 bytes = 16 hex chars
	if label != "" {
		return fmt.Sprintf("a%s-%s", label, suffix)
	}
	return fmt.Sprintf("a%s", suffix)
}

// randomHex generates a random hex string of the given byte length.
func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return fmt.Sprintf("%0*x", n*2, b)
}

// ═══════════════════════════════════════════════════════════
// Section: Tagged ID (from tagged_id.go)
// ═══════════════════════════════════════════════════════════

var taggedIDCounter uint64

// ToTaggedID creates a tagged ID string of the form "tag_counter_randomHex".
// Upstream: toTaggedId() in taggedId.ts
func ToTaggedID(tag string) string {
	counter := atomic.AddUint64(&taggedIDCounter, 1)
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	randomHex := hex.EncodeToString(randomBytes)
	return fmt.Sprintf("%s_%d_%s", tag, counter, randomHex)
}

// ParseTaggedID extracts the tag portion from a tagged ID.
// Returns the tag and true if valid, or empty string and false if invalid.
// Upstream: parsing logic from taggedId.ts
func ParseTaggedID(id string) (string, bool) {
	parts := strings.SplitN(id, "_", 3)
	if len(parts) < 2 {
		return "", false
	}
	return parts[0], true
}

// GetTaggedIDCounter extracts the counter portion from a tagged ID.
// Upstream: parsing logic from taggedId.ts
func GetTaggedIDCounter(id string) (uint64, bool) {
	parts := strings.SplitN(id, "_", 3)
	if len(parts) < 2 {
		return 0, false
	}
	counter, err := strconv.ParseUint(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return counter, true
}

// ValidateTaggedID checks if a string is a valid tagged ID with the given tag.
// Upstream: validation patterns from taggedId.ts
func ValidateTaggedID(id string, expectedTag string) bool {
	tag, ok := ParseTaggedID(id)
	if !ok {
		return false
	}
	return tag == expectedTag
}

// ═══════════════════════════════════════════════════════════
// Section: Word Lists & Slug Generation (from words.go)
// ═══════════════════════════════════════════════════════════

// Adjectives for slug generation - whimsical and delightful
// Ported from upstream words.ts
var adjectives = []string{
	"abundant", "ancient", "bright", "calm", "cheerful", "clever", "cozy",
	"curious", "dapper", "dazzling", "deep", "delightful", "eager", "elegant",
	"enchanted", "fancy", "fluffy", "gentle", "gleaming", "golden", "graceful",
	"happy", "hidden", "humble", "jolly", "joyful", "keen", "kind", "lively",
	"lovely", "lucky", "luminous", "magical", "majestic", "mellow", "merry",
	"mighty", "misty", "noble", "peaceful", "playful", "polished", "precious",
	"proud", "quiet", "quirky", "radiant", "rosy", "serene", "shiny", "silly",
	"sleepy", "smooth", "snazzy", "snug", "snuggly", "soft", "sparkling",
	"spicy", "splendid", "sprightly", "starry", "steady", "sunny", "swift",
	"tender", "tidy", "toasty", "tranquil", "twinkly", "valiant", "vast",
	"velvet", "vivid", "warm", "whimsical", "wild", "wise", "witty",
	"wondrous", "zany", "zesty", "zippy", "breezy", "bubbly", "buzzing",
	"cheeky", "cosmic", "crispy", "crystalline", "cuddly", "drifting",
	"dreamy", "effervescent", "ethereal", "fizzy", "flickering", "floating",
	"floofy", "fluttering", "foamy", "frolicking", "fuzzy", "giggly",
	"glimmering", "glistening", "glittery", "glowing", "goofy", "groovy",
	"harmonic", "hazy", "humming", "iridescent", "jaunty", "jazzy", "jiggly",
	"melodic", "moonlit", "mossy", "nifty", "peppy", "prancy", "purrfect",
	"purring", "quizzical", "rippling", "rustling", "shimmering", "shimmying",
	"snappy", "snoopy", "squishy", "swirling", "ticklish", "tingly",
	"twinkling", "velvety", "wiggly", "wobbly", "woolly", "zazzy",
	"abstract", "adaptive", "agile", "async", "atomic", "binary", "cached",
	"compiled", "composed", "compressed", "concurrent", "cryptic", "curried",
	"declarative", "delegated", "distributed", "dynamic", "elegant",
	"encapsulated", "enumerated", "eventual", "expressive", "federated",
	"functional", "generic", "greedy", "hashed", "idempotent", "immutable",
	"imperative", "indexed", "inherited", "iterative", "lazy", "lexical",
	"linear", "linked", "logical", "memoized", "modular", "mutable",
	"nested", "optimized", "parallel", "parsed", "partitioned", "piped",
	"polymorphic", "pure", "reactive", "recursive", "refactored",
	"reflective", "replicated", "resilient", "robust", "scalable",
	"sequential", "serialized", "sharded", "sorted", "staged", "stateful",
	"stateless", "streamed", "structured", "synchronous", "synthetic",
	"temporal", "transient", "typed", "unified", "validated", "vectorized",
	"virtual",
}

// Nouns for slug generation - whimsical creatures, nature, and fun objects
var nouns = []string{
	"aurora", "avalanche", "blossom", "breeze", "brook", "bubble", "canyon",
	"cascade", "cloud", "clover", "comet", "coral", "cosmos", "creek",
	"crescent", "crystal", "dawn", "dewdrop", "dusk", "eclipse", "ember",
	"feather", "fern", "firefly", "flame", "flurry", "fog", "forest",
	"frost", "galaxy", "garden", "glacier", "glade", "grove", "harbor",
	"horizon", "island", "lagoon", "lake", "leaf", "lightning", "meadow",
	"meteor", "mist", "moon", "moonbeam", "mountain", "nebula", "nova",
	"ocean", "orbit", "pebble", "petal", "pine", "planet", "pond", "puddle",
	"quasar", "rain", "rainbow", "reef", "ripple", "river", "shore", "sky",
	"snowflake", "spark", "spring", "star", "stardust", "starlight",
	"storm", "stream", "summit", "sun", "sunbeam", "sunrise", "sunset",
	"thunder", "tide", "twilight", "valley", "volcano", "waterfall",
	"wave", "willow", "wind", "alpaca", "axolotl", "badger", "bear",
	"beaver", "bee", "bird", "bumblebee", "bunny", "cat", "chipmunk",
	"crab", "crane", "deer", "dolphin", "dove", "dragon", "dragonfly",
	"duckling", "eagle", "elephant", "falcon", "finch", "flamingo", "fox",
	"frog", "giraffe", "goose", "hamster", "hare", "hedgehog", "hippo",
	"hummingbird", "jellyfish", "kitten", "koala", "ladybug", "lark",
	"lemur", "llama", "lobster", "lynx", "manatee", "meerkat", "moth",
	"narwhal", "newt", "octopus", "otter", "owl", "panda", "parrot",
	"peacock", "pelican", "penguin", "phoenix", "piglet", "platypus",
	"pony", "porcupine", "puffin", "puppy", "quail", "quokka", "rabbit",
	"raccoon", "raven", "robin", "salamander", "seahorse", "seal", "sloth",
	"snail", "sparrow", "sphinx", "squid", "squirrel", "starfish", "swan",
	"tiger", "toucan", "turtle", "unicorn", "walrus", "whale", "wolf",
	"wombat", "wren", "yeti", "zebra", "acorn", "anchor", "balloon",
	"beacon", "biscuit", "blanket", "bonbon", "book", "boot", "cake",
	"candle", "candy", "castle", "charm", "clock", "cocoa", "cookie",
	"crayon", "crown", "cupcake", "donut", "dream", "fairy", "fiddle",
	"flask", "flute", "fountain", "gadget", "gem", "gizmo", "globe",
	"goblet", "hammock", "harp", "haven", "hearth", "honey", "journal",
	"kazoo", "kettle", "key", "kite", "lantern", "lemon", "lighthouse",
	"locket", "lollipop", "mango", "map", "marble", "marshmallow",
	"melody", "mitten", "mochi", "muffin", "music", "nest", "noodle",
	"oasis", "origami", "pancake", "parasol", "peach", "pearl", "pie",
	"pillow", "pinwheel", "pixel", "pizza", "plum", "popcorn", "pretzel",
	"prism", "pudding", "pumpkin", "puzzle", "quiche", "quill", "quilt",
	"riddle", "rocket", "rose", "scone", "scroll", "shell", "sketch",
	"snowglobe", "sonnet", "sparkle", "spindle", "sprout", "sundae",
	"swing", "taco", "teacup", "teapot", "thimble", "toast", "token",
	"tome", "tower", "treasure", "treehouse", "trinket", "truffle",
	"tulip", "umbrella", "waffle", "wand", "whisper", "whistle",
	"widget", "wreath", "zephyr", "abelson", "adleman", "aho", "allen",
	"babbage", "bachman", "backus", "barto", "bengio", "bentley", "blum",
	"boole", "brooks", "catmull", "cerf", "cherny", "church", "clarke",
	"cocke", "codd", "conway", "cook", "corbato", "cray", "curry", "dahl",
	"diffie", "dijkstra", "dongarra", "eich", "emerson", "engelbart",
	"feigenbaum", "floyd", "gosling", "graham", "gray", "hamming",
	"hanrahan", "hartmanis", "hejlsberg", "hellman", "hennessy", "hickey",
	"hinton", "hoare", "hollerith", "hopcroft", "hopper", "iverson",
	"kahan", "kahn", "karp", "kay", "kernighan", "knuth", "kurzweil",
	"lamport", "lampson", "lecun", "lerdorf", "liskov", "lovelace",
	"matsumoto", "mccarthy", "metcalfe", "micali", "milner", "minsky",
	"moler", "moore", "naur", "neumann", "newell", "nygaard", "papert",
	"parnas", "pascal", "patterson", "pearl", "perlis", "pike", "pnueli",
	"rabin", "reddy", "ritchie", "rivest", "rossum", "russell", "scott",
	"sedgewick", "shamir", "shannon", "sifakis", "simon", "stallman",
	"stearns", "steele", "stonebraker", "stroustrup", "sutherland",
	"sutton", "tarjan", "thacker", "thompson", "torvalds", "turing",
	"ullman", "valiant", "wadler", "wall", "wigderson", "wilkes",
	"wilkinson", "wirth", "wozniak", "yao",
}

// Verbs for the middle word - whimsical action words
var verbs = []string{
	"baking", "beaming", "booping", "bouncing", "brewing", "bubbling",
	"chasing", "churning", "coalescing", "conjuring", "cooking", "crafting",
	"crunching", "cuddling", "dancing", "dazzling", "discovering",
	"doodling", "dreaming", "drifting", "enchanting", "exploring",
	"finding", "floating", "fluttering", "foraging", "forging",
	"frolicking", "gathering", "giggling", "gliding", "greeting",
	"growing", "hatching", "herding", "honking", "hopping", "hugging",
	"humming", "imagining", "inventing", "jingling", "juggling",
	"jumping", "kindling", "knitting", "launching", "leaping", "mapping",
	"marinating", "meandering", "mixing", "moseying", "munching",
	"napping", "nibbling", "noodling", "orbiting", "painting",
	"percolating", "petting", "plotting", "pondering", "popping",
	"prancing", "purring", "puzzling", "questing", "riding", "roaming",
	"rolling", "sauteeing", "scribbling", "seeking", "shimmying",
	"singing", "skipping", "sleeping", "snacking", "sniffing",
	"snuggling", "soaring", "sparking", "spinning", "splashing",
	"sprouting", "squishing", "stargazing", "stirring", "strolling",
	"swimming", "swinging", "tickling", "tinkering", "toasting",
	"tumbling", "twirling", "waddling", "wandering", "watching",
	"weaving", "whistling", "wibbling", "wiggling", "wishing",
	"wobbling", "wondering", "yawning", "zooming",
}

// randomInt generates a cryptographically random integer in the range [0, max).
func randomInt(max int) int {
	buf := make([]byte, 4)
	if _, err := rand.Read(buf); err != nil {
		panic(fmt.Sprintf("crypto/rand.Read failed (broken CSPRNG): %v", err))
	}
	value := uint32(buf[0])<<24 | uint32(buf[1])<<16 | uint32(buf[2])<<8 | uint32(buf[3])
	return int(value % uint32(max))
}

// pickRandom picks a random element from a slice.
func pickRandom(arr []string) string {
	return arr[randomInt(len(arr))]
}

// generateWordSlug generates a random word slug in the format "adjective-verb-noun".
// Ported from upstream words.ts generateWordSlug.
func generateWordSlug() string {
	return fmt.Sprintf("%s-%s-%s", pickRandom(adjectives), pickRandom(verbs), pickRandom(nouns))
}

// generateShortWordSlug generates a shorter random word slug in the format "adjective-noun".
// Ported from upstream words.ts generateShortWordSlug.
func generateShortWordSlug() string {
	return fmt.Sprintf("%s-%s", pickRandom(adjectives), pickRandom(nouns))
}
