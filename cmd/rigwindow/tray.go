package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"fyne.io/systray"

	"github.com/borismilner/rig/client"
	"github.com/borismilner/rig/proto/rig/v1/registryv1"
)

// design/tray is the source (section 11); cmd/rigwindow/icons is where the
// Makefile's build-rigwindow copies it, because go:embed cannot reach above
// its own package. Same arrangement as dist above it in this package.
//
//go:embed all:icons
var trayIcons embed.FS

// trayRefresh polls rigd for which estate it is rather than being told once,
// because the window survives a daemon restart (section 11) and a tray that
// only asks at launch would freeze on whatever it first saw.
const trayRefresh = 5 * time.Second

// The menu, held package-level because the poll loop retitles it on every
// tick and systray has no way to read an item back.
//
// BORIS, 2026-09-16: "Clicking on the icon reveals the different options and
// it also shows the version and whether it's prod or dev." So the first two
// rows are FACTS AND NOT COMMANDS - disabled, because a menu entry that looks
// clickable and does nothing is worse than a label.
var (
	menuVersion *systray.MenuItem
	menuEstate  *systray.MenuItem
	menuWindow  *systray.MenuItem

	// menuDetail is the third fact row and it is EMPTY while the daemon
	// answers. Boris, 2026-09-17, on the down state: "it can also indicate it
	// with a red dot and details when clicked."
	//
	// ⛔ A RED DOT WITHOUT THIS ROW SAYS "SOMETHING IS WRONG" AND STOPS.
	// The two rows above can only say `no daemon answering`, which tells him
	// it is down and not since when, not which estate it was, and not what to
	// run. The badge is the alarm; this is the answer to it.
	menuDetail *systray.MenuItem

	// detachedSince is when the daemon last stopped answering, so the detail
	// row can say HOW LONG. Zero while it is answering.
	//
	// ⛔ A DURATION IS WHAT SEPARATES A RESTART FROM AN OUTAGE. `make
	// install` stops rigd and starts it again, and the tray notices: eight
	// seconds detached is a deploy, forty minutes is something he has to look
	// at. Without the clock both render identically and he learns to ignore
	// the badge, which is the worst outcome for an alarm.
	detachedSince time.Time

	// lastIcon is the icon last set from a NAMED estate, so the down badge can
	// be that estate's own rather than a generic one.
	lastIcon string
)

// fyne.io/systray, not Wails' own application.SystemTray: the Wails beta.19
// implementation exports the StatusNotifierItem correctly (properties answer
// over D-Bus, confirmed with dbus-send) but its RegisterStatusNotifierItem
// call never lands in org.kde.StatusNotifierWatcher's own
// RegisteredStatusNotifierItems list, so nothing ever draws it - live-checked
// on this machine's GNOME session, no error logged either side. AgentBox
// registers fine with fyne.io/systray in the same session, so this package
// uses that instead of chasing the beta bug.
// ⛔ IT TAKES NO *application.App ANY MORE. The only thing it ever used the
// app for was app.Quit() behind the tray's quit row, and that row is gone
// because a tray that can remove itself is not "always available". Keeping the
// parameter would leave the capability one line from returning.
func runTraySupervisor(sup *supervisor) {
	systray.Run(func() {
		systray.SetTooltip("rig")

		// Pre-daemon text, so the menu is never blank before the first poll
		// answers. The window's OWN build stamp is the honest answer here:
		// nothing has told us the daemon's yet.
		menuVersion = systray.AddMenuItem("rig "+version+" (window)", "the version this window was built at")
		menuVersion.Disable()
		menuEstate = systray.AddMenuItem("looking for a daemon", "which estate this tray is attached to")
		menuEstate.Disable()
		menuDetail = systray.AddMenuItem("", "what to do when the daemon is not answering")
		menuDetail.Disable()
		menuDetail.Hide()

		systray.AddSeparator()
		menuWindow = systray.AddMenuItem(windowTitle(false), "Open or close the rig window")
		systray.AddSeparator()

		// ⛔ THERE IS NO QUIT ROW, AND ITS ABSENCE IS THE REQUIREMENT.
		// Boris, 2026-09-17: "Rig system-tray icon should always be
		// available." A row that removes the icon contradicts that in one
		// click, and it did: `Quit rig window` called app.Quit(), which ends
		// the process the tray lives in, and the tray was gone for 2h42m
		// before he asked whether it was there at all.
		//
		// ⛔ THE ROW WAS ALSO MIS-NAMED, WHICH IS WHY IT COST HIM THE ICON.
		// It said WINDOW and its tooltip said "Close the window and its tray
		// icon", so the truth was in the tooltip nobody hovers. Closing a
		// window is an ordinary act; ending the one thing whose job is to be
		// visible is not, and the menu offered them as the same gesture.
		//
		// Stopping it is still possible and is now where it belongs: the unit
		// that starts it. The row below says so rather than leaving him to
		// find out that nothing in the menu stops it.
		stop := systray.AddMenuItem("systemctl --user stop rigwindow.service",
			"the tray is always on by design; this is how to stop it")
		stop.Disable()

		// Left-click still toggles, so the gesture that worked before this
		// menu existed keeps working. The menu is an addition, not a
		// replacement: section 11 says the tray is an access point, and
		// taking away the one-click toggle to add options would trade one
		// for the other.
		systray.SetOnTapped(sup.toggle)

		// One receiver, because there is one clickable row. It stays a
		// goroutine with a loop rather than collapsing to a single receive:
		// ClickedCh fires on EVERY click, and a one-shot receive would make
		// the menu work once.
		go func() {
			for range menuWindow.ClickedCh {
				sup.toggle()
			}
		}()

		go pollEstate(sup)
	}, nil)
}

// windowTitle is what the window row says, and it says what the click will
// DO: with a window process alive the click closes it, otherwise it opens
// one. "Hide" would be a lie now that a close ends the process.
func windowTitle(open bool) string {
	if open {
		return "Close rig"
	}
	return "Show rig"
}

// retitleWindowItem keeps the entry describing what clicking it will DO, the
// way AgentBox's own tray does. Called from the supervisor on every change of
// the window process and from the poll loop so it self-corrects.
func retitleWindowItem(sup *supervisor) {
	if menuWindow == nil {
		return
	}
	menuWindow.SetTitle(windowTitle(sup.open()))
}

// pollEstate keeps the icon AND the two fact rows honest for the life of the
// process. Section 11 is explicit that an UNNAMED estate gets no tray at all -
// every test and reproduction recipe starts one, and a third icon appearing
// during `make ci` is the failure this is meant to prevent - so this only
// calls SetIcon once a named estate answers, and quits the tray if that ever
// reverts.
//
// Detached (rigd unreachable) is different: the tray is its own process and
// stays up, on its last-known icon. A dedicated detached glyph is
// one of the three dimensions section 11 still owes and is not decided here.
// The TEXT does not stay on its last-known value, though, and that asymmetry
// is deliberate: a stale icon is ambiguous, a stale VERSION is a lie, so the
// rows say the daemon is gone while the icon holds.
func pollEstate(sup *supervisor) {
	for {
		est, connected := estateSnapshot()
		switch {
		case connected && est.GetRole() == registryv1.EstateRole_ESTATE_ROLE_PRODUCTION:
			setTrayIcon("production.png", "rig - production")
			setFacts(est)
		case connected && est.GetRole() == registryv1.EstateRole_ESTATE_ROLE_DEVELOPMENT:
			setTrayIcon("development.png", "rig - development")
			setFacts(est)
		case connected:
			// Unnamed or unspecified. ⛔ THIS USED TO QUIT THE TRAY AND NOW
			// LABELS IT, BECAUSE "ALWAYS AVAILABLE" OUTRANKS THE RULE IT WAS
			// SERVING. Boris, 2026-09-17: "Rig system-tray icon should always
			// be available."
			//
			// ⛔ THE OLD RULE'S CONCERN WAS REAL AND IS NOT BEING DISMISSED:
			// every test and reproduction recipe starts a daemon, and an icon
			// per test run is worse than no icon. But an unnamed estate is a
			// reason to SAY "unnamed" on the icon, not a reason to have none -
			// the same argument §11 already makes about the detached state,
			// one level up. And the concern does not reach here anyway:
			// `make ci` starts daemons, never this window, which needs a
			// graphical session it does not have.
			//
			// The icon is left on its last-known glyph deliberately. There is
			// no unnamed artwork and inventing one to cover a branch rigd
			// refuses to produce - it validates the estate vocabulary by name
			// - would be drawing for a state nobody can reach today.
			setUnnamed()
		default:
			// Detached: BADGE the icon, and say how long in the text.
			//
			// ⛔ THIS USED TO KEEP THE ICON UNCHANGED, and the comment above
			// recorded the missing glyph as one of the three dimensions
			// section 11 still owed. Boris ruled it 2026-09-17: a red dot.
			// ⛔ THE BADGE GOES ON THE ESTATE'S OWN GLYPH rather than on a
			// shared down icon, so the tray does not answer "rig is not
			// running" by forgetting which estate it was - that is the fact
			// the tray exists to carry.
			setTrayIcon(downIcon(lastIcon), "rig - no daemon answering")
			setDetached()
		}
		retitleWindowItem(sup)
		time.Sleep(trayRefresh)
	}
}

// setFacts writes the two rows Boris asked for. The version served is the
// DAEMON's, not this window's, because the daemon is the thing actually
// running: a window left open across an upgrade would otherwise report the
// version it was built at while talking to a newer estate.
func setFacts(est *registryv1.EstateResponse) {
	if menuVersion == nil || menuEstate == nil {
		return
	}
	v := est.GetDaemonVersion()
	if v == "" {
		// An empty string on the wire is indistinguishable from unserved, so
		// it is NOT rendered as a version. Same reasoning as decision 6.
		v = "unknown"
	}
	menuVersion.SetTitle("rig " + v)
	menuVersion.SetTooltip("the version of the daemon this tray is attached to")

	name := est.GetName()
	if name == "" {
		// Unnamed reaches here only before the branch above quits the tray.
		name = "unnamed"
	}
	menuEstate.SetTitle("estate: " + name)
	menuEstate.SetTooltip("which estate this tray is attached to")

	// ⛔ THE RECOVERY IS AS LOAD-BEARING AS THE ALARM. A detail row left on
	// screen after the daemon came back is a stale alarm, and one stale alarm
	// is all it takes for him to stop reading the row. The clock is reset with
	// it so the next outage is measured from ITS start.
	detachedSince = time.Time{}
	if menuDetail != nil {
		menuDetail.SetTitle("")
		menuDetail.Hide()
	}
}

// setDetached says the daemon is gone rather than leaving the last version on
// screen. It falls back to this window's own build stamp and SAYS it is the
// window's, so the row is never a claim about a daemon that is not answering.
func setDetached() {
	if menuVersion == nil || menuEstate == nil {
		return
	}
	menuVersion.SetTitle("rig " + version + " (window)")
	menuEstate.SetTitle("no daemon answering")

	if detachedSince.IsZero() {
		detachedSince = time.Now()
	}
	if menuDetail == nil {
		return
	}
	// ⛔ THE ROW NAMES THE COMMAND. "The daemon is down" is a fact he
	// already has from the badge; what he does not have at that moment is the
	// one line that fixes it, and the window cannot run it for him - starting
	// a daemon is a decision, and section 28 keeps "replace what is deployed"
	// and "decide to deploy" apart deliberately.
	menuDetail.SetTitle(fmt.Sprintf("down for %s - systemctl --user start rigd.service",
		roundedSince(detachedSince)))
	menuDetail.SetTooltip("the daemon has not answered since " +
		detachedSince.Format("15:04:05"))
	menuDetail.Show()
}

// roundedSince is how long ago something was, at a grain a person reads.
//
// ⛔ SECONDS ARE THE POINT BELOW A MINUTE AND NOISE ABOVE IT. `make install`
// stops rigd and starts it again inside a few seconds, so "down for 4s" is
// what tells him he is watching a deploy rather than an outage; at forty
// minutes the seconds are a number nobody reads and a row that changes every
// five seconds forever.
func roundedSince(t time.Time) time.Duration {
	d := time.Since(t)
	if d < time.Minute {
		return d.Round(time.Second)
	}
	return d.Round(time.Minute)
}

// downIcon is the badged variant of an estate icon, or the plain badged
// production glyph when no estate has ever answered.
//
// ⛔ THE FALLBACK IS A CHOICE AND NOT A DEFAULT. A window started while rigd
// is already down has no last-known estate, and showing nothing would leave
// him with the thing he complained about: no visual way of knowing. Production
// is the estate his tray runs, so its badged glyph is the honest guess - and
// the menu's detail row says the estate is unknown rather than asserting one.
func downIcon(last string) string {
	base := "production"
	if last != "" {
		base = strings.TrimSuffix(strings.TrimSuffix(last, ".png"), "-down")
	}
	return base + "-down.png"
}

// setUnnamed says the estate has no name rather than taking the icon away.
//
// The version row still carries whatever the daemon reported: it IS answering,
// so its version is a fact, unlike the detached case where it is a memory.
func setUnnamed() {
	if menuEstate == nil {
		return
	}
	menuEstate.SetTitle("estate: unnamed")
	menuEstate.SetTooltip("the daemon is answering and reports no estate name")
	detachedSince = time.Time{}
	if menuDetail != nil {
		menuDetail.SetTitle("")
		menuDetail.Hide()
	}
}

func setTrayIcon(icon, tooltip string) {
	if !strings.HasSuffix(icon, "-down.png") {
		lastIcon = icon
	}
	if b := iconBytes(icon); b != nil {
		systray.SetIcon(b)
	}
	systray.SetTooltip(tooltip)
}

func iconBytes(name string) []byte {
	b, err := fs.ReadFile(trayIcons, "icons/"+name)
	if err != nil {
		return nil
	}
	return b
}

// estateSnapshot reuses RigService's own dial-per-call shape (see readDeadline
// in service.go) rather than holding a connection, for the same reason: no
// socket to go stale, so the next tick reconnects on its own.
//
// It returns the whole response rather than just the role, because the menu
// needs the name and the daemon version too and a second call would be a
// second dial answering about a possibly different instant.
func estateSnapshot() (*registryv1.EstateResponse, bool) {
	c, err := client.Connect()
	if err != nil {
		return nil, false
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), readDeadline)
	defer cancel()

	resp := &registryv1.EstateResponse{}
	if err := c.Call(ctx, "rig.estate", &registryv1.EstateRequest{}, resp); err != nil {
		return nil, false
	}
	return resp, true
}
