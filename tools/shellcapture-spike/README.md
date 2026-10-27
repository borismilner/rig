# shellcapture-spike

Throwaway proof for plan section 58. A Shell extension exports
`org.rig.Capture` on the session bus and paints any window actor to a PNG.
`inner.sh` runs it all inside a headless GNOME Shell with its own D-Bus
session, settings and Wayland socket, so the live desktop is untouched:

    dbus-run-session -- ./inner.sh "$PWD/work"   # work/ needs data/gnome-shell/extensions/rigcapture@rig copied in

Not the product. The product is a rig capability (section 58).
