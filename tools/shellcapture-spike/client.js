imports.gi.versions.Gtk = '4.0';
const {Gtk, Gdk, GLib} = imports.gi;
Gtk.init();
const loop = GLib.MainLoop.new(null, false);
const css = new Gtk.CssProvider();
const cols = {red: '#ff0000', green: '#00ff00', blue: '#0000ff'};
css.load_from_string(Object.entries(cols).map(([n, c]) => `window.${n} { background: ${c}; }`).join('\n'));
Gtk.StyleContext.add_provider_for_display(Gdk.Display.get_default(), css, 800);
for (const n of Object.keys(cols)) {
    const w = new Gtk.Window({title: 'rigspike-' + n, default_width: 400, default_height: 300});
    w.add_css_class(n); w.present();
}
loop.run();
