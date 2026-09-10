import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import wails from "@wailsio/runtime/plugins/vite";

// The build lands beside the Go package that embeds it, not in frontend/dist.
// The sources have to stay here because the Makefile's four `cd frontend`
// targets were written before any of this existed; the binary has to embed from
// its own directory because go:embed cannot reach above a package. outDir is
// outside vite's root, so emptying it is refused unless asked for - and it is
// not asked for, because dist/.gitkeep is tracked and `go vet ./...` needs the
// directory to exist on a tree nobody has built yet. The Makefile clears stale
// output instead.
const outDir = "../cmd/rigwindow/dist";

export default defineConfig({
  // Relative asset paths, so the built page also opens from disk. Vite's
  // default writes /assets/..., which the window's asset server resolves fine
  // and a file:// URL does not - and the contrast gate audits a path, not a
  // URL. With absolute paths it loaded index.html, rendered nothing, measured
  // 0 nodes and reported clean.
  base: "./",
  build: {
    outDir,
    emptyOutDir: false,
  },
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [tailwindcss(), svelte(), wails("./bindings")],
});
