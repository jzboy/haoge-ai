package library

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/proc"
)

// The Library's Discover row on its Claude plugins tab: the plugins the
// marketplaces Claude Code knows offer, read from each one's marketplace
// file on this computer (plugins/known_marketplaces.json and the folder
// Claude Code cloned it into), and Claude Code's own marketplace
// (claude-plugins-official) whether or not Claude Code added it — its
// list is fetched once a day from GitHub (or a CDN of it where GitHub
// can't be reached), kept here, and a copy of it ships in the binary so
// the row lists the marketplace on a computer that can reach neither.
// Each plugin is installed through the same source the tab's Find row
// takes (installPlugins).

// PluginMarketItem is one plugin a marketplace offers.
type PluginMarketItem struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Author      string `json:"author,omitempty"`
	Category    string `json:"category,omitempty"`
	Marketplace string `json:"marketplace"`
	// Source is what to install it from: the marketplace's repository
	// (owner/repo), or its folder here.
	Source string `json:"source"`
	// Homepage is the plugin's own page, and Repo the repository its
	// source names, both "" when it names neither.
	Homepage string `json:"homepage,omitempty"`
	Repo     string `json:"repo,omitempty"`
	// Have is whether the library holds it already.
	Have bool `json:"have,omitempty"`
}

// PluginMarket is what the tab's Discover row lists.
type PluginMarket struct {
	Marketplaces []string           `json:"marketplaces"`
	Items        []PluginMarketItem `json:"items"`
	// Official is Claude Code's own marketplace's list: "network" when
	// GitHub or its CDN answered, "cache" when the copy kept here stood
	// in, "seed" for the one in the binary, "" when there is none.
	Official string `json:"official,omitempty"`
}

// marketKnown is one entry of Claude Code's known_marketplaces.json.
type marketKnown struct {
	Source struct {
		Source string `json:"source"`
		Repo   string `json:"repo"`
		URL    string `json:"url"`
	} `json:"source"`
	InstallLocation string `json:"installLocation"`
}

// PluginMarketList reads the marketplaces Claude Code knows and what each one
// offers. A marketplace that can't be read is left out; one with no
// plugins still names itself, so the row can say the list is empty.
func PluginMarketList() *PluginMarket {
	out := &PluginMarket{Marketplaces: []string{}, Items: []PluginMarketItem{}}
	l, _ := load()
	if b, err := readClaudeJSON(claudeMarketplaces()); err == nil && len(b) > 0 {
		var known map[string]marketKnown
		if json.Unmarshal(b, &known) == nil {
			for name, k := range known {
				src := knownMarketSource(k)
				if src == "" {
					continue
				}
				out.Marketplaces = append(out.Marketplaces, name)
				for _, it := range marketplacePlugins(k.InstallLocation) {
					it.Marketplace = name
					it.Source = src
					if l != nil {
						it.Have = l.plugin(it.Name) != nil
					}
					out.Items = append(out.Items, it)
				}
			}
		}
	}
	// Claude Code's own marketplace, added by Claude Code itself or not
	if !slices.Contains(out.Marketplaces, claudeOfficial) {
		items, from := officialMarketItems()
		out.Official = from
		if len(items) > 0 {
			out.Marketplaces = append(out.Marketplaces, claudeOfficial)
			seen := make(map[string]bool, len(out.Items))
			for _, it := range out.Items {
				seen[it.Name] = true
			}
			for _, it := range items {
				if seen[it.Name] {
					continue
				}
				it.Marketplace = claudeOfficial
				it.Source = officialMarketRepo
				if l != nil {
					it.Have = l.plugin(it.Name) != nil
				}
				out.Items = append(out.Items, it)
			}
		}
	}
	sort.Strings(out.Marketplaces)
	sort.SliceStable(out.Items, func(i, j int) bool {
		if out.Items[i].Marketplace != out.Items[j].Marketplace {
			return out.Items[i].Marketplace < out.Items[j].Marketplace
		}
		return strings.ToLower(out.Items[i].Name) < strings.ToLower(out.Items[j].Name)
	})
	return out
}

// knownMarketSource is what to install a marketplace's plugins from: its
// GitHub repository, else the folder Claude Code cloned it into.
func knownMarketSource(k marketKnown) string {
	if strings.EqualFold(k.Source.Source, "github") && k.Source.Repo != "" {
		return k.Source.Repo
	}
	if k.InstallLocation != "" {
		return k.InstallLocation
	}
	return k.Source.URL
}

// marketplacePlugins reads a marketplace's plugins from the
// .claude-plugin/marketplace.json in dir.
func marketplacePlugins(dir string) []PluginMarketItem {
	if dir == "" {
		return nil
	}
	b, err := os.ReadFile(filepath.Join(dir, ".claude-plugin", "marketplace.json"))
	if err != nil {
		return nil
	}
	return parseMarketBytes(b)
}

// parseMarketBytes is a marketplace.json's plugins: what a card shows (a
// plugin's own parts are read when it is installed).
func parseMarketBytes(b []byte) []PluginMarketItem {
	var doc struct {
		Plugins []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Category    string          `json:"category"`
			Author      json.RawMessage `json:"author"`
			Homepage    string          `json:"homepage"`
			// a marketplace entry's source is an object (where it is
			// fetched from) or a folder in the marketplace's own
			// repository ("./plugins/x"), which has no repository of its
			// own to link
			Source json.RawMessage `json:"source"`
		} `json:"plugins"`
	}
	if json.Unmarshal(b, &doc) != nil {
		return nil
	}
	items := make([]PluginMarketItem, 0, len(doc.Plugins))
	for _, p := range doc.Plugins {
		if p.Name == "" {
			continue
		}
		items = append(items, PluginMarketItem{
			Name: p.Name, Description: p.Description, Category: p.Category,
			Author: stringOrName(p.Author), Homepage: p.Homepage,
			Repo: sourceRepo(p.Source),
		})
	}
	return items
}


// stringOrName is a marketplace field that is a string in some entries and
// {"name": …} in others.
func stringOrName(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var o struct {
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.Name
	}
	return ""
}

// sourceRepo is the repository a plugin's source names, as
// https://github.com/o/r; "" for a folder inside the marketplace's own
// repository.
func sourceRepo(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return ""
	}
	var o struct {
		URL  string `json:"url"`
		Repo string `json:"repo"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return githubRepo(o.Repo, o.URL)
	}
	return ""
}

// githubRepo is the repository a source names, as https://github.com/o/r,
// "" when it names none: a repository (owner/repo or its URL) or a git
// URL there (a git-subdir's url).
func githubRepo(repo, url string) string {
	ref := repo
	if ref == "" {
		ref = url
	}
	ref = strings.TrimSuffix(strings.TrimSuffix(ref, "/"), ".git")
	if !strings.Contains(ref, "://") && len(strings.Split(ref, "/")) == 2 {
		return "https://github.com/" + ref
	}
	if i := strings.Index(ref, "github.com/"); i >= 0 {
		rest := strings.TrimPrefix(ref[i+len("github.com/"):], "/")
		parts := strings.Split(rest, "/")
		if len(parts) >= 2 {
			return "https://github.com/" + parts[0] + "/" + parts[1]
		}
	}
	return ""
}

// Claude Code's own marketplace: its repository, the files that list what
// it offers (GitHub, then a CDN of it where GitHub is out of reach), and
// where the answer is kept here.
const officialMarketRepo = "anthropics/claude-plugins-official"

var officialMarketURLs = []string{
	"https://raw.githubusercontent.com/" + officialMarketRepo + "/main/.claude-plugin/marketplace.json",
	"https://cdn.jsdelivr.net/gh/" + officialMarketRepo + "@main/.claude-plugin/marketplace.json",
}

//go:embed official_market_seed.json
var officialMarketSeed []byte

// officialSeed is the copy in the binary a computer that can reach neither
// GitHub nor its CDN is given; a test empties it so the official
// marketplace stays out of what it checks.
var officialSeed = officialMarketSeed

// officialMarketAge is how long the kept list stands before GitHub is
// asked again.
const officialMarketAge = 24 * time.Hour

// marketFetch reads a URL. A test stands in for it, so no test reaches
// the network.
var marketFetch = func(url string) ([]byte, error) {
	// a test's sandbox never reaches the network (LESSONS: a test never
	// does): a test that answers stands in for this
	if proc.Isolated() {
		return nil, fmt.Errorf("%s: no network in a test", url)
	}
	c := &http.Client{Timeout: 20 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

func officialMarketCache() string { return filepath.Join(Dir(), "plugin-market-official.json") }

// officialMarketItems is what Claude Code's own marketplace offers, and
// where the list came from: the copy kept here while it is under a day
// old, else GitHub's or its CDN's, kept for the next time; a fetch that
// fails keeps the list there was, however old, and with none the copy in
// the binary stands in.
func officialMarketItems() ([]PluginMarketItem, string) {
	cache := officialMarketCache()
	if st, err := os.Stat(cache); err == nil && time.Since(st.ModTime()) < officialMarketAge {
		if items := parseMarketFile(cache); items != nil {
			return items, "cache"
		}
	}
	for _, u := range officialMarketURLs {
		b, err := marketFetch(u)
		if err != nil {
			continue
		}
		if items := parseMarketBytes(b); len(items) > 0 {
			if err := os.MkdirAll(filepath.Dir(cache), 0o755); err == nil {
				if err := os.WriteFile(cache, b, 0o644); err != nil {
					log.Printf("library: keeping the official marketplace's list: %v", err)
				}
			}
			return items, "network"
		}
	}
	if items := parseMarketFile(cache); items != nil {
		return items, "cache"
	}
	return parseMarketBytes(officialSeed), "seed"
}

// parseMarketFile is parseMarketBytes for a file, nil when there is none
// or it can't be read.
func parseMarketFile(path string) []PluginMarketItem {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return parseMarketBytes(b)
}
