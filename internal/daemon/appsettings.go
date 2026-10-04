package daemon

import (
	"fmt"
	"path/filepath"

	"github.com/borismilner/rig/internal/config"
)

// A program's settings (plan/47 decision 1, plan/55 requirement 20): it
// declares a schema at hello, rig resolves it over the program layers, and
// config.get and config.set with program set read and change it, a change
// lasting in ~/.config/rig/apps/<id>.toml and posted as config.changed with
// the program named. The program reads its values the same way, and waits on
// config.changed rather than polling.

// programSettings compiles a program's declared schema and resolves it. A
// schema rig cannot compile refuses the hello: the declaration is the
// program's, and a setting it cannot be asked for is a defect to fix now.
// Each hello resolves afresh: a change made through rig is in the file.
func (d *Daemon) programSettings(id, schema string) (*config.Resolver, error) {
	if schema == "" {
		return nil, nil
	}
	s, err := config.NewSchema([]byte(schema))
	if err != nil {
		return nil, fmt.Errorf("hello: settings_schema: %w", err)
	}
	var file string
	if d.appsDir != "" {
		file = filepath.Join(d.appsDir, id+".toml")
	}
	return config.LoadProgram(s, file), nil
}

// appResolver is a program's resolver, or nil when it declared no settings.
func (d *Daemon) appResolver(id string) *config.Resolver {
	v, _ := d.appSettings.Load(id)
	r, _ := v.(*config.Resolver)
	return r
}
