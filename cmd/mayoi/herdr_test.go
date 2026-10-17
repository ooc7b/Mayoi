package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo"

	"mayoi/locales"
)

// herdrOn asks for herdr's terminals, which the panel shows no sign of
// until View → herdr Terminals is checked: it is off until asked for.
func herdrOn(a *app) { a.settings.Herdr = true }

func TestHerdrSections(t *testing.T) {
	out := "##pane\n{\"result\":{\"panes\":[]}}\n##workspace\n{\"result\":{\"workspaces\":[]}}\n##tab\n{\"result\":{\"tabs\":[]}}\n"
	sections, missing := herdrSections([]byte(out))
	if missing {
		t.Fatal("herdr taken for missing")
	}
	for _, name := range []string{"pane", "workspace", "tab"} {
		if !strings.Contains(sections[name], "result") {
			t.Errorf("section %q is %q", name, sections[name])
		}
	}
	if _, missing := herdrSections([]byte("##missing\n")); !missing {
		t.Error("herdr there, and not")
	}
}

func TestHerdrJSON(t *testing.T) {
	result, err := herdrJSON([]byte("an update is available\n{\"id\":\"cli:pane:list\",\"result\":{\"panes\":[]}}\n"))
	if err != nil || !strings.Contains(string(result), "panes") {
		t.Fatalf("herdrJSON = %q, %v", result, err)
	}
	_, err = herdrJSON([]byte(`{"id":"cli:pane:list","error":{"code":"server_not_running","message":"no herdr server is running"}}`))
	var failure *herdrFailure
	if !errors.As(err, &failure) || failure.Code != "server_not_running" {
		t.Fatalf("the failure is %v", err)
	}
	if _, err := herdrJSON([]byte("nonsense\n")); err == nil {
		t.Error("nonsense read as JSON")
	}
}

func TestHerdrRows(t *testing.T) {
	root := "/src/mayoi"
	panes := []herdrPane{
		{PaneID: "w2:p1", TerminalID: "t4", WorkspaceID: "w2", TabID: "w2:t1", Cwd: "/src/mayoi", Agent: "codex"},
		{PaneID: "w1:p2", TerminalID: "t2", WorkspaceID: "w1", TabID: "w1:t2", Cwd: "/src/mayoi/web", Label: "web"},
		{PaneID: "w1:p1", TerminalID: "t1", WorkspaceID: "w1", TabID: "w1:t1", Cwd: "/src/mayoix"},
		{PaneID: "w1:p3", TerminalID: "t3", WorkspaceID: "w1", TabID: "w1:t3", Cwd: "/tmp"},
		{PaneID: "w3:p1", TerminalID: "t5", WorkspaceID: "w3", TabID: "w3:t1", Cwd: "/src/mayoi", TerminalTitle: "zsh"},
	}
	workspaces := []herdrWorkspace{{ID: "w1", Label: "Mayoi", Number: 1}, {ID: "w2", Label: "Agents", Number: 2}, {ID: "w3", Number: 3}}
	tabs := []herdrTab{{ID: "w1:t1", Number: 1}, {ID: "w1:t2", Number: 2}, {ID: "w2:t1", Number: 1}, {ID: "w3:t1", Number: 2}}
	var got []string
	for _, row := range herdrRows(panes, workspaces, tabs, []string{root}) {
		got = append(got, row.pane.TerminalID+" "+row.name+" | "+row.what)
	}
	// Of the folder, and under it, in the order of their workspaces; the
	// one of a folder whose name starts the same stays out.
	want := []string{
		"t2 web | Mayoi · web",
		"t4 codex | Agents",
		"t5 zsh | ",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// TestHerdrRowsUseTitles: a terminal of herdr goes by the name it was
// given there, else by the title its program set, else by its agent, else
// by the number of its tab, as herdr shows it.
func TestHerdrRowsUseTitles(t *testing.T) {
	panes := []herdrPane{
		{PaneID: "1", Cwd: "/src", Label: "frontend", TabID: "w:t1"},
		{PaneID: "2", Cwd: "/src", TabID: "w:t2"},
		{PaneID: "3", Cwd: "/src", TabID: "w:t3", StrippedTitle: "building"},
		{PaneID: "4", Cwd: "/src", TabID: "w:t4", TerminalTitle: "⠋ building", DisplayAgent: "codex"},
		{PaneID: "5", Cwd: "/src", TabID: "w:t5", DisplayAgent: "codex"},
		{PaneID: "6", Cwd: "/src", TabID: "w:t6", Session: herdrSession{Agent: "claude"}},
		{PaneID: "7", Cwd: "/src", TabID: "w:t7"},
	}
	var tabs []herdrTab
	for i, label := range []string{"1", "api", "3", "4", "5", "6", "7"} {
		tabs = append(tabs, herdrTab{ID: "w:t" + strconv.Itoa(i+1), Label: label, Number: i + 1})
	}
	var got []string
	for _, row := range herdrRows(panes, nil, tabs, []string{"/src"}) {
		got = append(got, row.name)
	}
	// A tab named in herdr keeps its name; one still numbered is named
	// by its program, else its agent, else that number.
	want := []string{"frontend", "api", "building", "⠋ building", "codex", "claude", "7"}
	if !slices.Equal(got, want) {
		t.Fatalf("names %q, want %q", got, want)
	}
}

// TestHerdrShowsInThePanel: the terminal that shows one of herdr is a
// terminal of the panel, which opens on it, and not a tab of the column.
func TestHerdrShowsInThePanel(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	a.window.PanelOpen, a.window.TerminalOpen = false, false
	a.newTerminalInArgv(a.workDir(), []string{"sleep", "5"}, "codex")
	defer a.closeTerminals()
	if a.panelTerm == nil {
		t.Fatal("the panel shows no terminal")
	}
	if !a.window.PanelOpen || !a.window.TerminalOpen {
		t.Error("the panel did not open")
	}
	if a.panelTerm.title != "codex" {
		t.Errorf("the terminal is named %q", a.panelTerm.title)
	}
	if n := len(a.column().tabs); n != 0 {
		t.Errorf("the column gained %d tabs", n)
	}
}

// TestHerdrTerminalsWearItsLogo: a terminal that runs herdr is marked by
// herdr's logo, where one that runs a shell is marked by its prompt.
func TestHerdrTerminalsWearItsLogo(t *testing.T) {
	for _, c := range []struct {
		argv []string
		want bool
	}{
		{nil, false},
		{[]string{"sleep", "5"}, false},
		{[]string{"herdr", "terminal", "attach", "term_a"}, true},
		{[]string{"/opt/homebrew/bin/herdr"}, true},
	} {
		if got := runsHerdr(c.argv); got != c.want {
			t.Errorf("%q runs herdr: %v", c.argv, got)
		}
	}
	if termIcon(&session{herdr: true}) != iconHerdr {
		t.Error("a terminal of herdr wears another mark")
	}
	if termIcon(&session{}) != iconTerminal || termIcon(nil) != iconTerminal {
		t.Error("a shell wears another mark")
	}
}

// TestHerdrIn: a folder of herdr is of the list's folder when it is that
// folder, under it, or where the shell there moved to.
func TestHerdrIn(t *testing.T) {
	root := "/src/mayoi"
	for cwd, want := range map[string]bool{
		"/src/mayoi":     true,
		"/src/mayoi/":    true,
		"/src/mayoi/web": true,
		"/src/mayoix":    false,
		"/src":           false,
	} {
		if _, ok := herdrIn(herdrPane{Cwd: cwd}, []string{root}); ok != want {
			t.Errorf("%q is of %q: %v", cwd, root, ok)
		}
	}
	if _, ok := herdrIn(herdrPane{Cwd: "/src", ForegroundCwd: "/src/mayoi/web"}, []string{root}); !ok {
		t.Error("the folder of the program in front is not looked at")
	}
}

func TestHerdrCommandLine(t *testing.T) {
	argv, err := herdrCommandLine("/ssh:box/srv/app", "terminal", "attach", "term_1")
	if err != nil || !slices.Equal(argv, []string{"herdr", "terminal", "attach", "term_1"}) {
		t.Fatalf("a terminal of a host runs %v, %v", argv, err)
	}
	if testing.Short() || herdrCommand() == "" {
		return
	}
	argv, err = herdrCommandLine("/tmp", "terminal", "attach", "term_1")
	if err != nil || len(argv) != 4 || argv[0] == "herdr" || argv[3] != "term_1" {
		t.Fatalf("a terminal here runs %v, %v", argv, err)
	}
}

// TestHerdrLists reads herdr as the machine running the tests has it:
// herdr need not be there, and need not run, and what it reports is of
// the folder all the same.
func TestHerdrLists(t *testing.T) {
	if testing.Short() {
		t.Skip("herdr runs a command")
	}
	dir := t.TempDir()
	info := readHerdr(dir)
	if herdrCommand() == "" {
		if info.installed || info.err != "" {
			t.Fatalf("herdr is not here, and %+v", info)
		}
		return
	}
	if !info.installed || info.err != "" {
		t.Fatalf("herdr is here, and %+v", info)
	}
	for _, row := range info.rows {
		if cwd := row.pane.ForegroundCwd; cwd != "" && !strings.HasPrefix(cwd, filepath.Clean(dir)) {
			t.Errorf("%s is of %s, not %s", row.pane.PaneID, cwd, dir)
		}
	}
}

// TestHerdrChipShot shows the chips of the panel: that of a terminal of
// herdr, which wears its logo, beside that of a shell, which wears a
// prompt, in a dark window and a light one.
func TestHerdrChipShot(t *testing.T) {
	if testing.Short() {
		t.Skip("the chips show terminals")
	}
	for _, dark := range []bool{true, false} {
		hs := newHarness(t, dark)
		a := hs.a
		a.window.PanelOpen, a.window.TerminalOpen = true, true
		a.newTerminalInArgv(a.workDir(), []string{"sleep", "5"}, "codex")
		// A terminal of herdr, as the attach leaves it.
		a.panelTerm.herdr = true
		a.newTerminalInArgv(a.workDir(), nil, "zsh")
		hs.settle()
		suffix := "-light"
		if dark {
			suffix = "-dark"
		}
		hs.shot("herdr-chips" + suffix)
		// Moved to the editor area, the same terminal wears the same
		// mark in its tab.
		a.terminalToColumn(a.terms[0], a.column(), a.column().active+1)
		hs.settle()
		hs.shot("herdr-tab" + suffix)
		a.closeTerminals()
	}
}

// TestHerdrMenuShot shows the menu of the panel as it lists terminals, in
// a dark window and a light one. What it lists is made up here, so the
// shot needs no herdr.
func TestHerdrMenuShot(t *testing.T) {
	if testing.Short() {
		t.Skip("the menu shows terminals")
	}
	for _, dark := range []bool{true, false} {
		hs := newHarness(t, dark)
		a := hs.a
		herdrOn(a)
		a.window.PanelOpen, a.window.TerminalOpen = true, true
		a.run("view.newTerminal")
		hs.settle()
		a.herdr.open = true
		a.herdr.info = herdrInfo{installed: true, running: true, rows: []herdrRow{
			{pane: herdrPane{TerminalID: "term_a", WorkspaceID: "w1"}, name: "codex", what: "Mayoi"},
			{pane: herdrPane{TerminalID: "term_c", WorkspaceID: "w1"}, name: "1", what: "Mayoi · web"},
			{pane: herdrPane{TerminalID: "term_b", WorkspaceID: "w1"}, name: "herdr"},
		}}
		hs.settle()
		if _, ok := hs.tt.Find("codex"); !ok {
			t.Fatalf("the menu lists no terminal: %q", hs.tt.Texts())
		}
		if _, ok := hs.tt.Find(locales.CmdViewNewTerminal()); !ok {
			t.Fatalf("the menu has no New Terminal: %q", hs.tt.Texts())
		}
		if dark {
			hs.shot("herdr-dark")
		} else {
			hs.shot("herdr-light")
		}
		a.closeTerminals()
	}
}

// TestHerdrMenuSaysWhenMissing shows the menu where herdr is not.
func TestHerdrMenuSaysWhenMissing(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	herdrOn(a)
	a.window.PanelOpen, a.window.TerminalOpen = true, true
	a.run("view.newTerminal")
	hs.settle()
	a.herdr.open, a.herdr.info = true, herdrInfo{}
	hs.settle()
	if _, ok := hs.tt.Find(locales.HerdrMissing()); !ok {
		t.Fatalf("the menu does not say herdr is missing: %q", hs.tt.Texts())
	}
	if _, ok := hs.tt.Find(locales.CmdViewNewTerminal()); ok {
		t.Error("a menu without herdr offers to make a terminal")
	}
	a.closeTerminals()
}

// TestHerdrMenuClosesWithThePanel checks the menu goes with the panel.
func TestHerdrMenuClosesWithThePanel(t *testing.T) {
	hs := newHarness(t, true)
	a := hs.a
	herdrOn(a)
	a.window.PanelOpen, a.window.TerminalOpen = true, true
	a.run("view.newTerminal")
	hs.settle()
	a.herdr.open = true
	hs.settle()
	a.hidePanel()
	if a.herdr.open {
		t.Error("the menu of a hidden panel stayed open")
	}
	a.closeTerminals()
}

// herdrStub puts a herdr in PATH that writes the commands it is given to
// a file, and answers reply, and returns that file.
func herdrStub(t *testing.T, reply string) string {
	t.Helper()
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> '" + calls + "'\nprintf '%s\\n' '" + reply + "'\n"
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return calls
}

// herdrCalls are the commands herdrStub was told.
func herdrCalls(t *testing.T, path string) []string {
	t.Helper()
	out, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
}

// TestHerdrRenameTellsHerdr: the name of a terminal of herdr is the name
// of its tab, which herdr shows in its tab bar, and the name of its pane
// goes with it, which would be the one Mayoi showed in the tab's place.
func TestHerdrRenameTellsHerdr(t *testing.T) {
	calls := herdrStub(t, `{"result":{}}`)
	dir := t.TempDir()
	if err := renameInHerdr(dir, herdrRef{tab: "w1:t1", pane: "w1:p1"}, "api server"); err != nil {
		t.Fatal(err)
	}
	want := []string{"tab rename w1:t1 api server", "pane rename w1:p1 --clear"}
	if got := herdrCalls(t, calls); !slices.Equal(got, want) {
		t.Fatalf("herdr was told %q, want %q", got, want)
	}
	// A terminal whose pane is not known is named by its tab alone.
	if err := renameInHerdr(dir, herdrRef{tab: "w1:t1"}, "api"); err != nil {
		t.Fatal(err)
	}
	if got := herdrCalls(t, calls); len(got) != 3 || got[2] != "tab rename w1:t1 api" {
		t.Fatalf("herdr was told %q", got)
	}
}

// TestHerdrRenameFails: a rename that herdr refuses is its own failure,
// and leaves the pane of the terminal alone.
func TestHerdrRenameFails(t *testing.T) {
	calls := herdrStub(t, `{"error":{"code":"tab_not_found","message":"no tab w9:t9"}}`)
	err := renameInHerdr(t.TempDir(), herdrRef{tab: "w9:t9", pane: "w9:p1"}, "api")
	if err == nil || err.Error() != "no tab w9:t9" {
		t.Fatalf("the rename failed as %v", err)
	}
	if got := herdrCalls(t, calls); !slices.Equal(got, []string{"tab rename w9:t9 api"}) {
		t.Fatalf("herdr was told %q after failing", got)
	}
}

// TestHerdrRenameFromThePanel: the menu of a terminal of herdr offers a
// name for it, and the prompt that opens asks with the name it has; a
// shell has no name in herdr to give.
func TestHerdrRenameFromThePanel(t *testing.T) {
	if testing.Short() {
		t.Skip("starts terminals")
	}
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	t.Cleanup(a.closeTerminals)
	herdrOn(a)
	a.window.PanelOpen, a.window.TerminalOpen = true, true
	s := a.newTerminalInArgv(a.workDir(), []string{"sleep", "5"}, "codex")
	// A terminal of herdr, as the attach leaves it.
	s.herdr, s.herdrAt = true, herdrRef{tab: "w1:t1", pane: "w1:p1"}
	a.newTerminalInArgv(a.workDir(), nil, "zsh")
	hs.settle()
	if err := tt.RightClick("1: codex"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), locales.RenameEllipsis()) {
		t.Fatalf("a terminal of herdr offers no name: %q", tt.Menu())
	}
	if err := tt.ChooseMenuItem(locales.RenameEllipsis()); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	hs.shot("herdr-rename-prompt")
	if a.prompt == nil || a.prompt.value != "codex" {
		t.Fatalf("the prompt asks with %+v", a.prompt)
	}
	if msg := a.prompt.done("  "); msg != locales.HerdrRenameNeeded() {
		t.Errorf("an empty name is taken: %q", msg)
	}
	a.prompt = nil
	hs.settle()
	if err := tt.RightClick("2: zsh"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(tt.Menu(), locales.RenameEllipsis()) {
		t.Errorf("a shell offers a name in herdr: %q", tt.Menu())
	}
	tt.CloseMenu()
	// Moved to the editor area, its tab offers the same name. A file is
	// shown after it, so that the title bar names that file and the tab
	// of the terminal is the one that goes by the name alone.
	a.terminalToColumn(s, a.column(), a.column().active+1)
	a.openFile(hs.path("main.go"), true)
	hs.settle()
	if err := tt.RightClick("codex"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), locales.RenameEllipsis()) {
		t.Errorf("a tab of a terminal of herdr offers no name: %q", tt.Menu())
	}
	// The menu of a tab, not that of a chip of the panel.
	if !slices.Contains(tt.Menu(), locales.MoveToPanel()) {
		t.Errorf("the menu of the tab is not shown: %q", tt.Menu())
	}
	tt.CloseMenu()
}

// TestHerdrTerminalsDetach: taking a terminal of herdr out of the panel
// leaves it running in herdr's server, so the menu and the button that do
// it detach from it rather than kill it; a shell's still say kill.
func TestHerdrTerminalsDetach(t *testing.T) {
	if testing.Short() {
		t.Skip("starts terminals")
	}
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	t.Cleanup(a.closeTerminals)
	a.window.PanelOpen, a.window.TerminalOpen = true, true
	s := a.newTerminalInArgv(a.workDir(), []string{"sleep", "5"}, "codex")
	// A terminal of herdr, as the attach leaves it.
	s.herdr, s.herdrAt = true, herdrRef{tab: "w1:t1", pane: "w1:p1"}
	a.newTerminalInArgv(a.workDir(), nil, "zsh")
	hs.settle()
	if err := tt.RightClick("1: codex"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), locales.TermDetach()) {
		t.Fatalf("a terminal of herdr offers no detach: %q", tt.Menu())
	}
	if slices.Contains(tt.Menu(), locales.TermKill()) {
		t.Errorf("a terminal of herdr offers to be killed: %q", tt.Menu())
	}
	tt.CloseMenu()
	if err := tt.RightClick("2: zsh"); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(tt.Menu(), locales.TermKill()) {
		t.Fatalf("a shell offers no way to end it: %q", tt.Menu())
	}
	if slices.Contains(tt.Menu(), locales.TermDetach()) {
		t.Errorf("a shell offers to detach: %q", tt.Menu())
	}
	tt.CloseMenu()
	// The button of the terminal the panel shows says the same.
	if err := tt.Click("1: codex"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if _, ok := tt.Find(locales.TermDetach()); !ok {
		t.Errorf("the panel's button does not say Detach: %q", tt.Texts())
	}
	hs.shot("herdr-detach")
	if err := tt.Click("2: zsh"); err != nil {
		t.Fatal(err)
	}
	hs.settle()
	if _, ok := tt.Find(locales.TermKill()); !ok {
		t.Errorf("the panel's button does not say Kill: %q", tt.Texts())
	}
}

// TestHerdrShowsOneTerminalOnce: a terminal of herdr the panel or a tab
// shows already is shown again, its panel chip or its tab coming to the
// front, rather than attached a second time.
func TestHerdrShowsOneTerminalOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("starts terminals")
	}
	hs := newHarness(t, true)
	a := hs.a
	t.Cleanup(a.closeTerminals)
	a.window.PanelOpen, a.window.TerminalOpen = true, true
	s := a.newTerminalInArgv(a.workDir(), []string{"sleep", "5"}, "codex")
	// A terminal of herdr, as the attach leaves it.
	s.herdr, s.herdrAt = true, herdrRef{tab: "w1:t1", pane: "w1:p1"}
	// Another, which the panel shows instead, and then hides.
	a.newTerminalInArgv(a.workDir(), nil, "zsh")
	hs.settle()
	a.hidePanel()
	hs.settle()
	before := len(a.terms)
	row := herdrRow{pane: herdrPane{PaneID: "w1:p1", TerminalID: "t1", TabID: "w1:t1"}, name: "codex"}
	a.attachHerdr(row)
	hs.settle()
	if n := len(a.terms); n != before {
		t.Fatalf("%d terminals, from %d", n, before)
	}
	if a.panelTerm != s || !a.window.PanelOpen {
		t.Errorf("the panel shows %q, open %v", a.panelTerm.title, a.window.PanelOpen)
	}
	// In a tab, that tab is the one shown.
	a.terminalToColumn(s, a.column(), a.column().active+1)
	a.newTerminalInArgv(a.workDir(), nil, "bash")
	hs.settle()
	before = len(a.terms)
	a.attachHerdr(row)
	hs.settle()
	if n := len(a.terms); n != before {
		t.Fatalf("%d terminals, from %d", n, before)
	}
	if col := a.column(); col.tabs[col.active] != s.tab {
		t.Errorf("the tab shown is %q, not the terminal of herdr", col.tabs[col.active].path())
	}
	// One that nothing shows is attached, not shown.
	if a.showHerdr("w9:p9") || a.showHerdr("") {
		t.Error("a terminal nothing shows is found")
	}
}

// menuItem is the item of the menu bar that says label, in a menu or a
// submenu of it, or nil.
func menuItem(t *testing.T, a *app, label string) *mygo.MenuItem {
	t.Helper()
	var find func(items []*mygo.MenuItem) *mygo.MenuItem
	find = func(items []*mygo.MenuItem) *mygo.MenuItem {
		for _, it := range items {
			if it.Label == label {
				return it
			}
			if it := find(it.Submenu); it != nil {
				return it
			}
		}
		return nil
	}
	return find(a.menu(false, func(string, *mygo.Window) {}).Items())
}

// TestHerdrOffUntilAsked: herdr's terminals are off until they are asked
// for, which View → herdr Terminals does, a checkbox the menu and the
// palette keep clear. Asked for, the panel's button comes; turned off
// again, it goes, and a terminal of herdr offers no name.
func TestHerdrOffUntilAsked(t *testing.T) {
	if testing.Short() {
		t.Skip("the panel shows terminals")
	}
	hs := newHarness(t, true)
	a, tt := hs.a, hs.tt
	t.Cleanup(a.closeTerminals)
	a.window.PanelOpen, a.window.TerminalOpen = true, true
	hs.settle()
	if _, ok := tt.Find(locales.HerdrMenu()); ok {
		t.Fatalf("the panel shows herdr's button unasked: %q", tt.Texts())
	}
	if a.commandOn("view.herdr") {
		t.Error("the palette says herdr is on")
	}
	if it := menuItem(t, a, locales.CmdViewHerdr()); it == nil || it.Type != mygo.MenuItemCheckbox || it.Checked {
		t.Fatalf("View has no clear checkbox of herdr's terminals: %+v", it)
	}
	a.run("view.herdr")
	hs.settle()
	if _, ok := tt.Find(locales.HerdrMenu()); !ok {
		t.Fatalf("the panel has no button of herdr once asked: %q", tt.Texts())
	}
	if !a.commandOn("view.herdr") {
		t.Error("the palette does not say herdr is on")
	}
	if it := menuItem(t, a, locales.CmdViewHerdr()); it == nil || !it.Checked {
		t.Errorf("View's checkbox of herdr is not checked: %+v", it)
	}
	// A terminal of herdr, as the attach leaves it, stays a terminal of
	// the panel once herdr is off again, and only loses its name.
	s := a.newTerminalInArgv(a.workDir(), []string{"sleep", "5"}, "codex")
	s.herdr, s.herdrAt = true, herdrRef{tab: "w1:t1", pane: "w1:p1"}
	a.run("view.herdr")
	hs.settle()
	if _, ok := tt.Find(locales.HerdrMenu()); ok {
		t.Error("the panel's button of herdr stayed once it was turned off")
	}
	if err := tt.RightClick("1: codex"); err != nil {
		t.Fatal(err)
	}
	if slices.Contains(tt.Menu(), locales.RenameEllipsis()) {
		t.Errorf("a terminal of herdr offers a name with herdr off: %q", tt.Menu())
	}
	tt.CloseMenu()
}
