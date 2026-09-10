package diff

import "testing"

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
