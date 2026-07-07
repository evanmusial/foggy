package foggy

import "testing"

func TestPasswordWrapRoundTrip(t *testing.T) {
	dbKey, err := randomDBKey()
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := wrapWithPassword(dbKey, "a very long test password")
	if err != nil {
		t.Fatal(err)
	}
	out, err := unwrapWithPassword(wrapped, "a very long test password")
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(dbKey) {
		t.Fatal("unwrapped key mismatch")
	}
	if _, err := unwrapWithPassword(wrapped, "wrong password"); err == nil {
		t.Fatal("wrong password unexpectedly unwrapped key")
	}
}

func TestBackupCodeNormalization(t *testing.T) {
	got := normalizeCode("abcd efgh-ijkl mnop")
	want := "ABCD-EFGH-IJKL-MNOP"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
