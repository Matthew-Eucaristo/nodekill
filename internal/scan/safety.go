package scan

import (
	"errors"
	"slices"
)

// Level is how freely an item may be removed.
type Level string

const (
	Safe      Level = "safe"      // rebuildable; may be preselected
	Review    Level = "review"    // allowed, never preselected: it costs something or is worth a look
	Protected Level = "protected" // shown for context; nodekill will not remove it
	Live      Level = "live"      // in use right now; nothing is allowed
)

func (l Level) rank() int { return slices.Index([]Level{Safe, Review, Protected, Live}, l) }

// Allowed reports whether nodekill may act on an item at this level.
func (l Level) Allowed() bool { return l == Safe || l == Review }

// Reason codes, part of the JSON contract.
const (
	ReasonInUse           = "in_use"           // live: a Node process works in the project
	ReasonToolRunning     = "tool_running"     // live: the package manager is running
	ReasonNotIgnored      = "not_ignored"      // protected: git does not ignore it
	ReasonTracked         = "tracked"          // protected: git tracks files inside
	ReasonGitError        = "git_error"        // protected: git could not answer
	ReasonNestedRepo      = "nested_repo"      // protected: a .git lies inside
	ReasonNoGit           = "no_git"           // protected: generic output outside a repository
	ReasonCatalog         = "catalog"          // protected: the catalog never removes it
	ReasonNotInstalled    = "not_installed"    // protected: the owning tool is missing, so its official command cannot run
	ReasonUnknownFormat   = "unknown_format"   // protected: a layout nodekill does not recognise
	ReasonCurrent         = "current"          // protected: the active Node version or a bundled package
	ReasonNoCommand       = "no_command"       // protected: its tool has no official way to remove it
	ReasonDependents      = "dependents"       // protected: other installed software needs it
	ReasonRecent          = "recent"           // review: project active in the last 7 days
	ReasonUnknownAge      = "unknown_age"      // review: activity could not be determined
	ReasonNoLockfile      = "no_lockfile"      // review: reinstalling may resolve newer versions
	ReasonAmbiguousPM     = "ambiguous_pm"     // review: several package managers' lockfiles
	ReasonLocalChanges    = "local_changes"    // review: files changed after the install
	ReasonLivenessUnknown = "liveness_unknown" // review: running processes could not be checked
	ReasonDeployable      = "deployable"       // review: build output that may be what you ship
	ReasonCatalogReview   = "catalog_review"   // review: the catalog marks it for a look
	ReasonSharedStore     = "shared_store"     // review: other projects depend on it (e.g. Bun's global store)
)

// Reason explains one piece of evidence.
type Reason struct {
	Code   string `json:"code"`
	Level  Level  `json:"level"`
	Detail string `json:"detail"`
}

// Safety is the level and the evidence behind it.
type Safety struct {
	Level   Level    `json:"level"`
	Reasons []Reason `json:"reasons"`
}

func (s *Safety) add(code string, l Level, detail string) {
	for _, r := range s.Reasons {
		if r.Code == code {
			return
		}
	}
	s.Reasons = append(s.Reasons, Reason{code, l, detail})
	if l.rank() > s.Level.rank() {
		s.Level = l
	}
}

// Has reports whether a reason code is present.
func (s Safety) Has(code string) bool {
	for _, r := range s.Reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

// ReasonInfo documents a reason code for `capabilities`.
type ReasonInfo struct {
	Code   string `json:"code"`
	Level  Level  `json:"level"`
	Effect string `json:"effect"`
}

// Reasons lists every code nodekill can emit.
var Reasons = []ReasonInfo{
	{ReasonInUse, Live, "blocks every action: a Node process (dev server, build, test runner) works in the project"},
	{ReasonToolRunning, Live, "blocks the cache command while the package manager runs"},
	{ReasonNotIgnored, Protected, "blocks: git does not ignore the directory, so it may be source"},
	{ReasonTracked, Protected, "blocks: git tracks files inside"},
	{ReasonGitError, Protected, "blocks: git could not confirm it is ignored and untracked"},
	{ReasonNestedRepo, Protected, "blocks: another repository lies inside"},
	{ReasonNoGit, Protected, "blocks: a generic output name outside a repository could be anything"},
	{ReasonCatalog, Protected, "blocks: holds your data (renders, local databases), never a cache"},
	{ReasonNotInstalled, Protected, "blocks: the owning tool is not installed, so its official cleanup cannot run; the item shows the command"},
	{ReasonUnknownFormat, Protected, "blocks: the layout is not one nodekill recognises"},
	{ReasonCurrent, Protected, "blocks: the Node version in use or set as default, or a package that ships with Node"},
	{ReasonNoCommand, Protected, "blocks: its tool has no official way to remove it (e.g. Volta's Node versions)"},
	{ReasonDependents, Protected, "blocks: other installed software depends on it (e.g. Homebrew formulae)"},
	{ReasonRecent, Review, "not preselected: the project changed in the last 7 days"},
	{ReasonUnknownAge, Review, "not preselected: the project's last activity is unknown"},
	{ReasonNoLockfile, Review, "not preselected: without a lockfile a reinstall may resolve newer versions"},
	{ReasonAmbiguousPM, Review, "not preselected: lockfiles of several package managers"},
	{ReasonLocalChanges, Review, "not preselected: files inside changed after the install (patches?)"},
	{ReasonLivenessUnknown, Review, "not preselected: running processes could not be checked"},
	{ReasonDeployable, Review, "not preselected: build output that may be what you deploy or publish"},
	{ReasonCatalogReview, Review, "not preselected: rebuildable, but worth a look first"},
	{ReasonSharedStore, Review, "not preselected: other projects use it and must reinstall afterwards"},
}

var (
	errSymlink = errors.New("is a symlink")
	errNotDir  = errors.New("is not a directory")
)
