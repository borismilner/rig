imports.gi.versions.Gtk = '4.0';
const {Gtk, Gdk, GLib} = imports.gi;
Gtk.init();
const loop = GLib.MainLoop.new(null, false);
const css = new Gtk.CssProvider();
const secondary = GLib.getenv('RIG_SHELL_E2E_SECONDARY') === '1';
const cols = secondary
    ? {unmarked: '#ffff00'}
    : {red: '#ff0000', green: '#00ff00', blue: '#0000ff'};
css.load_from_string(Object.entries(cols).map(([n, c]) => `window.${n} { background: ${c}; }`).join('\n'));
Gtk.StyleContext.add_provider_for_display(Gdk.Display.get_default(), css, 800);
const windows = new Map();
for (const n of Object.keys(cols)) {
    const w = new Gtk.Window({title: 'rigspike-' + n, default_width: 400, default_height: 300});
    w.add_css_class(n); w.present();
    windows.set(n, w);
}
if (!secondary)
    GLib.unix_signal_add(GLib.PRIORITY_DEFAULT, 10, () => {
        windows.get('blue').maximize();
        return GLib.SOURCE_CONTINUE;
    });
loop.run();
