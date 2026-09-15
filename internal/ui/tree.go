package ui

import (
	"sort"
	"strings"

	"ghpr/internal/diff"
)

// treeNode is one visible line of the file tree: either a directory or a
// file. Directories are identified by their full path prefix (without a
// trailing slash); files carry the index into Model.files.
type treeNode struct {
	label   string // text shown (directory name(s) or file base name)
	path    string // directory path or file path
	depth   int
	isDir   bool
	fileIdx int // valid when !isDir
	// aggregates for directories
	files, viewed int
	adds, dels    int
}

type dirEntry struct {
	name    string
	dirs    map[string]*dirEntry
	fileIdx []int
}

func newDir(name string) *dirEntry {
	return &dirEntry{name: name, dirs: map[string]*dirEntry{}}
}

// buildTree returns the visible nodes of the file tree given which
// directories are collapsed. Directory chains with a single child directory
// and no files are compacted into one node ("a/b/c"). shown, when non-nil,
// filters the files by index; directories with no shown files are omitted.
func buildTree(files []diff.File, collapsed map[string]bool, isViewed func(string) bool, shown func(int) bool) []treeNode {
	root := newDir("")
	for i := range files {
		if shown != nil && !shown(i) {
			continue
		}
		parts := strings.Split(files[i].Path(), "/")
		d := root
		for _, p := range parts[:len(parts)-1] {
			child, ok := d.dirs[p]
			if !ok {
				child = newDir(p)
				d.dirs[p] = child
			}
			d = child
		}
		d.fileIdx = append(d.fileIdx, i)
	}

	var out []treeNode
	var walk func(d *dirEntry, prefix string, depth int)
	walk = func(d *dirEntry, prefix string, depth int) {
		names := make([]string, 0, len(d.dirs))
		for n := range d.dirs {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			child := d.dirs[n]
			label, path := n, joinPath(prefix, n)
			// Compact single-child directory chains.
			for len(child.dirs) == 1 && len(child.fileIdx) == 0 {
				var only *dirEntry
				for _, c := range child.dirs {
					only = c
				}
				label = label + "/" + only.name
				path = joinPath(path, only.name)
				child = only
			}
			node := treeNode{label: label, path: path, depth: depth, isDir: true}
			aggregate(child, files, isViewed, &node)
			out = append(out, node)
			if !collapsed[path] {
				walk(child, path, depth+1)
			}
		}
		idx := append([]int(nil), d.fileIdx...)
		sort.Slice(idx, func(a, b int) bool { return files[idx[a]].Path() < files[idx[b]].Path() })
		for _, fi := range idx {
			p := files[fi].Path()
			out = append(out, treeNode{label: p[strings.LastIndex(p, "/")+1:], path: p, depth: depth, fileIdx: fi})
		}
	}
	walk(root, "", 0)
	return out
}

func joinPath(prefix, name string) string {
	if prefix == "" {
		return name
	}
	return prefix + "/" + name
}

func aggregate(d *dirEntry, files []diff.File, isViewed func(string) bool, n *treeNode) {
	for _, fi := range d.fileIdx {
		n.files++
		n.adds += files[fi].Additions
		n.dels += files[fi].Deletions
		if isViewed != nil && isViewed(files[fi].Path()) {
			n.viewed++
		}
	}
	for _, c := range d.dirs {
		aggregate(c, files, isViewed, n)
	}
}

// dirsOf returns every ancestor directory path of a file path.
func dirsOf(path string) []string {
	var out []string
	for i, c := range path {
		if c == '/' {
			out = append(out, path[:i])
		}
	}
	return out
}
