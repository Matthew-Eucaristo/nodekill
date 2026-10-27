package ui

import (
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Matthew-Eucaristo/nodekill/internal/clean"
	"github.com/Matthew-Eucaristo/nodekill/internal/scan"
	"github.com/Matthew-Eucaristo/nodekill/internal/testfix"
)

func model(t *testing.T) *Model {
	t.Helper()
	root := testfix.Real(t, t.TempDir())
	testfix.Write(t, filepath.Join(root, "shop"), map[string]string{
		"package.json": `{"dependencies":{"next":"16"}}`, "package-lock.json": "{}", ".gitignore": "node_modules/\n.next/\n",
		"node_modules/next/index.js": "x", ".next/cache/a": "x",
	})
	testfix.Repo(t, filepath.Join(root, "shop"), testfix.Old)
	testfix.Write(t, filepath.Join(root, "fresh"), map[string]string{"package.json": `{}`, ".gitignore": "node_modules/\n", "node_modules/a": "x"})
	testfix.Repo(t, filepath.Join(root, "fresh"), testfix.Old)
	opts := scan.Options{Roots: []string{root}, Home: root, Env: scan.DefaultEnv()}
	res, err := scan.Scan(opts)
	if err != nil {
		t.Fatal(err)
	}
	res.MeasureAll()
	m := New(opts, clean.Options{Mode: clean.DryRun, Scan: opts}, 0)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 32})
	m.res = res
	m.cursorKey = firstItem(m.rows())
	return m
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		msg := tea.KeyPressMsg{Text: k}
		switch k {
		case "space":
			msg = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
		case "enter":
			msg = tea.KeyPressMsg{Code: tea.KeyEnter}
		case "esc":
			msg = tea.KeyPressMsg{Code: tea.KeyEscape}
		case "tab":
			msg = tea.KeyPressMsg{Code: tea.KeyTab}
		default:
			msg.Code = []rune(k)[0]
		}
		m.Update(msg)
	}
}

func screen(m *Model) string { return ansi.Strip(m.render()) }

func TestRenderAndSelect(t *testing.T) {
	m := model(t)
	s := screen(m)
	for _, want := range []string{"nodekill", "dry run", "node_modules", ".next", "safe", "review", "no lockfile", "npm · Next.js"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen lacks %q:\n%s", want, s)
		}
	}
	press(m, "s")
	if len(m.selected) != 2 {
		t.Fatalf("s selected %d, want the 2 safe items", len(m.selected))
	}
	press(m, "enter")
	if s := screen(m); !strings.Contains(s, "Dry run: nothing will change") || !strings.Contains(s, "Move to the Trash") {
		t.Fatalf("confirm screen:\n%s", s)
	}
	press(m, "esc")
	if m.confirm != nil || len(m.selected) != 2 {
		t.Fatal("esc should cancel the confirmation and keep the selection")
	}
}

func TestGroupingFilterLegend(t *testing.T) {
	m := model(t)
	press(m, "tab")
	if !strings.Contains(screen(m), "group: framework") || !strings.Contains(screen(m), "Next.js") {
		t.Error("framework grouping")
	}
	press(m, "tab")
	if !strings.Contains(screen(m), "idle 90+ days") {
		t.Errorf("age grouping:\n%s", screen(m))
	}
	press(m, "/", "z", "z", "z", "enter")
	if !strings.Contains(screen(m), "Nothing matches") {
		t.Error("filter")
	}
	press(m, "esc", "?")
	if !strings.Contains(screen(m), "Safety") {
		t.Error("legend")
	}
}
