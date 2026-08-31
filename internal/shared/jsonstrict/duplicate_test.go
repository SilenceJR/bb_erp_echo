package jsonstrict

import "testing"

func TestRejectDuplicateKeys(t *testing.T) {
	for _, payload := range []string{
		`{"a":1,"a":2}`,
		`{"outer":{"value":1,"value":2}}`,
		`{"items":[{"id":1,"id":2}]}`,
	} {
		if err := RejectDuplicateKeys([]byte(payload)); err == nil {
			t.Fatalf("expected duplicate rejection for %s", payload)
		}
	}
	if err := RejectDuplicateKeys([]byte(`{"outer":{"value":1},"items":[{"id":1}]}`)); err != nil {
		t.Fatal(err)
	}
}
