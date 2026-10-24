package replay

import "testing"

func TestSizeOf(t *testing.T) {
	cases := map[int]string{0: "small", 80: "small", 81: "medium", 500: "medium", 501: "big", 2000: "big", 2001: "agent-scale"}
	for lines, want := range cases {
		if got := SizeOf(lines); got != want {
			t.Errorf("SizeOf(%d) = %s, want %s", lines, got, want)
		}
	}
}

func TestSumNumstat(t *testing.T) {
	files, lines := sumNumstat("3\t1\ta.go\n-\t-\timage.png\n10\t0\tb.go\n")
	if files != 3 || lines != 14 {
		t.Errorf("got %d files, %d lines", files, lines)
	}
}
