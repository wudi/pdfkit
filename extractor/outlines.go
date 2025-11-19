package extractor

import "pdflib/ir/raw"

// Bookmark describes a PDF outline entry.
type Bookmark struct {
	Title    string
	Page     int
	Children []Bookmark
}

// TOCEntry is a flattened bookmark entry augmented with labels and depth.
type TOCEntry struct {
	Title string
	Page  int
	Label string
	Depth int
}

// ExtractBookmarks walks the document outline tree (if present).
func (e *Extractor) ExtractBookmarks() []Bookmark {
	outlineDict := derefDict(e.raw, valueFromDict(e.catalog, "Outlines"))
	if outlineDict == nil {
		return nil
	}
	return buildOutlineBranch(e.raw, valueFromDict(outlineDict, "First"), e.pages)
}

// ExtractTableOfContents flattens bookmarks and attaches page labels.
func (e *Extractor) ExtractTableOfContents() []TOCEntry {
	bookmarks := e.ExtractBookmarks()
	var entries []TOCEntry
	var walk func(items []Bookmark, depth int)
	walk = func(items []Bookmark, depth int) {
		for _, item := range items {
			label := ""
			if item.Page >= 0 {
				label = e.pageLabels[item.Page]
			}
			entries = append(entries, TOCEntry{
				Title: item.Title,
				Page:  item.Page,
				Label: label,
				Depth: depth,
			})
			if len(item.Children) > 0 {
				walk(item.Children, depth+1)
			}
		}
	}
	walk(bookmarks, 0)
	return entries
}

func buildOutlineBranch(doc *raw.Document, obj raw.Object, pages []*raw.DictObj) []Bookmark {
	if obj == nil {
		return nil
	}
	var list []Bookmark
	current := obj
	for current != nil {
		dict := derefDict(doc, current)
		if dict == nil {
			break
		}
		title, _ := stringFromObject(valueFromDict(dict, "Title"))
		page := resolveDestPage(doc, valueFromDict(dict, "Dest"), pages)
		if page == -1 {
			page = resolveActionDest(doc, valueFromDict(dict, "A"), pages)
		}
		bookmark := Bookmark{Title: title, Page: page}
		bookmark.Children = buildOutlineBranch(doc, valueFromDict(dict, "First"), pages)
		list = append(list, bookmark)
		next := valueFromDict(dict, "Next")
		if next == nil {
			break
		}
		current = next
	}
	return list
}

func resolveDestPage(doc *raw.Document, obj raw.Object, pages []*raw.DictObj) int {
	if obj == nil {
		return -1
	}
	resolved := deref(doc, obj)
	switch v := resolved.(type) {
	case raw.RefObj:
		pageDict := derefDict(doc, v)
		return indexOfPage(pageDict, pages)
	case *raw.ArrayObj:
		if len(v.Items) == 0 {
			return -1
		}
		return resolveDestPage(doc, v.Items[0], pages)
	}
	return indexOfPage(derefDict(doc, resolved), pages)
}

func resolveActionDest(doc *raw.Document, obj raw.Object, pages []*raw.DictObj) int {
	action := derefDict(doc, obj)
	if action == nil {
		return -1
	}
	if typ, ok := nameFromDict(action, "S"); !ok || typ != "GoTo" {
		return -1
	}
	return resolveDestPage(doc, valueFromDict(action, "D"), pages)
}

func indexOfPage(target *raw.DictObj, pages []*raw.DictObj) int {
	if target == nil {
		return -1
	}
	for idx, page := range pages {
		if page == target {
			return idx
		}
	}
	return -1
}
