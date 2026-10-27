// rig capture (plan section 58): org.rig.Capture on the session bus, API 1.
//
// Inside the compositor the pixels are already there, so a window that is
// covered, minimized or on another workspace can be painted without asking
// the person and without it being in front. Nothing here decides who may
// call: rigd owns that and logs every capture. The bus is the user's own.

import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Meta from 'gi://Meta';
import Shell from 'gi://Shell';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import {PACKAGE_VERSION} from 'resource:///org/gnome/shell/misc/config.js';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const API = 1;
const ERR = 'org.rig.Capture.Error.';
const MAX_SIDE = 32768;
const MAX_PIXELS = 134217728;
const MAX_INFLIGHT = 4;
const REPLY_TIMEOUT_S = 20;

const XML = `<node><interface name="org.rig.Capture">
<method name="Version"><arg type="u" direction="out" name="api"/><arg type="s" direction="out" name="shell"/></method>
<method name="ListWindows"><arg type="s" direction="out" name="json"/></method>
<method name="ListOutputs"><arg type="s" direction="out" name="json"/></method>
<method name="CaptureWindow"><arg type="t" direction="in" name="id"/><arg type="s" direction="in" name="path"/><arg type="s" direction="out" name="json"/></method>
<method name="CaptureArea"><arg type="i" direction="in" name="x"/><arg type="i" direction="in" name="y"/><arg type="i" direction="in" name="w"/><arg type="i" direction="in" name="h"/><arg type="b" direction="in" name="cursor"/><arg type="s" direction="in" name="path"/><arg type="s" direction="out" name="json"/></method>
</interface></node>`;

const SHOWN_TYPES = new Set([
    Meta.WindowType.NORMAL, Meta.WindowType.DIALOG,
    Meta.WindowType.MODAL_DIALOG, Meta.WindowType.UTILITY,
]);

class Refusal extends Error {
    constructor(name, message) {
        super(message);
        this.dbusName = ERR + name;
    }
}

// windows lists the top-level windows, bottom of the stack first.
function windows() {
    return global.get_window_actors()
        .map(a => a.meta_window)
        .filter(w => w && SHOWN_TYPES.has(w.get_window_type()));
}

function windowInfo(w) {
    const r = w.get_frame_rect();
    const actor = w.get_compositor_private();
    const active = global.workspace_manager.get_active_workspace();
    const onAll = w.is_on_all_workspaces();
    return {
        id: Number(w.get_id()),
        title: w.get_title() ?? '',
        app: w.get_wm_class() ?? '',
        pid: w.get_pid() ?? 0,
        workspace: onAll ? -1 : (w.get_workspace()?.index() ?? -1),
        minimized: w.minimized,
        focused: w.has_focus(),
        visible: !w.minimized && !!actor?.visible && (onAll || w.located_on_workspace(active)),
        fullscreen: w.is_fullscreen(),
        modal: w.get_window_type() === Meta.WindowType.MODAL_DIALOG,
        x: r.x, y: r.y, w: r.width, h: r.height,
    };
}

function outputs() {
    const d = global.display;
    return Main.layoutManager.monitors.map((m, i) => {
        let name = '';
        try { name = d.get_monitor_connector?.(i) ?? ''; } catch (_e) { /* older API */ }
        return {
            index: i, name: name || `monitor-${i}`,
            x: m.x, y: m.y, w: m.width, h: m.height,
            scale: d.get_monitor_scale(i), primary: i === d.get_primary_monitor(),
        };
    });
}

// create opens path for writing, failing if anything is there already (and
// so never following a link planted at the name). rigd chose the name.
function createFile(path) {
    if (typeof path !== 'string' || !path.startsWith('/') || path.includes('\0') || path.includes('..'))
        throw new Refusal('BadArgument', 'the path must be absolute with no ".." or NUL');
    const f = Gio.File.new_for_path(path);
    const parent = f.get_parent();
    if (!parent || !parent.query_exists(null) ||
        parent.query_file_type(Gio.FileQueryInfoFlags.NONE, null) !== Gio.FileType.DIRECTORY)
        throw new Refusal('BadArgument', 'the directory of the path does not exist');
    try {
        return f.create(Gio.FileCreateFlags.PRIVATE, null);
    } catch (e) {
        throw new Refusal('BadArgument', `cannot create the file: ${e.message}`);
    }
}

function checkSize(w, h) {
    if (w <= 0 || h <= 0)
        throw new Refusal('BadArgument', 'the picture has no area');
    if (w > MAX_SIDE || h > MAX_SIDE || w * h > MAX_PIXELS)
        throw new Refusal('TooLarge', `${w}x${h} pixels is over the limit`);
}

export default class RigCapture extends Extension {
    enable() {
        this._inflight = 0;
        this._timers = new Set();
        this._impl = Gio.DBusExportedObject.wrapJSObject(XML, {
            Version: () => [API, PACKAGE_VERSION],
            ListWindows: () => JSON.stringify(windows().map(windowInfo)),
            ListOutputs: () => JSON.stringify(outputs()),
            CaptureWindowAsync: ([id, path], invocation) => this._run(invocation, () => this._window(id, path)),
            CaptureAreaAsync: ([x, y, w, h, cursor, path], invocation) =>
                this._run(invocation, () => this._area(x, y, w, h, cursor, path)),
        });
        this._impl.export(Gio.DBus.session, '/org/rig/Capture');
        this._own = Gio.bus_own_name_on_connection(Gio.DBus.session, 'org.rig.Capture',
            Gio.BusNameOwnerFlags.NONE, null, null);
    }

    disable() {
        for (const id of this._timers)
            GLib.source_remove(id);
        this._timers = null;
        this._impl?.unexport();
        this._impl = null;
        if (this._own)
            Gio.bus_unown_name(this._own);
        this._own = 0;
    }

    // _run answers the call exactly once: with the result, with the error the
    // job threw, or with a timeout if the compositor never finishes.
    _run(invocation, job) {
        let answered = false;
        let timer = 0;
        const finish = (fn) => {
            if (answered)
                return;
            answered = true;
            if (timer) {
                this._timers?.delete(timer);
                GLib.source_remove(timer);
            }
            this._inflight--;
            fn();
        };
        const ok = json => finish(() => invocation.return_value(new GLib.Variant('(s)', [json])));
        const fail = e => finish(() => {
            const name = e instanceof Refusal ? e.dbusName : ERR + 'Failed';
            if (!(e instanceof Refusal))
                console.error(`rigcapture: ${e}\n${e?.stack ?? ''}`);
            invocation.return_dbus_error(name, String(e?.message ?? e));
        });
        if (this._inflight >= MAX_INFLIGHT) {
            invocation.return_dbus_error(ERR + 'Failed', 'too many captures in flight');
            return;
        }
        this._inflight++;
        timer = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, REPLY_TIMEOUT_S, () => {
            this._timers?.delete(timer);
            timer = 0;
            fail(new Error('the compositor did not finish in time'));
            return GLib.SOURCE_REMOVE;
        });
        this._timers.add(timer);
        let p;
        try {
            p = job();
        } catch (e) {
            fail(e);
            return;
        }
        p.then(ok, fail);
    }

    async _window(id, path) {
        const w = windows().find(x => Number(x.get_id()) === Number(id));
        if (!w)
            throw new Refusal('NoSuchWindow', `no window has id ${id}`);
        const actor = w.get_compositor_private();
        if (!actor)
            throw new Refusal('NoSuchWindow', 'the window is going away');
        const tex = actor.paint_to_content(null)?.get_texture?.();
        if (!tex)
            throw new Error('the window could not be painted');
        const tw = tex.get_width(), th = tex.get_height();
        checkSize(tw, th);
        const aw = actor.get_width() || tw;
        const scale = tw / aw;
        const fr = w.get_frame_rect();
        const clamp = (v, hi) => Math.max(0, Math.min(Math.round(v), hi));
        const fx = clamp((fr.x - actor.get_x()) * scale, tw);
        const fy = clamp((fr.y - actor.get_y()) * scale, th);
        const frame = {
            X: fx, Y: fy,
            W: clamp(fr.width * scale, tw - fx), H: clamp(fr.height * scale, th - fy),
        };
        const out = createFile(path);
        try {
            await Shell.Screenshot.composite_to_stream(tex, 0, 0, tw, th, 1, null, 0, 0, 1, out);
            out.close(null);
        } catch (e) {
            try { out.close(null); } catch (_e) { /* already closed */ }
            throw e;
        }
        return JSON.stringify({width: tw, height: th, scale, frame});
    }

    async _area(x, y, w, h, cursor, path) {
        checkSize(w, h);
        const shot = new Shell.Screenshot();
        const [content, scale, cursorContent, cursorPoint, cursorScale] =
            await shot.screenshot_stage_to_content();
        const tex = content.get_texture();
        const sx = Math.max(0, Math.round(x * scale)), sy = Math.max(0, Math.round(y * scale));
        const sw = Math.min(Math.round(w * scale), tex.get_width() - sx);
        const sh = Math.min(Math.round(h * scale), tex.get_height() - sy);
        if (sw <= 0 || sh <= 0)
            throw new Refusal('BadArgument', 'the area lies outside the desktop');
        const ctex = cursor && cursorContent ? cursorContent.get_texture() : null;
        const out = createFile(path);
        try {
            await Shell.Screenshot.composite_to_stream(tex, sx, sy, sw, sh, scale,
                ctex, cursorPoint?.x ?? 0, cursorPoint?.y ?? 0, cursorScale ?? 1, out);
            out.close(null);
        } catch (e) {
            try { out.close(null); } catch (_e) { /* already closed */ }
            throw e;
        }
        // Whether the pointer is in the picture: a hidden pointer is not an error.
        return JSON.stringify({width: sw, height: sh, scale, cursor: ctex ? 'drawn' : (cursor ? 'hidden' : '')});
    }
}
