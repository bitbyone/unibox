package cli

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestTheSpinnerIsClearedBeforeAnythingElseIsWritten(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	s := &taskSpinner{out: &out}
	s.set("preparing task pricing")
	// As spin draws it.
	fmt.Fprint(&out, "⠋ preparing task pricing")
	s.drawn = true
	w := spinnerWriter{s, &out}
	fmt.Fprintln(w, "mounted ~/code/web")
	if got := out.String(); got != "⠋ preparing task pricing\r\x1b[Kmounted ~/code/web\n" || s.drawn {
		t.Fatalf("%q", got)
	}
	// Handing the terminal on clears it, and nothing is drawn after.
	out.Reset()
	s.drawn = true
	s.set("")
	if out.String() != "\r\x1b[K" || s.drawn || s.what != "" {
		t.Fatalf("%q", out.String())
	}
	if strings.Contains(out.String(), "preparing") {
		t.Fatal("drawn after the terminal was handed on")
	}
}
