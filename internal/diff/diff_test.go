package diff

import (
	"fmt"
	"strings"
	"testing"
)

const sample = `diff --git a/main.go b/main.go
index 1111111..2222222 100644
--- a/main.go
+++ b/main.go
@@ -1,6 +1,8 @@
 package main
 
-import "fmt"
+import (
+	"fmt"
+)
 
 func main() {
 	fmt.Println("hi")
diff --git a/new.txt b/new.txt
new file mode 100644
index 0000000..3333333
--- /dev/null
+++ b/new.txt
@@ -0,0 +1,2 @@
+hello
+world
diff --git a/img.png b/img.png
new file mode 100644
Binary files /dev/null and b/img.png differ
diff --git a/old.txt b/renamed.txt
similarity index 100%
rename from old.txt
rename to renamed.txt
`

func TestParse(t *testing.T) {
	files := Parse(sample)
	if len(files) != 4 {
		t.Fatalf("want 4 files, got %d", len(files))
	}
	f := files[0]
	if f.Path() != "main.go" || f.Status != Modified || len(f.Hunks) != 1 {
		t.Fatalf("main.go: %+v", f)
	}
	h := f.Hunks[0]
	if h.OldStart != 1 || h.OldCount != 6 || h.NewStart != 1 || h.NewCount != 8 {
		t.Fatalf("hunk header: %+v", h)
	}
	if f.Additions != 3 || f.Deletions != 1 {
		t.Fatalf("counts: +%d -%d", f.Additions, f.Deletions)
	}
	// Line numbering: the removed import is old line 3; first added is new line 3.
	var del, add *Line
	for i := range h.Lines {
		l := &h.Lines[i]
		if l.Kind == Del && del == nil {
			del = l
		}
		if l.Kind == Add && add == nil {
			add = l
		}
	}
	if del.OldNum != 3 || del.NewNum != 0 || add.NewNum != 3 || add.OldNum != 0 {
		t.Fatalf("numbering del=%+v add=%+v", del, add)
	}
	last := h.Lines[len(h.Lines)-1]
	if last.OldNum != 6 || last.NewNum != 8 {
		t.Fatalf("last context line %+v", last)
	}

	if files[1].Status != Added || files[1].Additions != 2 {
		t.Fatalf("new.txt: %+v", files[1])
	}
	if !files[2].IsBinary {
		t.Fatalf("img.png should be binary")
	}
	if files[3].Status != Renamed || files[3].OldPath != "old.txt" || files[3].NewPath != "renamed.txt" {
		t.Fatalf("rename: %+v", files[3])
	}
}

func TestSideBySide(t *testing.T) {
	files := Parse(sample)
	rows := SideBySide(&files[0].Hunks[0])
	// 2 ctx, then 1 del vs 3 adds -> 3 rows, then 3 ctx = 8 rows.
	if len(rows) != 8 {
		t.Fatalf("want 8 rows, got %d", len(rows))
	}
	if rows[2].Left == nil || rows[2].Right == nil || rows[2].Left.Kind != Del || rows[2].Right.Kind != Add {
		t.Fatalf("row 2 should pair del/add: %+v", rows[2])
	}
	if rows[3].Left != nil || rows[3].Right == nil {
		t.Fatalf("row 3 should be add-only: %+v", rows[3])
	}
	if rows[0].Left != rows[0].Right {
		t.Fatalf("context row should share the line")
	}
}

const expandSample = `diff --git a/f.txt b/f.txt
--- a/f.txt
+++ b/f.txt
@@ -2,3 +2,4 @@
 b
-c
+C
+C2
 d
@@ -7,0 +9,2 @@
+X
+Y
@@ -10,2 +12,0 @@
-j
-k
`

func TestExpand(t *testing.T) {
	files := Parse(expandSample)
	if len(files) != 1 {
		t.Fatalf("files %d", len(files))
	}
	// new file: a b C C2 d e f g X Y h i (12 lines); old: a b c d e f g h i j k (11 lines)
	content := "a\nb\nC\nC2\nd\ne\nf\ng\nX\nY\nh\ni\n"
	full := Expand(&files[0], content)
	if !full.Full || len(full.Hunks) != 1 {
		t.Fatalf("expected one full hunk")
	}
	var got []string
	for _, l := range full.Hunks[0].Lines {
		sign := " "
		switch l.Kind {
		case Add:
			sign = "+"
		case Del:
			sign = "-"
		}
		got = append(got, fmt.Sprintf("%s%d/%d:%s", sign, l.OldNum, l.NewNum, l.Text))
	}
	want := []string{
		" 1/1:a", " 2/2:b", "-3/0:c", "+0/3:C", "+0/4:C2", " 4/5:d", " 5/6:e", " 6/7:f", " 7/8:g",
		"+0/9:X", "+0/10:Y", " 8/11:h", " 9/12:i", "-10/0:j", "-11/0:k",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("expand mismatch\n got %v\nwant %v", got, want)
	}
	h := full.Hunks[0]
	if h.NewCount != 12 || h.OldCount != 11 {
		t.Fatalf("counts old=%d new=%d", h.OldCount, h.NewCount)
	}
	// Empty content still lists the hunks.
	if e := Expand(&files[0], ""); len(e.Hunks[0].Lines) == 0 {
		t.Fatalf("empty content should still contain hunk lines")
	}
}

func TestInDiff(t *testing.T) {
	f := Parse(expandSample)[0]
	cases := []struct {
		side string
		n    int
		want bool
	}{
		{"RIGHT", 3, true}, {"RIGHT", 2, true}, {"RIGHT", 6, false}, {"RIGHT", 9, true},
		{"LEFT", 3, true}, {"LEFT", 10, true}, {"LEFT", 6, false}, {"LEFT", 0, false},
	}
	for _, c := range cases {
		if got := f.InDiff(c.side, c.n); got != c.want {
			t.Errorf("InDiff(%s,%d)=%v want %v", c.side, c.n, got, c.want)
		}
	}
}

func TestFingerprint(t *testing.T) {
	a := Parse(sample)
	b := Parse(sample)
	if a[0].Fingerprint() != b[0].Fingerprint() {
		t.Fatal("same diff must hash the same")
	}
	if a[0].Fingerprint() == a[1].Fingerprint() {
		t.Fatal("different files must differ")
	}
	c := Parse(strings.Replace(sample, "fmt.Println(\"hi\")", "fmt.Println(\"bye\")", 1))
	if c[0].Fingerprint() == a[0].Fingerprint() {
		t.Fatal("changed content must change the fingerprint")
	}
	if len(a[0].Fingerprint()) != 32 {
		t.Fatalf("fingerprint length %d", len(a[0].Fingerprint()))
	}
}

func TestParseHunks(t *testing.T) {
	hunks := ParseHunks("@@ -1,2 +1,3 @@\n a\n-b\n+c\n+d\n@@ -10 +11 @@\n-x\n+y")
	if len(hunks) != 2 || len(hunks[0].Lines) != 4 || hunks[1].OldStart != 10 || hunks[1].NewStart != 11 {
		t.Fatalf("hunks = %+v", hunks)
	}
	if hunks[0].Lines[2].Kind != Add || hunks[0].Lines[2].NewNum != 2 {
		t.Fatalf("line numbering: %+v", hunks[0].Lines[2])
	}
	if ParseHunks("") != nil {
		t.Fatal("empty patch has no hunks")
	}
}
