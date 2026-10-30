package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/borismilner/rig/internal/config"
	"github.com/borismilner/rig/internal/observe"
	"github.com/borismilner/rig/internal/paths"
	rigv1 "github.com/borismilner/rig/proto/rig/v1"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

const (
	debugBrowserDefaultProfile  = "boris.milner@gmail.com"
	debugChromeDefaultSource    = "~/.config/google-chrome"
	debugChromeReadyFor         = 15 * time.Second
	debugChromeProbeEvery       = 100 * time.Millisecond
	debugChromeMaxLocalState    = 16 << 20
	debugChromeMaxPreferences   = 64 << 20
	debugChromeFrameColor       = -2937041 // ARGB #FFD32F2F, high-contrast red.
	debugChromeFrameVariant     = 3        // Chrome's BrowserColorVariant::kVibrant.
	debugChromeFrameColorScheme = 1        // Chrome's BrowserColorScheme::kLight.
	debugChromeColorThemeID     = "user_color_theme_id"
)

var (
	errDebugChromeInvalid     = errors.New("invalid debug Chrome request")
	errDebugChromeUnavailable = errors.New("debug Chrome unavailable")
)

// debugBrowser provides one browser-neutral MCP start surface. Chrome is the
// first engine; each later engine can supply its protocol and endpoint here.
// It never opens the source data directory with remote debugging enabled.
type debugBrowser struct {
	settings *config.Resolver
}

type debugChromeInfo struct {
	Profile          string `json:"profile"`
	ProfileDirectory string `json:"profile_directory"`
	PID              int    `json:"pid"`
	StartedUnixNano  int64  `json:"started_unix_nano"`
	ProfileKey       string `json:"profile_key"`
	BrowserID        string `json:"browser_id"`
}

type chromeLocalState struct {
	Profile struct {
		InfoCache map[string]struct {
			UserName string `json:"user_name"`
		} `json:"info_cache"`
	} `json:"profile"`
}

type chromeVersion struct {
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type debugChromeEndpoint struct {
	devtoolsURL string
	websocket   string
	browserID   string
	port        int
}

func (d *Daemon) serveDebugBrowser(ctx context.Context, c *conn, f *rigv1.Frame) {
	var req verbsv1.DebugBrowserStartRequest
	if !unmarshalOr(c, f, "debug_browser.start", &req) {
		return
	}
	start := time.Now()
	resp, err := d.debugBrowser.start(ctx, req.GetBrowser(), req.GetProfile())
	if err != nil {
		d.auditDebugBrowser(c, start, req.GetBrowser(), req.GetProfile(), err, "")
		d.failDebugBrowser(c, f, err)
		return
	}
	resp.OutlineStatus = "unavailable"
	if d.capture != nil {
		if outlineErr := d.capture.SetDebugBrowserPID(ctx, int(resp.GetPid())); outlineErr == nil {
			resp.OutlineStatus = "available"
		}
	}
	d.auditDebugBrowser(c, start, req.GetBrowser(), req.GetProfile(), nil, resp.GetOutlineStatus())
	c.reply(f.GetStreamId(), resp)
}

func (s *debugBrowser) start(ctx context.Context, engine, requested string) (*verbsv1.DebugBrowserStartResponse, error) {
	browser := strings.ToLower(strings.TrimSpace(engine))
	if browser == "" {
		return nil, fmt.Errorf("%w: browser is required; use browser=chrome", errDebugChromeInvalid)
	}
	if browser != "chrome" {
		return nil, fmt.Errorf("%w: browser %q is not implemented yet; use browser=chrome", errDebugChromeUnavailable, browser)
	}
	profile := strings.TrimSpace(requested)
	if profile == "" {
		profile = s.setting("debug_browser.default_profile", debugBrowserDefaultProfile)
	}
	if profile == "" || strings.ContainsAny(profile, "\x00\r\n") {
		return nil, fmt.Errorf("%w: profile must be an email or one Chrome profile directory", errDebugChromeInvalid)
	}

	source, err := s.sourceDir()
	if err != nil {
		return nil, err
	}
	profileDir, err := findChromeProfile(source, profile)
	if err != nil {
		return nil, err
	}
	key := debugChromeKey(profile, profileDir)
	state, err := paths.StateDir()
	if err != nil {
		return nil, fmt.Errorf("%w: locating rig's private state directory: %v", errDebugChromeUnavailable, err)
	}
	root := filepath.Join(state, "debug-chrome")
	if err := ensurePrivateDir(root); err != nil {
		return nil, fmt.Errorf("creating the debug Chrome state directory: %w", err)
	}
	clone := filepath.Join(root, key)
	lock, err := lockDebugChrome(ctx, filepath.Join(root, key+".lock"))
	if err != nil {
		return nil, err
	}
	defer unlockDebugChrome(lock)

	if info, infoErr := readDebugChromeInfo(clone); infoErr == nil && info.ProfileKey == key && chromeProcessMatches(info.PID, clone) {
		if ep, ok := probeDebugChromeEndpoint(clone); ok && info.BrowserID != "" && info.BrowserID == ep.browserID {
			return debugBrowserResponse(browser, profile, "cdp", info.PID, ep, true), nil
		}
		return nil, fmt.Errorf("%w: the recorded Chrome process still owns this profile but its DevTools endpoint does not match", errDebugChromeUnavailable)
	}
	if pid, ok := findChromeProcessForDataDir(clone); ok {
		return nil, fmt.Errorf("%w: Chrome process %d still owns this profile without matching rig endpoint metadata", errDebugChromeUnavailable, pid)
	}
	if err := clearStaleChromeMetadata(clone); err != nil {
		return nil, err
	}
	if err := prepareChromeClone(source, profileDir, clone, key, profile); err != nil {
		return nil, err
	}

	binary, err := s.chromeBinary()
	if err != nil {
		return nil, err
	}
	args := []string{
		"--user-data-dir=" + clone,
		"--profile-directory=" + profileDir,
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=0",
		"--no-first-run",
		"--no-default-browser-check",
		"about:blank",
	}
	cmd := exec.Command(binary, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("%w: starting Chrome: %v", errDebugChromeUnavailable, err)
	}
	finished := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(finished)
	}()
	info := debugChromeInfo{
		Profile: profile, ProfileDirectory: profileDir, PID: cmd.Process.Pid,
		StartedUnixNano: time.Now().UnixNano(), ProfileKey: key,
	}
	if err := writeDebugChromeInfo(clone, info); err != nil {
		stopDebugChrome(cmd.Process.Pid, finished)
		return nil, fmt.Errorf("recording the debug Chrome process: %w", err)
	}

	deadline := time.NewTimer(debugChromeReadyFor)
	defer deadline.Stop()
	tick := time.NewTicker(debugChromeProbeEvery)
	defer tick.Stop()
	for {
		if ep, ok := probeDebugChromeEndpoint(clone); ok {
			if pid, found := findChromeProcessForDataDir(clone); found {
				info.PID = pid
			}
			info.BrowserID = ep.browserID
			if err := writeDebugChromeInfo(clone, info); err != nil {
				stopDebugChrome(cmd.Process.Pid, finished)
				return nil, fmt.Errorf("recording the debug Chrome process: %w", err)
			}
			return debugBrowserResponse(browser, profile, "cdp", info.PID, ep, false), nil
		}
		select {
		case <-ctx.Done():
			stopDebugChrome(cmd.Process.Pid, finished)
			return nil, fmt.Errorf("%w: waiting for Chrome's DevTools endpoint: %v", errDebugChromeUnavailable, ctx.Err())
		case <-finished:
			return nil, fmt.Errorf("%w: Chrome exited before its DevTools endpoint became ready", errDebugChromeUnavailable)
		case <-deadline.C:
			stopDebugChrome(cmd.Process.Pid, finished)
			return nil, fmt.Errorf("%w: Chrome did not publish a DevTools endpoint within %s", errDebugChromeUnavailable, debugChromeReadyFor)
		case <-tick.C:
		}
	}
}

func (s *debugBrowser) setting(key, fallback string) string {
	if s != nil && s.settings != nil {
		if value := strings.TrimSpace(s.settings.String(key)); value != "" {
			return value
		}
	}
	return fallback
}

func (s *debugBrowser) sourceDir() (string, error) {
	path := s.setting("debug_chrome.user_data_dir", debugChromeDefaultSource)
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%w: resolving the Chrome source directory: %v", errDebugChromeUnavailable, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("%w: resolving the Chrome source directory: %v", errDebugChromeInvalid, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", fmt.Errorf("%w: Chrome user data directory is unavailable", errDebugChromeUnavailable)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%w: Chrome user data directory must be a real directory", errDebugChromeInvalid)
	}
	return path, nil
}

func (s *debugBrowser) chromeBinary() (string, error) {
	binary := s.setting("debug_chrome.binary", "google-chrome")
	if !filepath.IsAbs(binary) && !strings.ContainsRune(binary, filepath.Separator) {
		found, err := exec.LookPath(binary)
		if err != nil {
			return "", fmt.Errorf("%w: Chrome executable %q is not on PATH", errDebugChromeUnavailable, binary)
		}
		return found, nil
	}
	if !filepath.IsAbs(binary) {
		return "", fmt.Errorf("%w: configured Chrome executable must be absolute", errDebugChromeInvalid)
	}
	info, err := os.Stat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0o111 == 0 {
		return "", fmt.Errorf("%w: configured Chrome executable is not an executable file", errDebugChromeUnavailable)
	}
	return binary, nil
}

func findChromeProfile(source, requested string) (string, error) {
	localStatePath := filepath.Join(source, "Local State")
	info, err := os.Lstat(localStatePath)
	if err != nil || !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: Chrome Local State is unavailable", errDebugChromeUnavailable)
	}
	f, err := openNoFollow(localStatePath)
	if err != nil {
		return "", fmt.Errorf("%w: reading Chrome Local State", errDebugChromeUnavailable)
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, debugChromeMaxLocalState+1))
	if err != nil || len(body) > debugChromeMaxLocalState {
		return "", fmt.Errorf("%w: Chrome Local State is unreadable or too large", errDebugChromeUnavailable)
	}
	var state chromeLocalState
	if err := json.Unmarshal(body, &state); err != nil {
		return "", fmt.Errorf("%w: Chrome Local State is not valid JSON", errDebugChromeUnavailable)
	}

	var matches []string
	for dir, profile := range state.Profile.InfoCache {
		if strings.EqualFold(strings.TrimSpace(profile.UserName), requested) {
			matches = append(matches, dir)
		}
	}
	if len(matches) == 0 && safeChromeProfileDir(requested) {
		if _, ok := state.Profile.InfoCache[requested]; ok {
			matches = append(matches, requested)
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("%w: no Chrome profile matches the requested account or directory", errDebugChromeInvalid)
	}
	if len(matches) != 1 {
		return "", fmt.Errorf("%w: more than one Chrome profile matches the requested account", errDebugChromeInvalid)
	}
	if !safeChromeProfileDir(matches[0]) {
		return "", fmt.Errorf("%w: Chrome profile directory is not a safe name", errDebugChromeInvalid)
	}
	profilePath := filepath.Join(source, matches[0])
	profileInfo, err := os.Lstat(profilePath)
	if err != nil || profileInfo.Mode()&os.ModeSymlink != 0 || !profileInfo.IsDir() {
		return "", fmt.Errorf("%w: selected Chrome profile directory is unavailable", errDebugChromeUnavailable)
	}
	return matches[0], nil
}

func safeChromeProfileDir(name string) bool {
	return name != "" && name != "." && name != ".." && filepath.Base(name) == name &&
		!strings.ContainsAny(name, "/\\\x00\r\n")
}

func debugChromeKey(profile, dir string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(profile) + "\x00" + dir))
	return hex.EncodeToString(sum[:])
}

func ensurePrivateDir(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("state path is not a real directory")
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	return nil
}

func clearStaleChromeMetadata(root string) error {
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking the debug Chrome profile directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("debug Chrome profile path is not a real directory")
	}
	for _, name := range []string{"DevToolsActivePort", ".rig-debug-chrome.json", ".rig-debug-chrome.tmp"} {
		path := filepath.Join(root, name)
		entry, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("checking stale Chrome metadata: %w", err)
		}
		if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() {
			return fmt.Errorf("debug Chrome metadata %q is not a regular file", name)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("removing stale Chrome metadata: %w", err)
		}
	}
	return nil
}

func prepareChromeClone(source, profileDir, clone, key, profile string) error {
	if info, err := os.Lstat(clone); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return errors.New("debug Chrome data path is not a real directory")
		}
		if marker, err := os.ReadFile(filepath.Join(clone, ".rig-profile-key")); err == nil && string(marker) == key {
			profileInfo, profileErr := os.Lstat(filepath.Join(clone, profileDir))
			if profileErr == nil && profileInfo.Mode()&os.ModeSymlink == 0 && profileInfo.IsDir() {
				return applyDebugChromeFrameTheme(clone, profileDir)
			}
		}
		if err := os.RemoveAll(clone); err != nil {
			return fmt.Errorf("removing an incomplete debug Chrome clone: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking the debug Chrome clone: %w", err)
	}
	root := filepath.Dir(clone)
	tmp, err := os.MkdirTemp(root, ".debug-chrome-clone-")
	if err != nil {
		return fmt.Errorf("creating a private profile clone: %w", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(tmp)
		}
	}()
	if err := copyChromeFile(filepath.Join(source, "Local State"), filepath.Join(tmp, "Local State")); err != nil {
		return err
	}
	if err := copyChromeTree(filepath.Join(source, profileDir), filepath.Join(tmp, profileDir)); err != nil {
		return err
	}
	if err := applyDebugChromeFrameTheme(tmp, profileDir); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, ".rig-profile-key"), []byte(key), 0o600); err != nil {
		return fmt.Errorf("writing the clone marker: %w", err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".rig-profile-name"), []byte(profile), 0o600); err != nil {
		return fmt.Errorf("writing the profile marker: %w", err)
	}
	if err := os.Rename(tmp, clone); err != nil {
		return fmt.Errorf("publishing the debug Chrome clone: %w", err)
	}
	ok = true
	return nil
}

func applyDebugChromeFrameTheme(clone, profileDir string) error {
	path := filepath.Join(clone, profileDir, "Preferences")
	prefs := map[string]any{}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return errors.New("Chrome profile Preferences must be a regular file")
		}
		f, err := openNoFollow(path)
		if err != nil {
			return fmt.Errorf("opening Chrome profile Preferences: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(f, debugChromeMaxPreferences+1))
		closeErr := f.Close()
		if err := errors.Join(readErr, closeErr); err != nil {
			return fmt.Errorf("reading Chrome profile Preferences: %w", err)
		}
		if len(body) > debugChromeMaxPreferences {
			return errors.New("Chrome profile Preferences exceeds the size limit")
		}
		if err := json.Unmarshal(body, &prefs); err != nil {
			return fmt.Errorf("Chrome profile Preferences is not valid JSON: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("checking Chrome profile Preferences: %w", err)
	}
	browser := jsonObject(prefs, "browser")
	theme := jsonObject(browser, "theme")
	// Chrome stores the active user color and palette under versioned profile
	// prefs. The vivid variant and light scheme keep the red marker visible.
	theme["user_color2"] = debugChromeFrameColor
	theme["color_variant2"] = debugChromeFrameVariant
	theme["color_scheme2"] = debugChromeFrameColorScheme

	extensions := jsonObject(prefs, "extensions")
	extensionTheme := jsonObject(extensions, "theme")
	extensionTheme["id"] = debugChromeColorThemeID
	delete(extensionTheme, "pack")

	body, err := json.Marshal(prefs)
	if err != nil {
		return fmt.Errorf("encoding Chrome profile Preferences: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".rig-debug-chrome-preferences-")
	if err != nil {
		return fmt.Errorf("creating Chrome profile Preferences: %w", err)
	}
	tmp := file.Name()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("setting Chrome profile Preferences permissions: %w", err)
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("writing Chrome profile Preferences: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("syncing Chrome profile Preferences: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("closing Chrome profile Preferences: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("publishing Chrome profile Preferences: %w", err)
	}
	return nil
}

func jsonObject(parent map[string]any, key string) map[string]any {
	if child, ok := parent[key].(map[string]any); ok {
		return child
	}
	child := map[string]any{}
	parent[key] = child
	return child
}

func copyChromeFile(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("Chrome profile source file is unavailable or not regular")
	}
	in, err := openNoFollow(source)
	if err != nil {
		return fmt.Errorf("opening Chrome profile source: %w", err)
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("creating Chrome profile copy: %w", err)
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if err := errors.Join(copyErr, closeErr); err != nil {
		return fmt.Errorf("copying Chrome profile file: %w", err)
	}
	return nil
}

func copyChromeTree(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("Chrome profile source must be a real directory")
	}
	if err := os.Mkdir(target, 0o700); err != nil {
		return err
	}
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == source {
			return nil
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() && chromeCacheDir(rel) {
			return filepath.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Chrome profile contains a symlink at %q", rel)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		to := filepath.Join(target, rel)
		if info.IsDir() {
			return os.Mkdir(to, 0o700)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("Chrome profile contains a non-regular file at %q", rel)
		}
		return copyChromeFile(path, to)
	})
}

func chromeCacheDir(rel string) bool {
	switch filepath.ToSlash(rel) {
	case "Service Worker/CacheStorage":
		return true
	}
	switch filepath.Base(rel) {
	case "Cache", "Code Cache", "GPUCache", "DawnGraphiteCache", "DawnWebGPUCache":
		return true
	default:
		return false
	}
}

func lockDebugChrome(ctx context.Context, path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening debug Chrome lock: %w", err)
	}
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = f.Close()
			return nil, fmt.Errorf("taking debug Chrome lock: %w", err)
		}
		select {
		case <-ctx.Done():
			_ = f.Close()
			return nil, fmt.Errorf("%w: waiting for another debug Chrome start: %v", errDebugChromeUnavailable, ctx.Err())
		case <-tick.C:
		}
	}
}

func unlockDebugChrome(f *os.File) {
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	_ = f.Close()
}

func probeDebugChromeEndpoint(root string) (debugChromeEndpoint, bool) {
	activePath := filepath.Join(root, "DevToolsActivePort")
	info, err := os.Lstat(activePath)
	if err != nil || !info.Mode().IsRegular() {
		return debugChromeEndpoint{}, false
	}
	f, err := openNoFollow(activePath)
	if err != nil {
		return debugChromeEndpoint{}, false
	}
	body, err := io.ReadAll(io.LimitReader(f, 1024))
	_ = f.Close()
	if err != nil {
		return debugChromeEndpoint{}, false
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) < 2 {
		return debugChromeEndpoint{}, false
	}
	activeBrowserID := strings.TrimPrefix(strings.TrimSpace(lines[1]), "/devtools/browser/")
	if activeBrowserID == "" || strings.Contains(activeBrowserID, "/") {
		return debugChromeEndpoint{}, false
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil || port < 1 || port > 65535 {
		return debugChromeEndpoint{}, false
	}
	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/json/version")
	if err != nil {
		return debugChromeEndpoint{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return debugChromeEndpoint{}, false
	}
	var version chromeVersion
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&version); err != nil {
		return debugChromeEndpoint{}, false
	}
	u, err := url.Parse(version.WebSocketDebuggerURL)
	if err != nil || u.Scheme != "ws" || (u.Hostname() != "localhost" && u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/devtools/browser/") {
		return debugChromeEndpoint{}, false
	}
	if u.Port() != "" && u.Port() != strconv.Itoa(port) {
		return debugChromeEndpoint{}, false
	}
	u.Host = net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	browserID := strings.TrimPrefix(u.Path, "/devtools/browser/")
	if browserID == "" || strings.Contains(browserID, "/") || browserID != activeBrowserID {
		return debugChromeEndpoint{}, false
	}
	return debugChromeEndpoint{
		devtoolsURL: "http://127.0.0.1:" + strconv.Itoa(port) + "/json/version",
		websocket:   u.String(), browserID: browserID, port: port,
	}, true
}

func chromeProcessMatches(pid int, userDataDir string) bool {
	if pid <= 0 {
		return false
	}
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return false
	}
	for _, arg := range strings.Split(string(cmdline), "\x00") {
		if arg == "--user-data-dir="+userDataDir {
			return true
		}
	}
	environ, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err == nil {
		for _, entry := range strings.Split(string(environ), "\x00") {
			if entry == "RIG_DEBUG_CHROME_DATA="+userDataDir {
				return true
			}
		}
	}
	return false
}

func findChromeProcessForDataDir(userDataDir string) (int, bool) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0, false
	}
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if chromeProcessMatches(pid, userDataDir) {
			return pid, true
		}
	}
	return 0, false
}

func readDebugChromeInfo(root string) (debugChromeInfo, error) {
	var info debugChromeInfo
	f, err := openNoFollow(filepath.Join(root, ".rig-debug-chrome.json"))
	if err != nil {
		return info, err
	}
	body, err := io.ReadAll(io.LimitReader(f, 4096))
	_ = f.Close()
	if err != nil {
		return info, err
	}
	err = json.Unmarshal(body, &info)
	return info, err
}

func openNoFollow(path string) (*os.File, error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), path), nil
}

func writeDebugChromeInfo(root string, info debugChromeInfo) error {
	body, err := json.Marshal(info)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(root, ".rig-debug-chrome-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, filepath.Join(root, ".rig-debug-chrome.json")); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func debugBrowserResponse(browser, profile, protocol string, pid int, ep debugChromeEndpoint, already bool) *verbsv1.DebugBrowserStartResponse {
	return &verbsv1.DebugBrowserStartResponse{
		Browser: browser, Protocol: protocol, Profile: profile, Pid: int32(pid), DevtoolsUrl: ep.devtoolsURL,
		WebsocketEndpoint: ep.websocket, AlreadyRunning: already,
	}
}

func stopDebugChrome(pid int, finished <-chan struct{}) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
}

func (d *Daemon) auditDebugBrowser(c *conn, start time.Time, browser, profile string, err error, outlineStatus string) {
	if d.logs == nil {
		return
	}
	caller := c.name()
	if caller == "" {
		caller = callerName(c.principal())
	}
	attrs := map[string]string{
		"op": "start", "browser": strings.ToLower(strings.TrimSpace(browser)),
		"caller": caller, "principal": c.principal().Kind.String(),
		"profile": profile, "ms": strconv.FormatInt(time.Since(start).Milliseconds(), 10),
	}
	if outlineStatus != "" {
		attrs["outline_status"] = outlineStatus
	}
	if _, seat, _, ok := d.provenance(c); ok {
		attrs["seat"] = seat
	}
	level, message := 0, "debug browser start"
	if err != nil {
		level, message = 4, "debug browser start refused"
		attrs["result"] = debugChromeResult(err)
	} else {
		attrs["result"] = "ok"
	}
	d.logs.Append(observe.Record{At: start.UnixNano(), Level: level, Client: "rigd", Message: message, Attrs: attrs})
}

func debugChromeResult(err error) string {
	switch {
	case errors.Is(err, errDebugChromeInvalid):
		return "invalid"
	case errors.Is(err, errDebugChromeUnavailable):
		return "unavailable"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	default:
		return "failed"
	}
}

func (d *Daemon) failDebugBrowser(c *conn, f *rigv1.Frame, err error) {
	status := &rigv1.Status{Message: "rig.debug_browser.start: " + err.Error()}
	switch {
	case errors.Is(err, errDebugChromeInvalid):
		status.Code = rigv1.Code_CODE_INVALID
	case errors.Is(err, errDebugChromeUnavailable), errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		status.Code = rigv1.Code_CODE_UNAVAILABLE
		status.Precondition = "Google Chrome and the configured profile are available"
		status.Fix = "check debug_chrome.binary, debug_chrome.user_data_dir and the profile selection in rig config"
	default:
		status.Code = rigv1.Code_CODE_INTERNAL
	}
	c.failStatus(f.GetStreamId(), status)
}
