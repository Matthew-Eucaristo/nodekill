// Package report prints scans and receipts as plain text.
package report

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Matthew-Eucaristo/nodekill/internal/clean"
	"github.com/Matthew-Eucaristo/nodekill/internal/format"
	"github.com/Matthew-Eucaristo/nodekill/internal/scan"
)

// Groups orders items for display: projects (largest first), then caches,
// global packages and Node versions.
func Groups(items []*scan.Item) []Group {
	byKey := map[string]*Group{}
	var order []*Group
	for _, it := range items {
		key, title := string(it.Category), CategoryTitle(it.Category)
		if it.Project != nil {
			key, title = "p:"+it.Project.Group, it.Project.Group
		}
		g := byKey[key]
		if g == nil {
			g = &Group{Key: key, Title: title, Category: it.Category}
			byKey[key] = g
			order = append(order, g)
		}
		g.Items = append(g.Items, it)
		g.Bytes += max(it.Size.Allocated, 0)
	}
	slices.SortStableFunc(order, func(a, b *Group) int {
		if a.Category != b.Category {
			return cmp.Compare(catRank(a.Category), catRank(b.Category))
		}
		return cmp.Compare(b.Bytes, a.Bytes)
	})
	var out []Group
	for _, g := range order {
		slices.SortStableFunc(g.Items, func(a, b *scan.Item) int { return cmp.Compare(b.Size.Allocated, a.Size.Allocated) })
		out = append(out, *g)
	}
	return out
}

// Group is a project (or workspace) or a category of machine-wide items.
type Group struct {
	Key, Title string
	Category   scan.Category
	Items      []*scan.Item
	Bytes      int64
}

func catRank(c scan.Category) int {
	return slices.Index([]scan.Category{scan.CatProject, scan.CatCache, scan.CatGlobal, scan.CatRuntime}, c)
}

// CategoryTitle names a category for headings.
func CategoryTitle(c scan.Category) string {
	return map[scan.Category]string{scan.CatProject: "Projects", scan.CatCache: "Caches and stores",
		scan.CatGlobal: "Global packages and npx", scan.CatRuntime: "Node versions"}[c]
}

// Text writes a scan as a table.
func Text(w io.Writer, res *scan.Result) {
	home, _ := os.UserHomeDir()
	now := time.Now()
	var total, free, safe scan.Item
	for _, it := range res.Items {
		total.Size = total.Size.Add(it.Size)
		if it.Safety.Level.Allowed() {
			free.Size = free.Size.Add(it.Size)
		}
		if it.Safety.Level == scan.Safe {
			safe.Size = safe.Size.Add(it.Size)
		}
	}
	var roots []string
	for _, r := range res.Roots {
		roots = append(roots, format.Tilde(r, home))
	}
	fmt.Fprintf(w, "nodekill: %d item(s) in %d project(s), %s on disk, %s freeable; %s safe to clean  (roots: %s)\n",
		len(res.Items), len(res.Projects), format.Bytes(total.Size.Allocated), format.Bytes(free.Size.Exclusive()),
		format.Bytes(safe.Size.Exclusive()), strings.Join(roots, ", "))
	for _, warn := range res.Warnings {
		fmt.Fprintln(w, "warning:", warn)
	}
	if len(res.Items) == 0 {
		fmt.Fprintln(w, "\nNothing to clean.")
		return
	}
	for _, g := range Groups(res.Items) {
		title := g.Title
		if g.Category == scan.CatProject {
			title = format.Tilde(g.Title, home)
			if p := g.Items[0].Project; p != nil {
				title += "  " + ProjectBadges(p, now)
			}
		}
		fmt.Fprintf(w, "\n%s  %s\n", title, format.Bytes(g.Bytes))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, it := range g.Items {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\t%s\n", format.Bytes(size(it)), Frees(it), it.Safety.Level, Label(it, home), Codes(it), it.ID)
		}
		tw.Flush()
	}
	fmt.Fprintln(w, "\nsafe: rebuildable, may be preselected · review: allowed, never preselected · protected/live: nodekill will not touch it")
}

func size(it *scan.Item) int64 {
	if !it.Measured {
		return -1
	}
	return it.Size.Allocated
}

// Frees shows the exclusive bytes when they differ from the allocated ones.
func Frees(it *scan.Item) string {
	if !it.Measured {
		return ""
	}
	if it.Size.Shared > 0 && it.Size.Exclusive() < it.Size.Allocated*9/10 {
		return "frees " + format.Bytes(it.Size.Exclusive())
	}
	return ""
}

// Label is an item's display name: the artifact path below its workspace,
// or the cache, package or version name.
func Label(it *scan.Item, home string) string {
	if p := it.Project; p != nil {
		rel := format.Rel(p.Group, it.Path)
		if rel == "" {
			rel = it.Name
		}
		return rel
	}
	return it.Name
}

// Codes lists non-safe reason codes compactly.
func Codes(it *scan.Item) string {
	var c []string
	for _, r := range it.Safety.Reasons {
		c = append(c, r.Code)
	}
	if len(it.Command) > 0 {
		c = append(c, "via `"+CommandText(it)+"`")
	}
	return strings.Join(c, " ")
}

// CommandText shows a command with its executable reduced to its name.
func CommandText(it *scan.Item) string {
	if it.Shown != "" || len(it.Command) == 0 {
		return it.Shown
	}
	var parts []string
	for _, a := range append([]string{baseName(it.Command[0])}, it.Command[1:]...) {
		if strings.HasPrefix(a, "--logs-max") || a == "--no-update-notifier" || a == "--color=false" {
			continue // npm's quiet flags
		}
		parts = append(parts, a)
	}
	if parts[0] == "node" && len(parts) > 1 {
		parts = append([]string{baseName(strings.TrimSuffix(parts[1], ".js"))}, parts[2:]...)
	}
	return strings.Join(parts, " ")
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		p = p[i+1:]
	}
	return strings.TrimSuffix(strings.TrimSuffix(p, ".exe"), ".cmd")
}

// ProjectBadges summarises a project: package manager, tools, activity.
func ProjectBadges(p *scan.Project, now time.Time) string {
	var parts []string
	if p.PM != "" {
		parts = append(parts, string(p.PM))
	}
	if len(p.Tools) > 0 {
		parts = append(parts, strings.Join(p.Tools, ", "))
	}
	parts = append(parts, "active "+format.Ago(p.LastActivity, now))
	if p.Worktree {
		parts = append(parts, "linked worktree")
	}
	return strings.Join(parts, " · ")
}

// Receipt summarises what a run did, keeping the three kinds of freeing
// apart: moved to the trash (freed when emptied), removed by an official
// command, deleted permanently.
func Receipt(w io.Writer, outs []*clean.Outcome, mode clean.Mode) {
	if len(outs) == 0 {
		return
	}
	home, _ := os.UserHomeDir()
	var trashed, byTool, deleted, freed int64
	var nTrash, nTool, nDel int
	var bad []*clean.Outcome
	for _, o := range outs {
		switch o.Status {
		case clean.StatusDone, clean.StatusDryRun:
			switch o.Action {
			case "command":
				byTool += o.Estimate
				nTool++
			case "delete":
				deleted += o.Estimate
				nDel++
			default:
				trashed += o.Estimate
				nTrash++
			}
		default:
			bad = append(bad, o)
		}
		if o.Freed > 0 {
			freed += o.Freed
		}
	}
	if mode == clean.DryRun {
		fmt.Fprintf(w, "\nDry run: would move %d item(s), %s, to the %s and run %d official command(s) (%s). Nothing changed.\n",
			nTrash, format.Bytes(trashed), clean.TrashName, nTool, format.Bytes(byTool))
	} else {
		fmt.Fprintln(w, "\nnodekill receipt")
		if nTrash > 0 {
			fmt.Fprintf(w, "  ✓ Moved to %-9s %9s  %d item(s); restore from the %s\n", clean.TrashName, format.Bytes(trashed), nTrash, clean.TrashName)
		}
		if nTool > 0 {
			fmt.Fprintf(w, "  ✓ Freed by tools     %9s  %d official command(s)\n", format.Bytes(byTool), nTool)
		}
		if nDel > 0 {
			fmt.Fprintf(w, "  ✓ Deleted            %9s  %d item(s), permanently\n", format.Bytes(deleted), nDel)
		}
		fmt.Fprintf(w, "  Free space changed by %s (measured)", signed(freed))
		if nTrash > 0 {
			fmt.Fprintf(w, "; another ~%s when you empty the %s", format.Bytes(trashed), clean.TrashName)
		}
		fmt.Fprintln(w)
	}
	for _, o := range bad {
		fmt.Fprintf(w, "  ✗ %s %s: %s\n", o.Status, format.Tilde(o.Path, home), o.Error)
	}
}

func signed(n int64) string {
	if n > 0 {
		return "+" + format.Bytes(n)
	}
	return format.Bytes(max(n, 0))
}
