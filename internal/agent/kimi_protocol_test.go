package agent

import (
	"path/filepath"
	"testing"

	"github.com/yetone/magpie/internal/edit"
	"github.com/yetone/magpie/internal/provider"
)

// #2846: choosing OpenAI in Kimi Code must survive a model catalog sync
// and a model picked in magpie, both of which used to restore type kimi.
func TestKimiCodeKeepsOpenAIProtocol(t *testing.T) {
	home := syncHome(t)
	t.Setenv("KIMI_SHARE_DIR", "")
	t.Setenv("KIMI_CODE_HOME", filepath.Join(home, ".kimi-code"))
	a, err := Find("kimi")
	if err != nil {
		t.Fatal(err)
	}
	f := a.Field("model")
	if err := f.Set("magpie/relay/glm-4.6"); err != nil {
		t.Fatal(err)
	}
	if err := edit.SetTOMLKey(a.Path, "providers.magpie", "type", "openai"); err != nil {
		t.Fatal(err)
	}
	if err := provider.Save(provider.Provider{ID: "later", Name: "Later", Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"next"}}); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		name string
		run  func() error
	}{
		{"catalog sync", func() error { SyncCatalog(); return nil }},
		{"model pick", func() error { return f.Set("magpie/later/next") }},
		{"second sync", func() error { SyncCatalog(); return nil }},
	} {
		if err := step.run(); err != nil {
			t.Fatal(err)
		}
		p, err := edit.GetTOMLTable(a.Path, "providers.magpie")
		if err != nil {
			t.Fatal(err)
		}
		if p["type"] != "openai" {
			t.Errorf("%s: protocol = %q, want openai", step.name, p["type"])
		}
		if p["base_url"] != gatewayV1() || p["api_key"] == "" {
			t.Errorf("%s: gateway wiring = %v", step.name, p)
		}
		if m, err := edit.GetTOMLTable(a.Path, `models."magpie/later/next"`); err != nil || m == nil || m["model"] != "later/next" {
			t.Errorf("%s: new catalog model = %v, %v", step.name, m, err)
		}
	}
}

// Only the new Kimi Code's OpenAI Chat choice changes: the default,
// invalid choices and the legacy kimi-cli keep the existing kimi adapter.
func TestKimiProtocolDefaults(t *testing.T) {
	for _, c := range []struct {
		name   string
		legacy bool
		line   string
	}{
		{name: "new provider"},
		{name: "missing type", line: "[providers.magpie]\n"},
		{name: "empty type", line: "[providers.magpie]\ntype = \"\"\n"},
		{name: "kimi", line: "[providers.magpie]\ntype = \"kimi\"\n"},
		{name: "unsupported type", line: "[providers.magpie]\ntype = \"unsupported\"\n"},
		{name: "legacy OpenAI", legacy: true, line: "[providers.magpie]\ntype = \"openai\"\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := syncHome(t)
			t.Setenv("KIMI_SHARE_DIR", "")
			t.Setenv("KIMI_CODE_HOME", "")
			dir := ".kimi-code"
			if c.legacy {
				dir = ".kimi"
			}
			path := filepath.Join(home, dir, "config.toml")
			writeFile(t, path, c.line)
			a, err := Find("kimi")
			if err != nil {
				t.Fatal(err)
			}
			if a.Path != path {
				t.Fatalf("config = %s, want %s", a.Path, path)
			}
			if err := a.Field("model").Set("magpie/relay/glm-4.6"); err != nil {
				t.Fatal(err)
			}
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
			p, err := edit.GetTOMLTable(path, "providers.magpie")
			if err != nil {
				t.Fatal(err)
			}
			if p["type"] != "kimi" {
				t.Errorf("protocol = %q, want kimi", p["type"])
			}
			before := readFile(path)
			if err := a.Sync(); err != nil {
				t.Fatal(err)
			}
			if got := readFile(path); got != before {
				t.Errorf("unchanged catalog changed config:\nbefore:\n%s\nafter:\n%s", before, got)
			}
		})
	}
}
