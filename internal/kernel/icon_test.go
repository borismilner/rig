package kernel_test

import (
	"strings"
	"testing"
)

// identity.icon is a name the window looks up in its own set (plan/11), so
// anything that is not a plain bounded name is refused at registration.
func TestAnIconIsAPlainBoundedName(t *testing.T) {
	for _, icon := range []string{"", "warehouse", "book-open", "sh", "git-pull-request"} {
		d := good("shelf")
		d.Identity.Icon = icon
		if err := d.Validate(); err != nil {
			t.Errorf("icon %q refused: %v", icon, err)
		}
	}
	for _, icon := range []string{"<svg onload=x>", "Warehouse", "book open", "../x", "é", strings.Repeat("a", 65)} {
		d := good("shelf")
		d.Identity.Icon = icon
		err := d.Validate()
		if err == nil || !strings.Contains(err.Error(), "identity.icon") {
			t.Errorf("icon %q: %v", icon, err)
		}
	}
}
