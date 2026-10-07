package benchmark

import "testing"

func TestIntegerSequenceRejectsIncompleteAndExtraOutput(t *testing.T) {
	v := Verify{IntegerSequence: &IntegerSequence{Start: 1, End: 3, EndMarker: "BENCH_DONE"}}
	for _, output := range []string{"BENCH_DONE", "1\n3\nBENCH_DONE", "1\n2\n2\nBENCH_DONE", "2\n1\n3\nBENCH_DONE", "1\n2\n3\nWRONG", "1\n2\n3\nBENCH_DONE\n\n"} {
		if v.CheckOutput(output) == "" {
			t.Fatalf("accepted %q", output)
		}
	}
	for _, output := range []string{"1\n2\n3\nBENCH_DONE", "1\n2\n3\nBENCH_DONE\n", "1\r\n2\r\n3\r\nBENCH_DONE\r\n"} {
		if err := v.CheckOutput(output); err != "" {
			t.Fatal(output, err)
		}
	}
	empty := ""
	exact := Verify{OutputEquals: &empty}
	if !exact.HasChecks() || exact.CheckOutput("") != "" || exact.CheckOutput("\n") == "" {
		t.Fatal("byte equality or empty assertion lost")
	}
}
