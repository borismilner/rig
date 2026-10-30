package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/borismilner/rig/internal/config"
	"github.com/borismilner/rig/internal/kernel"
	"github.com/borismilner/rig/proto/rig/v1/verbsv1"
)

const debugChromeTestProfile = "boris.milner@gmail.com"

func TestDebugChromeFakeServer(t *testing.T) {
	data := os.Getenv("RIG_DEBUG_CHROME_DATA")
	if data == "" {
		return
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	active := fmt.Sprintf("%d\n/devtools/browser/fake-browser\n", port)
	if err := os.WriteFile(filepath.Join(data, "DevToolsActivePort"), []byte(active), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Unsetenv("RIG_DEBUG_CHROME_DATA"); err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"webSocketDebuggerUrl": fmt.Sprintf("ws://localhost:%d/devtools/browser/fake-browser", port),
		})
	})}
	go func() { _ = server.Serve(listener) }()
	select {}
}

func TestDebugBrowserStartsAndReusesChromeClone(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	source := makeChromeSource(t)
	binary := fakeChromeBinary(t)
	settings := debugChromeSettings(t, source, binary)
	manager := &debugBrowser{settings: settings}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	started, err := manager.start(ctx, "chrome", "")
	if err != nil {
		t.Fatalf("starting Chrome: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-int(started.GetPid()), syscall.SIGKILL) })
	if started.GetBrowser() != "chrome" || started.GetProtocol() != "cdp" || started.GetProfile() != debugChromeTestProfile {
		t.Fatalf("start result did not identify browser, protocol and profile: %+v", started)
	}
	if !strings.HasPrefix(started.GetDevtoolsUrl(), "http://127.0.0.1:") || !strings.HasPrefix(started.GetWebsocketEndpoint(), "ws://127.0.0.1:") {
		t.Fatalf("endpoints are not loopback-only: %+v", started)
	}
	clone := filepath.Join(state, "rig", "debug-chrome", debugChromeKey(debugChromeTestProfile, "Profile 2"))
	info, err := readDebugChromeInfo(clone)
	if err != nil {
		t.Fatal(err)
	}
	if info.PID != int(started.GetPid()) || info.ProcessStartTicks == "" || info.DevtoolsPort < 1 || info.BrowserID == "" {
		t.Fatalf("live Chrome process identity was not persisted: %+v", info)
	}
	cmdline, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(info.PID), "cmdline"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(cmdline, []byte("--user-data-dir=")) || bytes.Contains(cmdline, []byte("--remote-debugging-port=")) {
		t.Fatal("fake Chrome retained its profile or DevTools launch arguments")
	}
	response, err := http.Get(started.GetDevtoolsUrl())
	if err != nil {
		t.Fatalf("DevTools discovery endpoint did not answer: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("DevTools discovery status %d", response.StatusCode)
	}

	for _, rel := range []string{
		"Profile 2/Extensions/fake-extension/manifest.json",
		"Profile 2/Local Extension Settings/vault/state.json",
		"Profile 2/Extension State/extension-settings.json",
	} {
		if _, err := os.Stat(filepath.Join(clone, rel)); err != nil {
			t.Errorf("profile clone lost extension data %q: %v", rel, err)
		}
	}
	sourcePreferences, err := os.ReadFile(filepath.Join(source, "Profile 2", "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	clonePreferences, err := os.ReadFile(filepath.Join(clone, "Profile 2", "Preferences"))
	if err != nil {
		t.Fatal(err)
	}
	var sourcePrefs, clonePrefs map[string]any
	if err := json.Unmarshal(sourcePreferences, &sourcePrefs); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(clonePreferences, &clonePrefs); err != nil {
		t.Fatal(err)
	}
	cloneBrowser := clonePrefs["browser"].(map[string]any)
	cloneTheme := cloneBrowser["theme"].(map[string]any)
	if cloneTheme["user_color2"] != float64(debugChromeFrameColor) ||
		cloneTheme["color_variant2"] != float64(debugChromeFrameVariant) ||
		cloneTheme["color_scheme2"] != float64(debugChromeFrameColorScheme) {
		t.Fatalf("clone does not carry the high-contrast frame color: %v", cloneTheme)
	}
	cloneExtensions := clonePrefs["extensions"].(map[string]any)
	cloneExtensionTheme := cloneExtensions["theme"].(map[string]any)
	if cloneExtensionTheme["id"] != debugChromeColorThemeID {
		t.Fatalf("clone did not select its identifying color theme: %v", cloneExtensionTheme)
	}
	if _, ok := cloneExtensionTheme["pack"]; ok {
		t.Fatalf("clone retained the previous theme pack: %v", cloneExtensionTheme)
	}
	sourceExtensions := sourcePrefs["extensions"].(map[string]any)
	if !reflect.DeepEqual(cloneExtensions["settings"], sourceExtensions["settings"]) {
		t.Fatal("applying the frame marker changed cloned extension settings")
	}
	if got, err := os.ReadFile(filepath.Join(source, "Profile 2", "Preferences")); err != nil || !reflect.DeepEqual(got, sourcePreferences) {
		t.Fatal("applying the frame marker changed source profile Preferences")
	}
	if _, err := os.Stat(filepath.Join(clone, "Profile 2", "Cache", "discard-me")); !os.IsNotExist(err) {
		t.Errorf("profile clone kept a regenerable cache: %v", err)
	}
	if err := os.Remove(filepath.Join(clone, "DevToolsActivePort")); err != nil {
		t.Fatal(err)
	}

	reused, err := manager.start(ctx, "chrome", debugChromeTestProfile)
	if err != nil {
		t.Fatalf("reusing Chrome: %v", err)
	}
	if !reused.GetAlreadyRunning() || reused.GetWebsocketEndpoint() != started.GetWebsocketEndpoint() || reused.GetPid() != started.GetPid() {
		t.Fatalf("second start did not reuse the live browser: first=%+v second=%+v", started, reused)
	}
	afterReuse, err := readDebugChromeInfo(clone)
	if err != nil {
		t.Fatal(err)
	}
	if afterReuse.PID != info.PID || afterReuse.ProcessStartTicks != info.ProcessStartTicks ||
		afterReuse.DevtoolsPort != info.DevtoolsPort || afterReuse.BrowserID != info.BrowserID {
		t.Fatalf("reuse changed the live process identity after ActivePort disappeared: before=%+v after=%+v", info, afterReuse)
	}
}

func TestDebugBrowserRejectsUnsupportedBrowserAndUnknownProfile(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	source := makeChromeSource(t)
	manager := &debugBrowser{settings: debugChromeSettings(t, source, fakeChromeBinary(t))}

	if _, err := manager.start(context.Background(), "firefox", ""); err == nil || !strings.Contains(err.Error(), "not implemented yet") {
		t.Fatalf("Firefox should be named as the next unsupported browser: %v", err)
	}
	if _, err := manager.start(context.Background(), "chrome", "missing@example.com"); err == nil || !strings.Contains(err.Error(), "no Chrome profile matches") {
		t.Fatalf("unknown profile was not refused: %v", err)
	}
	if _, err := os.Stat(filepath.Join(state, "rig", "debug-chrome")); !os.IsNotExist(err) {
		t.Fatalf("a refusal created debug browser state: %v", err)
	}
}

func TestDebugBrowserDoesNotReuseAnotherChromeOnAStalePort(t *testing.T) {
	state := t.TempDir()
	t.Setenv("XDG_STATE_HOME", state)
	source := makeChromeSource(t)
	binary := fakeChromeBinary(t)
	settings := debugChromeSettings(t, source, binary)
	profileDir := "Profile 2"
	key := debugChromeKey(debugChromeTestProfile, profileDir)
	clone := filepath.Join(state, "rig", "debug-chrome", key)
	if err := os.MkdirAll(clone, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(clone, ".rig-profile-key"), []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/json/version" {
			http.NotFound(w, r)
			return
		}
		_, _ = fmt.Fprintf(w, `{"webSocketDebuggerUrl":"ws://127.0.0.1:%d/devtools/browser/unrelated"}`, port)
	})}
	go func() { _ = server.Serve(listener) }()
	defer server.Close()
	if err := os.WriteFile(filepath.Join(clone, "DevToolsActivePort"), []byte(fmt.Sprintf("%d\n/devtools/browser/stale\n", port)), 0o600); err != nil {
		t.Fatal(err)
	}
	info := debugChromeInfo{PID: os.Getpid(), ProfileKey: key, BrowserID: "unrelated"}
	if err := writeDebugChromeInfo(clone, info); err != nil {
		t.Fatal(err)
	}
	if _, ok := probeDebugChromeEndpoint(clone); ok {
		t.Fatal("DevTools probe accepted a WebSocket id that differed from DevToolsActivePort")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	resp, err := (&debugBrowser{settings: settings}).start(ctx, "chrome", debugChromeTestProfile)
	if err != nil {
		t.Fatalf("starting after stale port metadata: %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(-int(resp.GetPid()), syscall.SIGKILL) })
	if resp.GetAlreadyRunning() || strings.Contains(resp.GetWebsocketEndpoint(), "unrelated") {
		t.Fatalf("reused an unrelated browser from stale metadata: %+v", resp)
	}
}

func TestDebugBrowserRejectsSymlinkedSourceRoot(t *testing.T) {
	base := t.TempDir()
	source := makeChromeSource(t)
	link := filepath.Join(base, "profile-root")
	if err := os.Symlink(source, link); err != nil {
		t.Fatal(err)
	}
	settings := debugChromeSettings(t, link, fakeChromeBinary(t))
	if _, err := (&debugBrowser{settings: settings}).sourceDir(); err == nil || !strings.Contains(err.Error(), "real directory") {
		t.Fatalf("symlinked profile root was not refused: %v", err)
	}
}

func TestDebugBrowserStartIsDeclaredAndBridged(t *testing.T) {
	var found bool
	for _, command := range selfDeclaration().Commands {
		if command.ID != "debug_browser.start" {
			continue
		}
		found = true
		if command.Effects != kernel.EffectsWritesFiles || command.Idempotent != kernel.Yes || command.Duration != kernel.DurationSeconds {
			t.Fatalf("unexpected browser start declaration: %+v", command)
		}
		break
	}
	if !found {
		t.Fatal("debug_browser.start is missing from rig's declaration")
	}
	bridgedVerb, ok := bridged["debug_browser.start"]
	if !ok {
		t.Fatal("debug_browser.start is missing from the MCP bridge")
	}
	if _, ok := bridgedVerb.req().(*verbsv1.DebugBrowserStartRequest); !ok {
		t.Fatalf("request type is %T", bridgedVerb.req())
	}
	if _, ok := bridgedVerb.resp().(*verbsv1.DebugBrowserStartResponse); !ok {
		t.Fatalf("response type is %T", bridgedVerb.resp())
	}
	var input map[string]any
	if err := json.Unmarshal(verbInput("debug_browser.start", bridgedVerb.req().ProtoReflect().Descriptor()), &input); err != nil {
		t.Fatal(err)
	}
	properties, _ := input["properties"].(map[string]any)
	if properties["browser"] == nil || properties["profile"] == nil {
		t.Fatalf("MCP input schema omitted browser or profile: %v", input)
	}
}

func TestDebugBrowserAuditUsesResolvedDefaultProfile(t *testing.T) {
	resp := &verbsv1.DebugBrowserStartResponse{Profile: debugChromeTestProfile}
	if got := debugBrowserAuditProfile("", resp); got != debugChromeTestProfile {
		t.Fatalf("audit profile = %q, want resolved default %q", got, debugChromeTestProfile)
	}
}

func makeChromeSource(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "google-chrome")
	profile := filepath.Join(root, "Profile 2")
	for name, body := range map[string]string{
		"Local State":           `{"profile":{"info_cache":{"Profile 2":{"user_name":"` + debugChromeTestProfile + `"}}}}`,
		"Profile 2/Preferences": `{"browser":{"theme":{"user_color":123,"user_color2":456,"color_scheme":1}},"extensions":{"theme":{"id":"existing-theme","pack":"Extensions/theme/1.0.0"},"settings":{"password-manager":{"state":1,"preferences":{"fixture":"retain"}},"another-extension":{"state":1}}}}`,
		"Profile 2/Extensions/fake-extension/manifest.json":   `{"name":"fixture"}`,
		"Profile 2/Local Extension Settings/vault/state.json": "encrypted-state-fixture",
		"Profile 2/Extension State/extension-settings.json":   "extension-data-fixture",
		"Profile 2/Cache/discard-me":                          "cache",
	} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(profile, 0o700); err != nil {
		t.Fatal(err)
	}
	return root
}

func fakeChromeBinary(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-google-chrome")
	script := "#!/bin/sh\nset -eu\ndata=''\nfor arg in \"$@\"; do\n  case \"$arg\" in --user-data-dir=*) data=${arg#--user-data-dir=};; esac\ndone\nexport RIG_DEBUG_CHROME_DATA=\"$data\"\nexec \"$RIG_TEST_BINARY\" -test.run=^TestDebugChromeFakeServer$\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RIG_TEST_BINARY", os.Args[0])
	return path
}

func debugChromeSettings(t *testing.T, source, binary string) *config.Resolver {
	t.Helper()
	schema, err := config.RigSchema()
	if err != nil {
		t.Fatal(err)
	}
	return config.Load(schema, config.Sources{Flags: map[string]string{
		"debug_chrome.user_data_dir": source,
		"debug_chrome.binary":        binary,
	}})
}
