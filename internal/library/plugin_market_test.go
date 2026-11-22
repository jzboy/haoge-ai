package library

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)


// noOfficial keeps Claude Code's own marketplace out of a test's answer:
// no test reaches GitHub, and the copy in the binary would stand in.
func noOfficial(t *testing.T) {
	t.Helper()
	was := officialSeed
	officialSeed = nil
	t.Cleanup(func() { officialSeed = was; os.Remove(officialMarketCache()) })
}

// The Claude plugins tab's Discover row: the marketplaces Claude Code
// knows, read here, with each plugin and what to install it from.
func TestPluginMarketReadsKnownMarketplaces(t *testing.T) {
	noOfficial(t)
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	mp := filepath.Join(dir, "plugins", "marketplaces", "claude-code-plugins")
	if err := os.MkdirAll(filepath.Join(mp, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	mk := `{"name":"claude-code-plugins","plugins":[
		{"name":"feature-dev","description":"Feature work"},
		{"name":"code-review","description":"Reviews a PR"}]}`
	if err := os.WriteFile(filepath.Join(mp, ".claude-plugin", "marketplace.json"), []byte(mk), 0o644); err != nil {
		t.Fatal(err)
	}
	loc, _ := json.Marshal(mp)
	known := `{"claude-code-plugins":{"source":{"source":"github","repo":"anthropics/claude-code"},"installLocation":` + string(loc) + `}}`
	if err := os.WriteFile(filepath.Join(dir, "plugins", "known_marketplaces.json"), []byte(known), 0o644); err != nil {
		t.Fatal(err)
	}

	m := PluginMarketList()
	if len(m.Marketplaces) != 1 || m.Marketplaces[0] != "claude-code-plugins" {
		t.Fatalf("marketplaces %v", m.Marketplaces)
	}
	if len(m.Items) != 2 {
		t.Fatalf("%d items, want 2: %+v", len(m.Items), m.Items)
	}
	// sorted by name: code-review, then feature-dev
	if m.Items[0].Name != "code-review" || m.Items[0].Description != "Reviews a PR" {
		t.Fatalf("first item %+v", m.Items[0])
	}
	if m.Items[0].Source != "anthropics/claude-code" {
		t.Fatalf("source %q, want the marketplace's repository", m.Items[0].Source)
	}
	if m.Items[0].Marketplace != "claude-code-plugins" {
		t.Fatalf("marketplace %q", m.Items[0].Marketplace)
	}
}

// A marketplace Claude Code cloned from a folder (no repository) is
// installed from that folder.
func TestPluginMarketFolderSource(t *testing.T) {
	noOfficial(t)
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	mp := filepath.Join(dir, "plugins", "marketplaces", "local")
	if err := os.MkdirAll(filepath.Join(mp, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mp, ".claude-plugin", "marketplace.json"), []byte(`{"name":"local","plugins":[{"name":"acme"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loc, _ := json.Marshal(mp)
	known := `{"local":{"source":{"source":"directory"},"installLocation":` + string(loc) + `}}`
	if err := os.WriteFile(filepath.Join(dir, "plugins", "known_marketplaces.json"), []byte(known), 0o644); err != nil {
		t.Fatal(err)
	}
	m := PluginMarketList()
	if len(m.Items) != 1 || m.Items[0].Source != mp {
		t.Fatalf("items %+v, want one from %s", m.Items, mp)
	}
}

// No marketplaces: an empty list, not an error.
func TestPluginMarketNone(t *testing.T) {
	noOfficial(t)
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	m := PluginMarketList()
	if m == nil || len(m.Items) != 0 || len(m.Marketplaces) != 0 {
		t.Fatalf("%+v", m)
	}
}

// Claude Code's own marketplace is listed whether or not Claude Code added
// it, from the list fetched once and kept here; a plugin another
// marketplace already names is listed once.
func TestPluginMarketIncludesOfficial(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	mp := filepath.Join(dir, "plugins", "marketplaces", "local")
	if err := os.MkdirAll(filepath.Join(mp, ".claude-plugin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mp, ".claude-plugin", "marketplace.json"), []byte(`{"name":"local","plugins":[{"name":"shared"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	loc, _ := json.Marshal(mp)
	known := `{"local":{"source":{"source":"directory"},"installLocation":` + string(loc) + `}}`
	if err := os.WriteFile(filepath.Join(dir, "plugins", "known_marketplaces.json"), []byte(known), 0o644); err != nil {
		t.Fatal(err)
	}

	was := marketFetch
	t.Cleanup(func() { marketFetch = was; os.Remove(officialMarketCache()) })
	asked := 0
	marketFetch = func(string) ([]byte, error) {
		asked++
		return []byte(`{"name":"claude-plugins-official","plugins":[
			{"name":"shared","description":"already listed"},
			{"name":"official-only","description":"only here"}]}`), nil
	}

	m := PluginMarketList()
	if asked != 1 {
		t.Fatalf("fetched %d times, want 1", asked)
	}
	if !slices.Contains(m.Marketplaces, claudeOfficial) {
		t.Fatalf("marketplaces %v, want %s", m.Marketplaces, claudeOfficial)
	}
	var shared, official int
	for _, it := range m.Items {
		switch it.Name {
		case "shared":
			shared++
			if it.Marketplace != "local" {
				t.Fatalf("shared listed under %q, want the marketplace that names it", it.Marketplace)
			}
		case "official-only":
			official++
			if it.Marketplace != claudeOfficial || it.Source != officialMarketRepo {
				t.Fatalf("official-only %+v", it)
			}
		}
	}
	if shared != 1 || official != 1 {
		t.Fatalf("items %+v", m.Items)
	}
}

// The CDN stands in when GitHub doesn't answer, and the copy in the binary
// when neither does: the tab lists the official marketplace anyway.
func TestPluginMarketOfficialFallbacks(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)
	was := marketFetch
	t.Cleanup(func() { marketFetch = was; os.Remove(officialMarketCache()) })

	var tried []string
	marketFetch = func(u string) ([]byte, error) {
		tried = append(tried, u)
		if len(tried) == 1 {
			return nil, errors.New("no route")
		}
		return []byte(`{"name":"claude-plugins-official","plugins":[{"name":"from-cdn"}]}`), nil
	}
	m := PluginMarketList()
	if m.Official != "network" || len(tried) != 2 {
		t.Fatalf("official %q, tried %v", m.Official, tried)
	}
	found := false
	for _, it := range m.Items {
		if it.Name == "from-cdn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("items %+v", m.Items)
	}

	os.Remove(officialMarketCache())
	tried = nil
	marketFetch = func(u string) ([]byte, error) { tried = append(tried, u); return nil, errors.New("offline") }
	m = PluginMarketList()
	if m.Official != "seed" || len(m.Items) < 100 {
		t.Fatalf("offline: official %q, %d items", m.Official, len(m.Items))
	}
}
