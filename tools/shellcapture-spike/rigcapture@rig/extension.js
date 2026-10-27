import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Shell from 'gi://Shell';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const XML = `<node><interface name="org.rig.Capture">
<method name="ListWindows"><arg type="s" direction="out" name="json"/></method>
<method name="CaptureWindow"><arg type="t" direction="in" name="id"/><arg type="s" direction="in" name="path"/><arg type="s" direction="out" name="result"/></method>
<method name="Arrange"><arg type="s" direction="out" name="result"/></method>
</interface></node>`;

const wins = () => global.get_window_actors().map(a => a.meta_window);

export default class RigCapture extends Extension {
    enable() {
        const self = this;
        this._impl = Gio.DBusExportedObject.wrapJSObject(XML, {
            ListWindows() {
                return JSON.stringify(wins().map(w => {
                    const r = w.get_frame_rect(), a = w.get_compositor_private();
                    return {id: Number(w.get_id()), title: w.get_title(), cls: w.get_wm_class(),
                        ws: w.get_workspace()?.index() ?? -1, min: w.minimized,
                        focus: w.has_focus(), vis: a?.visible, x: r.x, y: r.y, w: r.width, h: r.height};
                }));
            },
            CaptureWindowAsync([id, path], invocation) {
                const fail = m => invocation.return_value(new GLib.Variant('(s)', [m]));
                try {
                    const w = wins().find(x => Number(x.get_id()) === Number(id));
                    if (!w) return fail('no such window');
                    const a = w.get_compositor_private();
                    const tex = a.paint_to_content(null)?.get_texture?.();
                    if (!tex) return fail('no texture');
                    const out = Gio.File.new_for_path(path).replace(null, false, Gio.FileCreateFlags.NONE, null);
                    const wd = tex.get_width(), ht = tex.get_height();
                    Shell.Screenshot.composite_to_stream(tex, 0, 0, wd, ht, 1, null, 0, 0, 1, out)
                        .then(() => { out.close(null); fail('ok ' + wd + 'x' + ht); })
                        .catch(e => fail('error: ' + e));
                } catch (e) { fail('error: ' + e + ' ' + e.stack); }
            },
            Arrange() {
                const ws = global.workspace_manager;
                for (const w of wins()) {
                    if (w.get_title() === 'rigspike-green') {
                        ws.append_new_workspace(false, global.get_current_time());
                        w.change_workspace_by_index(1, false);
                    }
                    if (w.get_title() === 'rigspike-blue') w.minimize();
                }
                return 'arranged';
            },
        });
        this._impl.export(Gio.DBus.session, '/org/rig/Capture');
        this._own = Gio.bus_own_name_on_connection(Gio.DBus.session, 'org.rig.Capture',
            Gio.BusNameOwnerFlags.NONE, null, null);
    }
    disable() {
        this._impl?.unexport(); this._impl = null;
        if (this._own) Gio.bus_unown_name(this._own);
    }
}
