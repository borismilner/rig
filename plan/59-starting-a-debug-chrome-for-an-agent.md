## 59. Starting a debug Chrome for an agent

**Approved by Boris, 2026-10-30.** An agent can start a visible browser with
one of Boris's signed-in profiles and exercise the full authority of that
profile. Chrome is first. The start API is browser-neutral so Firefox can be
the next browser capability without changing how callers select a browser or
receive its control endpoint. This slice implements Chrome only.

### Start and attach

- rig exposes `debug_browser.start` through MCP. `browser` is `chrome` in
  this slice. The optional `profile` selects an account email; omission uses
  `debug_browser.default_profile`, defaulting to
  `boris.milner@gmail.com`.
- `debug_chrome.user_data_dir` names the source Chrome data directory. The
  default is `~/.config/google-chrome`. rig resolves the requested email to
  its profile directory from Chrome's `Local State`. The caller may also
  select a profile by its directory name when it has no Google account.
- `debug_chrome.binary` selects Chrome; rig searches PATH for
  `google-chrome` when unset.
- On first use, rig copies `Local State` and the selected profile into a
  dedicated, persistent data directory under rig's user state. It excludes
  regenerable caches and preserves installed extensions and their profile
  data, including a password-manager extension such as Bitwarden. It never
  debugs the everyday data directory in place. Later starts reuse the clone,
  so changes made in debug Chrome persist.
- The dedicated Chrome clone has a distinctive red frame color. This changes
  only the clone and preserves the source profile theme and extensions.
- rig also asks the desktop shell to draw a clearly visible red outline around
  the exact controlled browser window. The outline follows that window when it
  moves, resizes or changes workspace, and is removed when it closes. Only a
  PID registered by `debug_browser.start` receives the outline. If the shell
  marker service is unavailable, browser control still works and the response
  reports the outline as unavailable. Register again on both a new start and
  an idempotent reuse so a shell-extension restart can recover the marker.
- rig starts Chrome visibly, without headless or automation-only flags, with
  remote debugging on a dynamically assigned port bound to loopback. It
  returns the selected browser and profile, process id, protocol name, HTTP
  discovery URL, and browser WebSocket endpoint. A second start for that
  browser/profile returns the live instance rather than opening another.
- The Chrome protocol is CDP. The agent connects directly to the returned
  endpoint and can issue any browser or page CDP command, including page
  navigation, DOM and JavaScript evaluation, input, downloads, cookies,
  network, browser contexts and extension interaction. rig does not filter
  CDP or ask for `rig hand` approval for browser actions. The user explicitly
  delegates the signed-in browser authority to the invoking agent.
- The agent may use credentials and autofill already available in the copied
  profile's extensions. rig does not extract vault secrets or weaken a
  password manager's own lock. The browser tool must not ask Boris to repeat
  information the selected profile can already supply.

### Configuration and caller boundary

- `debug_browser.default_profile`, `debug_chrome.user_data_dir`, and
  `debug_chrome.binary` are rig settings. The first two can be set in the
  user's rig configuration. An MCP caller may select only a profile present
  in the configured source; it cannot supply a filesystem path or executable.
- Profile copies and DevTools port metadata are private to the local user.
  The source profile remains untouched. Path resolution rejects symlinked
  profile roots and never joins a caller-supplied path.
- The DevTools listener is loopback-only. The endpoint grants any local
  process full access to the signed-in browser, its extensions, cookies,
  password-manager UI, and any action the user could take through that
  browser. rig logs caller, browser, profile and start result, never page
  URLs, cookies, credentials or CDP payloads.
- If Chrome is unavailable, the profile cannot be resolved, or a live clone
  has no working DevTools endpoint, the call returns a specific error and
  does not kill unrelated browser processes.

### Firefox is next

Firefox's remote-control protocol is WebDriver BiDi, not CDP. Add it as the
next browser capability through `debug_browser.start`, returning its protocol
and endpoint in the same response shape. Firefox's profile clone must retain
its installed extensions and signed-in state. It gets the same full agent
authority and password-manager behavior described above. Firefox is not part
of this Chrome implementation slice.

### Acceptance

- The MCP tool starts a new Chrome clone for an account-selected profile and
  returns an endpoint that answers `/json/version` on `127.0.0.1`.
- The response identifies Chrome, CDP, the profile, pid and endpoint. A second
  start is idempotent and returns the same live browser endpoint.
- A CDP client can create a tab, navigate, inspect its DOM, evaluate script,
  send input, access available extension targets, and close the tab. These
  calls affect only the debug clone and trigger no rig hand prompts.
- The copied profile retains installed extensions and their profile data;
  the controlled frame is red, and a separate outline tracks the exact window
  in the shell e2e test; the source theme is untouched; default account is
  `boris.milner@gmail.com`; unknown profile/path inputs are refused.
- Tests use a fake Chrome executable and temporary profile tree. A live
  demonstration uses a harmless local page, never submits a form, and checks
  the captured debug-Chrome window.
