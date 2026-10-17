package main

// herdr runs terminals in a server of its own, which keeps them running
// when the window that showed them closes: the panel's herdr button lists
// the ones of the folder and shows them, in the panel, by running herdr's
// command line, which is all that is known of herdr here.
//
// herdr is nobody's requirement: a machine without it, or a host of a
// remote folder without it, has the menu say so, and the rest of the
// editor is as it was. What the menu offers is read again each time it
// opens, so a terminal started in herdr shows up there.

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mayoi/internal/remote"
	"mayoi/locales"
)

// herdrMark is the path of herdr's mark: a ram's head with a prompt in
// it, drawn on a square of its own, whose right and lower edges the head
// runs off. Mayoi knows herdr by its command line alone, so the mark is
// herdr's own, taken as it is from assets/logo.svg of herdrdev/herdr,
// with the square behind it left out.
const herdrMark = `M2794 3710 c-129 -33 -299 -135 -359 -214 -21 -28 -26 -42 -21 -63 9 -38 154 -178 199 -192 32 -11 41 -9 104 23 171 86 354 70 475 -43 150 -138 150 -379 0 -511 -107 -95 -278 -94 -386 2 l-46 40 -11 -29 c-16 -40 -14 -122 4 -164 60 -144 264 -222 452 -174 360 92 559 494 430 868 -36 103 -81 173 -175 267 -71 72 -100 93 -180 132 -52 26 -127 54 -167 62 -96 21 -230 20 -319 -4z M2183 3695 c-116 -32 -221 -108 -273 -199 -17 -28 -30 -54 -30 -58 0 -4 20 1 45 12 66 28 220 68 294 76 64 7 65 7 137 83 40 41 71 77 69 79 -2 2 -21 8 -42 13 -55 12 -140 10 -200 -6z M2212 3388 c-159 -22 -390 -122 -559 -241 -299 -210 -585 -600 -609 -828 -12 -118 40 -251 125 -318 96 -76 178 -98 426 -116 110 -8 224 -21 254 -29 125 -34 230 -115 272 -211 11 -24 24 -81 30 -127 20 -170 65 -271 166 -374 34 -35 63 -65 63 -67 0 -1 -10 -27 -22 -57 -29 -76 -37 -259 -14 -350 42 -170 158 -318 311 -397 44 -23 98 -46 120 -52 22 -6 45 -14 51 -18 5 -5 15 -48 22 -96 6 -48 14 -92 17 -97 4 -6 415 -10 1131 -10 l1124 0 0 1584 0 1585 -55 -19 c-84 -29 -143 -68 -232 -154 l-83 -78 -54 49 c-111 102 -233 151 -391 160 -113 6 -199 -10 -298 -54 l-60 -27 -26 34 c-37 51 -120 134 -127 128 -3 -4 0 -34 7 -67 17 -89 7 -268 -21 -356 -103 -328 -377 -545 -688 -545 -161 0 -273 41 -373 137 -37 35 -66 75 -85 116 -79 173 -8 407 124 407 44 0 68 -14 117 -66 74 -78 167 -82 238 -9 60 62 72 147 33 231 -42 90 -120 130 -238 122 -50 -4 -85 -14 -131 -37 -89 -45 -122 -52 -176 -40 -92 20 -262 163 -302 253 -11 25 -21 45 -22 45 -1 -1 -30 -6 -65 -11z m-256 -505 c115 -88 129 -102 132 -131 2 -18 -2 -40 -9 -49 -7 -8 -69 -55 -138 -105 -103 -73 -130 -88 -151 -83 -35 8 -51 34 -48 74 3 31 12 42 83 92 44 31 80 61 82 66 1 4 -30 31 -69 58 -39 28 -77 56 -85 63 -18 19 -16 72 4 94 32 35 63 23 199 -79z m528 -88 c23 -24 28 -52 14 -82 l-13 -28 -141 -3 c-130 -2 -142 -1 -158 17 -23 26 -24 66 -1 91 16 18 32 20 151 20 105 0 136 -3 148 -15z`

// iconHerdr is that mark, in the color of the text. Unlike Lucide's
// icons it is solid, not stroked, and its view box is cropped to the
// head, which herdr draws in the corner of a square of its own.
//
// herdr's logo is under the Apache License 2.0, like the rest of herdr.
// This one leaves the square behind it out, and takes the color of the
// text rather than the gray herdr gives it.
var iconHerdr = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="51.5 69 512 512" fill="currentColor">` +
	`<g transform="translate(0 512) scale(.1 -.1)" stroke="none"><path d="` + herdrMark + `"/></g></svg>`))

// errHerdrMissing is herdr not being where this machine keeps it.
var errHerdrMissing = errors.New("Herdr is not installed")

// herdrMenu is the button of the panel and the box it opens: what the
// last look at herdr found, and the look in flight.
type herdrMenu struct {
	open bool
	// gen tells the look shown from those started before it, which are
	// dropped as they come back.
	gen  int
	busy bool
	info herdrInfo
}

// herdrInfo is what a look at herdr found: whether it is installed,
// whether a server answers, and the terminals of the folder.
type herdrInfo struct {
	installed bool
	running   bool
	rows      []herdrRow
	err       string // why herdr could not be read, in its own words
}

// herdrPane is a terminal of herdr's, as `herdr pane list` tells it.
type herdrPane struct {
	PaneID        string `json:"pane_id"`
	TerminalID    string `json:"terminal_id"`
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	Cwd           string `json:"cwd"`
	ForegroundCwd string `json:"foreground_cwd"`
	// Label is the name the terminal was given in herdr, empty for one
	// never renamed.
	Label string `json:"label"`
	Agent string `json:"agent"`
	// DisplayAgent is how herdr names the agent of the terminal, and
	// Session the agent it started there.
	DisplayAgent string       `json:"display_agent"`
	Session      herdrSession `json:"agent_session"`
	// Title is what herdr calls the terminal, TerminalTitle the title
	// the program in it set last, and StrippedTitle that title without
	// the spinner of an agent.
	Title         string `json:"title"`
	TerminalTitle string `json:"terminal_title"`
	StrippedTitle string `json:"terminal_title_stripped"`
	Focused       bool   `json:"focused"`
}

// herdrSession is the agent herdr runs in a terminal.
type herdrSession struct {
	Agent string `json:"agent"`
}

// herdrWorkspace and herdrTab are what names a terminal's place.
type herdrWorkspace struct {
	ID     string `json:"workspace_id"`
	Label  string `json:"label"`
	Number int    `json:"number"`
}

type herdrTab struct {
	ID     string `json:"tab_id"`
	Label  string `json:"label"`
	Number int    `json:"number"`
}

// herdrRow is a terminal of herdr the menu offers: what it calls it, and
// where it is.
type herdrRow struct {
	pane herdrPane
	name string
	// what is its workspace and its folder, when it is under the folder
	// the menu lists: "Mayoi · src/tools".
	what string
}

// herdrRef is the terminal of herdr a session shows, as a rename names
// it: its tab, whose name herdr shows in its tab bar, and its pane, whose
// own name Mayoi would show in place of the tab's.
type herdrRef struct {
	tab  string
	pane string
}

// herdrCommand is the herdr of this machine. An app opened from the
// Finder has not the PATH of the shell, so the folders installers put it
// in are looked at too.
func herdrCommand() string {
	if path, err := exec.LookPath("herdr"); err == nil {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, path := range []string{
		filepath.Join(home, ".local", "bin", "herdr"),
		"/opt/homebrew/bin/herdr",
		"/usr/local/bin/herdr",
	} {
		if info, err := os.Stat(path); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return path
		}
	}
	return ""
}

// herdrCommandLine is the command that runs herdr with args on the
// machine of dir: here, or on the host of a remote folder, where the
// terminal runs it over the connection.
func herdrCommandLine(dir string, args ...string) ([]string, error) {
	if remote.Is(dir) {
		return append([]string{"herdr"}, args...), nil
	}
	path := herdrCommand()
	if path == "" {
		return nil, errHerdrMissing
	}
	return append([]string{path}, args...), nil
}

// runsHerdr reports whether argv runs herdr itself: the attach of a
// terminal of its own, or herdr alone, which starts the server of a
// machine that has none running yet.
func runsHerdr(argv []string) bool {
	return len(argv) > 0 && filepath.Base(argv[0]) == "herdr"
}

// termIcon is the mark of a terminal, where its name is: herdr's logo for
// one that runs herdr, and the prompt of a shell for one that does not.
func termIcon(s *session) *ui.SVG {
	if s != nil && s.herdr {
		return iconHerdr
	}
	return iconTerminal
}

// detaches reports whether closing a terminal leaves the program behind:
// a terminal of herdr's goes on running in its server, so the button that
// takes it out of the panel detaches from it rather than kills it.
func detaches(s *session) bool {
	return s != nil && s.herdrAt.tab != ""
}

// closeTermLabel is what that button says, and closeTermIcon is its mark.
func closeTermLabel(s *session) string {
	if detaches(s) {
		return locales.TermDetach()
	}
	return locales.TermKill()
}

func closeTermIcon(s *session) *ui.SVG {
	if detaches(s) {
		return iconUnplug
	}
	return iconTrash
}

// herdrHostDir is dir as the machine that runs herdr spells it.
func herdrHostDir(dir string) string {
	if _, path, ok := remote.Split(dir); ok {
		return path
	}
	return dir
}

// herdrRun runs herdr with args on the machine of dir and returns what it
// printed. A command that fails says so in the JSON it prints, on its
// error output, so both go to the same place and the exit status is left
// to herdrJSON.
func herdrRun(dir string, args ...string) ([]byte, error) {
	if c, _, ok := remote.Lookup(dir); ok {
		return c.Run(nil, "\"$@\" 2>&1\n", append([]string{"herdr"}, args...)...)
	}
	path := herdrCommand()
	if path == "" {
		return nil, errHerdrMissing
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil && out.Len() == 0 {
		return nil, err
	}
	return out.Bytes(), nil
}

// herdrFailure is a herdr command that failed, as its JSON says why.
type herdrFailure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *herdrFailure) Error() string { return e.Message }

// herdrJSON returns what herdr printed, the first line of it that is
// JSON, and nil for the rest: herdr's warnings and update notices.
func herdrJSON(out []byte) (json.RawMessage, error) {
	for line := range strings.Lines(string(out)) {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r struct {
			Result json.RawMessage `json:"result"`
			Error  *herdrFailure   `json:"error"`
		}
		if json.Unmarshal([]byte(line), &r) != nil {
			continue
		}
		if r.Error != nil {
			return nil, r.Error
		}
		if len(r.Result) > 0 {
			return r.Result, nil
		}
	}
	return nil, errors.New("Herdr printed nothing Mayoi could read")
}

// herdrDo runs herdr with args on the machine of dir, and fails as herdr
// says it did.
func herdrDo(dir string, args ...string) error {
	out, err := herdrRun(dir, args...)
	if err != nil {
		return err
	}
	_, err = herdrJSON(out)
	return err
}

// renameInHerdr gives a terminal of herdr the name name: the name of its
// tab, which herdr calls the terminal by, and no name of its pane of its
// own, which Mayoi would show instead of the tab's.
func renameInHerdr(dir string, ref herdrRef, name string) error {
	if err := herdrDo(dir, "tab", "rename", ref.tab, name); err != nil {
		return err
	}
	if ref.pane == "" {
		return nil
	}
	return herdrDo(dir, "pane", "rename", ref.pane, "--clear")
}

// herdrListScript runs herdr's three listings on a host, one after
// another, each after a line naming it, and says when herdr is not there
// at all. It is one round trip over the connection, and it is a shell
// script because a host's shell is known to be one; here the listings run
// without a shell.
const herdrListScript = `if ! command -v "$1" >/dev/null 2>&1; then echo "##missing"; exit 0; fi
for what in pane workspace tab; do
  echo "##$what"
  "$@" "$what" list 2>&1
done
`

// herdrListings are what herdr says of dir's machine: one listing for
// each of pane, workspace and tab, and whether herdr is there at all.
func herdrListings(dir string) (sections map[string]string, missing bool, err error) {
	if c, _, ok := remote.Lookup(dir); ok {
		out, err := c.Run(nil, herdrListScript, "herdr")
		if err != nil {
			return nil, false, err
		}
		sections, missing := herdrSections(out)
		return sections, missing, nil
	}
	if herdrCommand() == "" {
		return nil, true, nil
	}
	sections = map[string]string{}
	for _, what := range []string{"pane", "workspace", "tab"} {
		out, err := herdrRun(dir, what, "list")
		if err != nil {
			return nil, false, err
		}
		sections[what] = string(out)
	}
	return sections, false, nil
}

// herdrSections parts what herdrListScript printed into its listings.
func herdrSections(out []byte) (sections map[string]string, missing bool) {
	sections = map[string]string{}
	name := ""
	var b strings.Builder
	finish := func() {
		if name != "" {
			sections[name] = b.String()
			b.Reset()
		}
	}
	for line := range strings.Lines(string(out)) {
		if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "##"); ok {
			finish()
			name = rest
			if name == "missing" {
				missing, name = true, ""
			}
			continue
		}
		b.WriteString(line)
	}
	finish()
	return sections, missing
}

// readHerdr looks at herdr in dir: here, or on the host of a remote
// folder.
func readHerdr(dir string) herdrInfo {
	info := herdrInfo{}
	sections, missing, err := herdrListings(dir)
	if err != nil {
		info.err = err.Error()
		return info
	}
	if missing {
		return info // not installed, and no server to ask
	}
	info.installed = true
	var panes struct {
		Panes []herdrPane `json:"panes"`
	}
	result, err := herdrJSON([]byte(sections["pane"]))
	if err != nil {
		var failure *herdrFailure
		if !errors.As(err, &failure) || failure.Code != "server_not_running" {
			info.err = err.Error()
		}
		return info
	}
	if err := json.Unmarshal(result, &panes); err != nil {
		info.err = err.Error()
		return info
	}
	info.running = true
	var workspaces struct {
		Workspaces []herdrWorkspace `json:"workspaces"`
	}
	if result, err := herdrJSON([]byte(sections["workspace"])); err == nil {
		json.Unmarshal(result, &workspaces)
	}
	var tabs struct {
		Tabs []herdrTab `json:"tabs"`
	}
	if result, err := herdrJSON([]byte(sections["tab"])); err == nil {
		json.Unmarshal(result, &tabs)
	}
	info.rows = herdrRows(panes.Panes, workspaces.Workspaces, tabs.Tabs, herdrRoots(dir))
	return info
}

// herdrRoots are the spellings of the folder the menu lists for: a path
// on its host, or one of this machine's, which a symlink spells twice.
func herdrRoots(dir string) []string {
	if _, path, ok := remote.Split(dir); ok {
		return []string{path}
	}
	roots := []string{filepath.Clean(dir)}
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		if real = filepath.Clean(real); real != roots[0] {
			roots = append(roots, real)
		}
	}
	return roots
}

// herdrIn returns the folder of a pane, when it is in the folder the menu
// lists or under it.
func herdrIn(p herdrPane, roots []string) (string, bool) {
	for _, cwd := range []string{p.Cwd, p.ForegroundCwd} {
		cwd = strings.TrimSuffix(cwd, "/")
		for _, root := range roots {
			if root != "" && (cwd == root || strings.HasPrefix(cwd, root+"/")) {
				return cwd, true
			}
		}
	}
	return "", false
}

// herdrTabName is the name the tab of a terminal was given in herdr, ""
// for one still going by its number: herdr labels a tab with a count
// until it is renamed.
func herdrTabName(t herdrTab) string {
	if _, err := strconv.Atoi(strings.TrimSpace(t.Label)); err == nil {
		return ""
	}
	return t.Label
}

// herdrRows are the rows of the terminals of roots, in the order of their
// workspaces and tabs.
func herdrRows(panes []herdrPane, workspaces []herdrWorkspace, tabs []herdrTab, roots []string) []herdrRow {
	ws := make(map[string]herdrWorkspace, len(workspaces))
	for _, w := range workspaces {
		ws[w.ID] = w
	}
	tb := make(map[string]herdrTab, len(tabs))
	for _, t := range tabs {
		tb[t.ID] = t
	}
	var rows []herdrRow
	for _, p := range panes {
		cwd, ok := herdrIn(p, roots)
		if !ok {
			continue
		}
		row := herdrRow{pane: p, name: cmp.Or(p.Label, herdrTabName(tb[p.TabID]), p.StrippedTitle,
			p.TerminalTitle, p.Title, p.DisplayAgent, p.Agent, p.Session.Agent)}
		if row.name == "" {
			// Nothing herdr knows it by: its number, as herdr shows it,
			// which is all a shell started there has.
			row.name = tb[p.TabID].Label
			if row.name == "" {
				row.name = strconv.Itoa(len(rows) + 1)
			}
		}
		// Where it is, under the name: its workspace and its folder when
		// that is deeper, which a terminal named after its workspace has
		// no need of twice.
		var what []string
		if label := ws[p.WorkspaceID].Label; label != "" && label != row.name {
			what = append(what, label)
		}
		for _, root := range roots {
			if sub, ok := strings.CutPrefix(cwd, root+"/"); ok {
				what = append(what, sub)
				break
			}
		}
		row.what = strings.Join(what, " · ")
		rows = append(rows, row)
	}
	slices.SortStableFunc(rows, func(a, b herdrRow) int {
		wa, wb := ws[a.pane.WorkspaceID].Number, ws[b.pane.WorkspaceID].Number
		if wa != wb {
			return cmp.Compare(wa, wb)
		}
		if ta, tb := tb[a.pane.TabID].Number, tb[b.pane.TabID].Number; ta != tb {
			return cmp.Compare(ta, tb)
		}
		return strings.Compare(a.pane.PaneID, b.pane.PaneID)
	})
	return rows
}

// toggleHerdrMenu opens the panel's herdr menu, reading herdr again as it
// opens, or closes it.
func (a *app) toggleHerdrMenu() {
	a.herdr.open = !a.herdr.open
	if a.herdr.open {
		a.loadHerdr()
	}
}

// toggleHerdr shows the panel's button of herdr's terminals, or takes it
// away. Off, nothing of herdr shows: the panel has no button of it, and a
// terminal of hers offers no name. The terminals of herdr already attached
// stay, as terminals of the panel.
func (a *app) toggleHerdr() {
	a.settings.Herdr = !a.settings.Herdr
	a.settings.Changed()
	if !a.settings.Herdr {
		// The menu closes, and a look at herdr still in flight is
		// dropped, with the answer coming back to it.
		a.herdr.gen++
		a.herdr.open, a.herdr.busy = false, false
	}
}

// loadHerdr reads herdr's terminals in the folder, in the background.
func (a *app) loadHerdr() {
	a.herdr.gen++
	gen, dir := a.herdr.gen, a.workDir()
	a.herdr.busy = true
	go func() {
		info := readHerdr(dir)
		a.post(func() {
			if a.herdr.gen != gen {
				return // another look was asked for
			}
			a.herdr.busy, a.herdr.info = false, info
		})
	}()
}

// herdrWorkspaceID is the workspace of herdr the folder is open in, ""
// when it has none.
func (a *app) herdrWorkspaceID() string {
	for _, row := range a.herdr.info.rows {
		if row.pane.Focused {
			return row.pane.WorkspaceID
		}
	}
	if len(a.herdr.info.rows) > 0 {
		return a.herdr.info.rows[0].pane.WorkspaceID
	}
	return ""
}

// attachHerdr shows a terminal of herdr in the panel, as a terminal of
// its own, by running herdr's attach there: on this machine, or on the
// host of a remote folder, where it goes over the connection. One the
// panel or a tab shows already is shown again, rather than attached.
func (a *app) attachHerdr(row herdrRow) {
	a.herdr.open = false
	// The one already shown, in the panel or in a tab, is shown again:
	// herdr's attach twice would give one terminal of herdr two views.
	if a.showHerdr(row.pane.PaneID) {
		return
	}
	dir := a.workDir()
	argv, err := herdrCommandLine(dir, "terminal", "attach", row.pane.TerminalID)
	if err != nil {
		a.fail("%s", locales.HerdrAttachFailed(err.Error()))
		return
	}
	if s := a.newTerminalInArgv(dir, argv, row.name); s != nil {
		s.herdrAt = herdrRef{tab: row.pane.TabID, pane: row.pane.PaneID}
	}
}

// showHerdr shows the terminal that already shows the terminal of herdr
// pane: the panel takes it, or the tab that holds it comes to the front.
// It says whether one did.
func (a *app) showHerdr(pane string) bool {
	if pane == "" {
		return false
	}
	for _, s := range a.terms {
		if s.herdrAt.pane != pane {
			continue
		}
		if t := s.tab; t != nil {
			if col, i := a.locate(t); col != nil {
				a.activate(col, i, true)
			}
			return true
		}
		a.panelTerm = s
		a.window.PanelOpen, a.window.TerminalOpen = true, true
		a.settings.Changed()
		a.focusTerminal()
		return true
	}
	return false
}

// renameHerdr asks for the name of a terminal of herdr and gives it that
// name in herdr, which then shows it in its own tab bar, and here.
func (a *app) renameHerdr(s *session) {
	ref, dir := s.herdrAt, s.dir
	if ref.tab == "" {
		return
	}
	a.prompt = &prompt{title: locales.RenameTitle(), hint: locales.HerdrRenameHint(), value: s.title, done: func(value string) string {
		name := strings.TrimSpace(value)
		if name == "" {
			return locales.HerdrRenameNeeded()
		}
		go func() {
			err := renameInHerdr(dir, ref, name)
			a.post(func() {
				switch {
				case err != nil:
					a.fail("%s", locales.FailRename(name, err.Error()))
				default:
					s.title = name
				}
			})
		}()
		return ""
	}}
}

// newHerdrTerminal makes a terminal in herdr and shows it in the panel:
// in the workspace of the folder, or in one herdr makes for it.
func (a *app) newHerdrTerminal() {
	dir, workspace := a.workDir(), a.herdrWorkspaceID()
	a.herdr.open = false
	go func() {
		pane, err := createHerdrTerminal(dir, workspace)
		a.post(func() {
			switch {
			case err != nil:
				a.fail("%s", locales.HerdrCreateFailed(err.Error()))
			case pane.TerminalID != "":
				a.attachHerdr(herdrRow{pane: pane, name: "herdr"})
			default:
				// No server answers: herdr starts one, and shows itself.
				argv, err := herdrCommandLine(dir)
				if err != nil {
					a.fail("%s", locales.HerdrCreateFailed(err.Error()))
					return
				}
				a.newTerminalInArgv(dir, argv, "herdr")
			}
		})
	}()
}

// createHerdrTerminal makes a terminal in herdr, in workspace when the
// folder has one, and returns its pane, empty when no herdr server runs.
func createHerdrTerminal(dir, workspace string) (herdrPane, error) {
	cwd := herdrHostDir(dir)
	args := []string{"workspace", "create", "--cwd", cwd, "--no-focus"}
	if workspace != "" {
		args = []string{"tab", "create", "--workspace", workspace, "--cwd", cwd, "--no-focus"}
	}
	out, err := herdrRun(dir, args...)
	if err != nil {
		return herdrPane{}, err
	}
	result, err := herdrJSON(out)
	if err != nil {
		var failure *herdrFailure
		if errors.As(err, &failure) && failure.Code == "server_not_running" {
			return herdrPane{}, nil
		}
		return herdrPane{}, err
	}
	var made struct {
		RootPane herdrPane `json:"root_pane"`
	}
	if err := json.Unmarshal(result, &made); err != nil {
		return herdrPane{}, err
	}
	if made.RootPane.TerminalID == "" {
		return herdrPane{}, errors.New("herdr made no terminal")
	}
	return made.RootPane, nil
}

// herdrButton is the panel's herdr button: it opens the terminals of
// herdr of the folder, to show them in the panel, and makes one.
func (a *app) herdrButton(c *ui.Context) {
	b := a.iconButton(c, iconHerdr, locales.HerdrMenu(), a.herdr.open)
	if b.Clicked() {
		a.toggleHerdrMenu()
	}
	ui.PopoverBase(c, b, &a.herdr.open, func(panel ui.Element) {
		a.herdrPanel(c, panel)
	})
}

// herdrPanel is the box of the herdr button: the terminals of the folder,
// and New Terminal under them.
func (a *app) herdrPanel(c *ui.Context, panel ui.Element) {
	p := a.pal
	m := &a.herdr
	a.floating(c, panel.Width(300).Padding(5).Gap(1).Radius(10), false).Label(locales.HerdrMenu())
	panel.Children(func() {
		ui.Row(c).Height(26).Padding(0, 7, 2, 7).Gap(6).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconHerdr).FontSize(12).TextColor(p.faint)
			ui.Text(c, strings.ToUpper(locales.HerdrMenu())).FontSize(10.5).FontWeight(600).LetterSpacing(0.7).
				TextColor(p.faint).SingleLine()
			ui.Spacer(c)
			if m.busy {
				ui.Spinner(c).Size(10, 10)
			}
		})
		switch {
		case m.busy && len(m.info.rows) == 0:
			a.herdrNote(c, iconHerdr, locales.HerdrLooking(), "")
		case m.info.err != "":
			a.herdrNote(c, iconX, locales.HerdrFailed(m.info.err), "")
		case !m.info.installed:
			a.herdrNote(c, iconHerdr, locales.HerdrMissing(), locales.HerdrMissingTip())
		case !m.info.running:
			a.herdrNote(c, iconHerdr, locales.HerdrNoServer(), locales.HerdrNoServerTip())
		case len(m.info.rows) == 0:
			a.herdrNote(c, iconHerdr, locales.HerdrNone(), locales.HerdrNoneTip())
		default:
			a.herdrRowList(c)
		}
		if !m.info.installed && !m.busy {
			return // nothing to make without herdr
		}
		ui.Row(c).Height(1).Background(p.border).Margin(4, 2)
		a.herdrNewRow(c)
	})
}

// herdrRowList is the terminals of the box.
func (a *app) herdrRowList(c *ui.Context) {
	p := a.pal
	ui.Scroll(c.Key("herdr-rows")).MaxHeight(320).Children(func() {
		ui.Column(c).Gap(1).Children(func() {
			for _, row := range a.herdr.info.rows {
				b := ui.ButtonBase(c.Key("herdr:"+row.pane.TerminalID)).Height(32).Padding(0, 8).Gap(9).Radius(6).
					Justify(ui.Start).AlignItems(ui.Center).Label(row.name)
				if b.Hovered() {
					b.Background(p.hover)
				}
				if b.Clicked() {
					a.post(func() { a.attachHerdr(row) })
				}
				b.Children(func() {
					ui.Icon(c, iconTerminal).FontSize(13).TextColor(p.muted)
					ui.Column(c).Gap(1).Grow(1).Shrink(1).MinWidth(0).Justify(ui.Center).Children(func() {
						ui.Text(c, row.name).FontSize(12.5).TextColor(p.text).SingleLine()
						if row.what != "" {
							ui.Text(c, row.what).FontSize(11).TextColor(p.faint).SingleLine()
						}
					})
				})
			}
		})
	})
}

// herdrNewRow is New Terminal, under the terminals: it makes one in
// herdr and shows it.
func (a *app) herdrNewRow(c *ui.Context) {
	p := a.pal
	b := ui.ButtonBase(c.Key("herdr:new")).Height(30).Padding(0, 8).Gap(9).Radius(6).Justify(ui.Start).AlignItems(ui.Center).
		Label(locales.CmdViewNewTerminal())
	if b.Hovered() {
		b.Background(p.hover)
	}
	if b.Clicked() {
		a.post(a.newHerdrTerminal)
	}
	b.Children(func() {
		ui.Icon(c, iconPlus).FontSize(14).TextColor(p.muted)
		ui.Text(c, locales.CmdViewNewTerminal()).FontSize(12.5).TextColor(p.text).SingleLine()
	})
}

// herdrNote says how herdr is, where its terminals would be.
func (a *app) herdrNote(c *ui.Context, icon *ui.SVG, title, tip string) {
	p := a.pal
	ui.Row(c).Padding(8, 8, 9, 8).Gap(9).AlignItems(ui.Start).Children(func() {
		ui.Icon(c, icon).FontSize(13).TextColor(p.muted).Margin(1, 0, 0, 0)
		ui.Column(c).Gap(2).Grow(1).Shrink(1).Children(func() {
			ui.Text(c, title).FontSize(12.5).TextColor(p.text)
			if tip != "" {
				ui.Text(c, tip).FontSize(11.5).TextColor(p.muted)
			}
		})
	})
}
