package lilgraph

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

func marshalText(g *Lilgraph) ([]byte, error) {
	// TODO: take various options for formatting, e.g.:
	// const indent = "    "
	// const wrapAt = 100

	items := inPrintOrder(g)
	var out strings.Builder

	for _, item := range items {
		switch v := item.item.(type) {
		case *Node:
			out.WriteString(v.id)
			writeTypeAndAttrList(&out, v.typ, v.attrs, " ")
		case *Edge:
			out.WriteString(v.from.id)
			out.WriteString(" -")
			didAttrs := writeTypeAndAttrList(&out, v.typ, v.attrs, "")
			if didAttrs {
				out.WriteString("-")
			}
			out.WriteString("> ")
			out.WriteString(v.to.id)
		default:
			panic(fmt.Sprintf("unexpected item type %T in toPlaintext", v))
		}
		out.WriteString("\n")
	}
	return []byte(out.String()), nil
}

type toplevelitem struct {
	item   any
	offset int
}

// Order the graph content by original parse positions, where possible. Put
// newly-added items at the end -- nodes first, then edges.
func inPrintOrder(g *Lilgraph) []toplevelitem {
	items := make([]toplevelitem, 0, len(g.nodes)+len(g.edges))

	for _, n := range g.nodes {
		if len(n.attrs) == 0 && n.typ == "" && n.declPos == nil && (len(n.edgesFrom) > 0 || len(n.edgesTo) > 0) {
			// Doesn't need an explicit node def (because no type and no attrs);
			// Didn't appear in original source (because no declPos);
			// Is referred to in at least one edge;
			// = Don't bother rendering it at all, there's no value to having a
			// declaration in the file.
			continue
		}
		offset := -1
		if n.declPos != nil {
			offset = n.declPos.Offset
		}
		items = append(items, toplevelitem{item: n, offset: offset})
	}
	for _, e := range g.edges {
		offset := -2
		if e.pos != nil {
			offset = e.pos.Offset
		}
		items = append(items, toplevelitem{item: e, offset: offset})
	}

	slices.SortStableFunc(items, func(a, b toplevelitem) int {
		if a.offset < 0 {
			if b.offset < 0 {
				// Both had no AST position. Sort such that -1 (nodes) come
				// before -2 (edges).
				return cmp.Compare(b.offset, a.offset)
			}
			// a had no AST position, b did. Put a after b
			return 1
		}
		if b.offset < 0 {
			// b had no AST position, a did. Put b after a
			return -1
		}
		return cmp.Compare(a.offset, b.offset)
	})
	return items
}

// TODO: break at line length if needed, + indenting
func writeTypeAndAttrList(out *strings.Builder, typ string, attrs []attr, prefix string) bool {
	if typ == "" && len(attrs) == 0 {
		return false
	}
	out.WriteString(prefix + "[" + typ)
	if typ != "" && len(attrs) > 0 {
		out.WriteString("; ")
	}
	kvs := make([]string, 0, len(attrs))

	// Add any remaining attrs that weren't in the original AST
	for _, attr := range attrs {
		kvs = append(kvs, fmt.Sprintf("%s=%s", attr.key, quoteify(attr.value)))
	}

	// TODO: see if running sum of lens would exceed line len, if so, linebreaks + indent
	// for now, jam onto one line
	out.WriteString(strings.Join(kvs, ", "))
	out.WriteString("]")
	return true
}

func quoteify(val string) string {
	if !strings.Contains(val, `"`) && !strings.Contains(val, "\n") {
		return val
	}
	val = strings.ReplaceAll(val, `"`, `\"`)
	return `"` + val + `"`
}
