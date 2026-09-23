package record

import (
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

// TestTheFourLatestStepFormulationsAgreeAndAreMeasured runs every formulation
// against ONE corpus, checks they agree, and prints what each cost.
//
// ⛔ AGREEMENT IS CHECKED BEFORE SPEED, because the ruling this exists to serve
// is that the formulation is a CORRECTNESS surface. A form that is fast and
// returns two "latest" steps for one item has not won anything.
//
// Set RECORD_BENCH_ITEMS / RECORD_BENCH_STEPS to change the shape. The default
// is small enough for the normal suite; the numbers in the commit body were
// taken at 44 items x 1000 steps, which is section 39's measured shape.
func TestTheFourLatestStepFormulationsAgreeAndAreMeasured(t *testing.T) {
	items := envInt("RECORD_BENCH_ITEMS", 44)
	steps := envInt("RECORD_BENCH_STEPS", 20)

	name := estate(t, "development")
	s := openStore(t, name)
	seedStreams(t, s, items, steps)

	forms := make([]string, 0, len(latestStepSQL))
	for f := range latestStepSQL {
		forms = append(forms, f)
	}
	sort.Strings(forms)

	type result struct {
		form string
		d    time.Duration
		n    int
	}
	var (
		results []result
		ref     map[string]string
	)
	for _, form := range forms {
		// Warm, then measure the median of five.
		if _, err := s.LatestStepsUsing(tctx, form, "rig"); err != nil {
			t.Fatalf("%s: %v", form, err)
		}
		var ds []time.Duration
		var got map[string]Record
		for range 5 {
			start := time.Now()
			var err error
			got, err = s.LatestStepsUsing(tctx, form, "rig")
			if err != nil {
				t.Fatalf("%s: %v", form, err)
			}
			ds = append(ds, time.Since(start))
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] < ds[j] })

		flat := map[string]string{}
		for item, rec := range got {
			flat[item] = rec.ID
		}
		if ref == nil {
			ref = flat
		} else if len(flat) != len(ref) {
			t.Fatalf("%s returned %d items, the first form returned %d", form, len(flat), len(ref))
		} else {
			for item, id := range ref {
				if flat[item] != id {
					t.Fatalf("⛔ %s disagrees on item %s: %s vs %s. The formulations do not return the same rows.",
						form, item, flat[item], id)
				}
			}
		}
		results = append(results, result{form, ds[len(ds)/2], len(got)})
	}

	if len(ref) != items {
		t.Fatalf("the derivation found %d items, want %d", len(ref), items)
	}

	sort.Slice(results, func(i, j int) bool { return results[i].d < results[j].d })
	fmt.Printf("\n  latest-step-per-item: %d items x %d steps = %d progress records\n",
		items, steps, items*steps)
	fmt.Printf("  %-14s %12s %10s   %s\n", "formulation", "median of 5", "vs best", "")
	for _, r := range results {
		mark := ""
		if r.form == latestStepForm {
			mark = "  <- IN USE"
		}
		fmt.Printf("  %-14s %12s %9.1fx%s\n", r.form, r.d.Round(time.Microsecond),
			float64(r.d)/float64(results[0].d), mark)
	}
	fmt.Printf("  R2.3 target for project.brief: 50ms (a TARGET, knowingly unverified until slice 3)\n\n")
}

// seedStreams writes the corpus directly, because building it through Step
// would be one transaction per row and would measure the writer rather than
// the reader. The READ path under test is untouched by this.
func seedStreams(t *testing.T, s *Store, items, steps int) {
	t.Helper()
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()

	states := []string{"started", "blocked", "started"}
	for i := range items {
		item := fmt.Sprintf("wi-%04d", i)
		if _, err := tx.Exec(`INSERT INTO records (id, version, kind, project, body, fields,
			session, seat, epoch, created_at) VALUES (?,1,'work-item','rig','',?,'bench','bench',6,?)`,
			item, `{"title":"`+item+`","status":"active"}`, time.Now().UnixNano()); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO heads (id, version) VALUES (?, 1)`, item); err != nil {
			t.Fatal(err)
		}
		for j := range steps {
			id, err := uuidV7()
			if err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO records (id, version, kind, project, body, fields,
				session, seat, epoch, created_at) VALUES (?,1,'progress','rig','',?,'bench','bench',6,?)`,
				id, `{"state":"`+states[j%len(states)]+`","item":"`+item+`"}`, time.Now().UnixNano()); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO heads (id, version) VALUES (?, 1)`, id); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(`INSERT INTO links (src, type, dst) VALUES (?, 'part-of', ?)`, id, item); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil && n > 0 {
			return n
		}
	}
	return def
}
