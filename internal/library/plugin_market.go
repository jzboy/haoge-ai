package library

import (
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
// list is fetched from GitHub once a day and kept, so the row works on a
// computer whose Claude Code never added it. Each plugin is installed
// through the same source the tab's Find row takes (installPlugins).

// PluginMarketItem is one plugin a marketplace offers.
type PluginMarketItem struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Marketplace string `json:"marketplace"`
	// Source is what to install it from: the marketplace's repository
	// (owner/repo), or its folder here.
	Source string `json:"source"`
	// Have is whether the library holds it already.
	Have bool `json:"have,omitempty"`
}

// PluginMarket is what the tab's Discover row lists.
type PluginMarket struct {
	Marketplaces []string           `json:"marketplaces"`
	Items        []PluginMarketItem `json:"items"`
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
	b, err := readClaudeJSON(claudeMarketplaces())
	if err != nil || len(b) == 0 {
		return out
	}
	var known map[string]marketKnown
	if json.Unmarshal(b, &known) != nil {
		return out
	}
	l, _ := load()
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
	// Claude Code's own marketplace, added by Claude Code itself or not
	if !slices.Contains(out.Marketplaces, claudeOfficial) {
		if items := officialMarketItems(); len(items) > 0 {
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
// .claude-plugin/marketplace.json in dir: its name and description for
// each, which the row shows (a plugin's own parts are read when it is
// installed).
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

// parseMarketBytes is a marketplace.json's plugins: their names and
// descriptions, which the row shows (a plugin's own parts are read when
// it is installed).
func parseMarketBytes(b []byte) []PluginMarketItem {
	var doc struct {
		Plugins []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
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
		items = append(items, PluginMarketItem{Name: p.Name, Description: p.Description})
	}
	return items
}

// Claude Code's own marketplace: its repository, the file that lists what
// it offers, and where the answer is kept here.
const (
	officialMarketRepo = "anthropics/claude-plugins-official"
	officialMarketURL  = "https://raw.githubusercontent.com/" + officialMarketRepo + "/main/.claude-plugin/marketplace.json"
)

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

// officialMarketItems is what Claude Code's own marketplace offers: the
// list kept here while it is under a day old, else GitHub's, kept for the
// next time. A fetch that fails keeps the list there was, however old, and
// no list at all leaves the marketplace out.
func officialMarketItems() []PluginMarketItem {
	cache := officialMarketCache()
	if st, err := os.Stat(cache); err == nil && time.Since(st.ModTime()) < officialMarketAge {
		if items := parseMarketFile(cache); items != nil {
			return items
		}
	}
	if b, err := marketFetch(officialMarketURL); err == nil {
		if items := parseMarketBytes(b); len(items) > 0 {
			if err := os.WriteFile(cache, b, 0o644); err != nil {
				log.Printf("library: keeping the official marketplace's list: %v", err)
			}
			return items
		}
	}
	return parseMarketFile(cache)
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

