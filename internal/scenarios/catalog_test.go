package scenarios

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestPublishedPreparationSnapshotStaysCompatible(t *testing.T) {
	want := []byte(`{"id":"spot-basics","version":1,"title":"Spot basics: your demo account","townHall":1,"market":"spot","currency":"USD","startingBalanceMinor":"100000","currencyScale":2,"durationSeconds":180,"endPolicy":"cancel_pending_and_close_holdings","executionEnabled":false,"objectives":["Understand your demo cash balance","Read the session timer and account history"]}`)
	got, _ := Foundation().Snapshot()
	if !bytes.Equal(want, got) {
		t.Fatal("published preparation snapshot changed")
	}
}
func TestPrivateConditionsSurviveStorageWithoutHTTPExposure(t *testing.T) {
	all, err := Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range all {
		if d.Training == nil {
			continue
		}
		stored, _ := d.Snapshot()
		restored, err := Restore(stored)
		if err != nil || restored.Training == nil || restored.Training.Hash() != d.Training.Hash() {
			t.Fatal("condition snapshot lost")
		}
		public, _ := json.Marshal(restored)
		if bytes.Contains(public, []byte("frames")) || bytes.Contains(public, []byte("privateTraining")) {
			t.Fatal("private condition leaked")
		}
	}
}
