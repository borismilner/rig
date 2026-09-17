package main

import (
	"strings"
	"testing"
)

// ⛔ THE UNLISTED LINE IS SUPPRESSED AT ZERO AND PRINTED OTHERWISE, AND THE
// ASYMMETRY IS THE THING UNDER TEST RATHER THAN A DETAIL OF IT.
//
// The line exists to correct ONE inference - that OPEN plus CLOSED is the
// total - and a brief with nothing unlisted has no wrong inference to correct.
// Printing "0 work items are in neither list" would spend a line of the
// reader's attention saying nothing, which is the charge against the closed
// section itself if it is not careful; suppressing it when it is non-zero
// leaves a reader adding two lists up and wrong with no way to find out. Both
// directions are checked because the two repairs are each other's failure mode.
func TestTheUnlistedCountPrintsOnlyWhenThereIsSomethingToCorrect(t *testing.T) {
	const phrase = "in neither list"

	got := briefClosedSection(nil, nil, 0, briefStyle{})
	if strings.Contains(got, phrase) {
		t.Errorf("a brief with nothing unlisted still printed the correction:\n%s", got)
	}
	if !strings.Contains(got, "CLOSED") {
		t.Errorf("the section did not print its own heading, so a reader "+
			"cannot tell an empty closed list from a build that has none:\n%s", got)
	}

	got = briefClosedSection(nil, nil, 11, briefStyle{})
	if !strings.Contains(got, phrase) {
		t.Errorf("11 work items are in neither list and the brief said nothing, "+
			"so OPEN + CLOSED reads as the total and is short by 11:\n%s", got)
	}
	if !strings.Contains(got, "11") {
		t.Errorf("the correction printed without the number, which tells a "+
			"reader they are wrong and not by how much:\n%s", got)
	}
}

// ⛔ A CLOSING WORD THAT ARRIVES EMPTY IS NAMED AS A DEFECT AND NOT RENDERED
// BLANK, for briefGoverningKindCell's reason: a blank cell reads as a closure
// called nothing, and the word is the whole row.
//
// The store cannot produce this - an item with no closing word goes to the
// unlisted count instead - so the case is reachable only from a daemon whose
// wire dropped the field, which is exactly the skew a renderer must not absorb
// silently.
func TestAnEmptyClosingWordIsNamedRatherThanLeftBlank(t *testing.T) {
	got := briefClosedSection(
		[]BriefClosedItem{{ID: "B90", Title: "something", Word: ""}},
		[]BriefWordCount{{Word: "", Count: 1}}, 0, briefStyle{})

	if !strings.Contains(got, "defect") {
		t.Errorf("a closed row arrived with no word and the cell says nothing "+
			"about it, so the skew renders as a closure called nothing:\n%s", got)
	}
}

// ⛔ THE COUNTS AND THE LIST EACH PRINT WHEN THE OTHER IS ABSENT. Either
// arriving alone is a fact about the derivation or the wire, and one condition
// covering both would hide whichever half is missing - briefGoverningSection's
// rule, and the defect it was written against.
func TestTheClosedListAndItsCensusEachSurviveTheOtherBeingAbsent(t *testing.T) {
	rowsOnly := briefClosedSection(
		[]BriefClosedItem{{ID: "B90", Title: "finished", Word: "closed"}},
		nil, 0, briefStyle{})
	if !strings.Contains(rowsOnly, "B90") {
		t.Errorf("the rows arrived without a census and were dropped:\n%s", rowsOnly)
	}

	countsOnly := briefClosedSection(nil,
		[]BriefWordCount{{Word: "closed-by-ruling", Count: 2}}, 0, briefStyle{})
	if !strings.Contains(countsOnly, "closed-by-ruling") {
		t.Errorf("the census arrived without its rows and was dropped, so a "+
			"reader learns nothing about the 2 records behind it:\n%s", countsOnly)
	}
}
