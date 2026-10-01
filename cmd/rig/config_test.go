package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// useDisplayName stands in for what rigd would answer for display.name.
func useDisplayName(t *testing.T, name string) {
	t.Helper()
	saved := displayName
	t.Cleanup(func() { displayName = saved })
	displayName = func() string { return name }
}

func TestAnExportReadsBackAsItsKeys(t *testing.T) {
	p := filepath.Join(t.TempDir(), "export.toml")
	body := "# rig's resolved configuration\n\n# log.level from default\nlog.level = \"info\"\ndisplay.name = \"B \\\"q\\\"\"\n"
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readExport(p)
	if err != nil || got["log.level"] != `"info"` || got["display.name"] != `"B \"q\""` || len(got) != 2 {
		t.Fatalf("read %v, %v", got, err)
	}
	if err := os.WriteFile(p, []byte("log.level = info\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readExport(p); err == nil {
		t.Fatal("a line that is not an export's was read")
	}
}

func TestDiffAnswersTwoWhenTheFileIsUnreadable(t *testing.T) {
	err := configDiff(filepath.Join(t.TempDir(), "absent"), false)
	if errorCode(err) != 2 {
		t.Fatalf("an unreadable file exited %d: %v", errorCode(err), err)
	}
	if errorCode(errors.New("plain")) != 1 {
		t.Fatal("a plain error is not exit 1")
	}
}
