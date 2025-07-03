package lilgraph

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/orls/lilgraph/internal/ast"
	"github.com/orls/lilgraph/internal/gocc/lexer"
	"github.com/orls/lilgraph/internal/gocc/parser"
	"github.com/orls/lilgraph/internal/gocc/token"
)

var (
	ErrInvalidId    = errors.New("invalid node id")
	ErrParseFail    = errors.New("failed parsing")
	ErrLoop         = errors.New("cannot create edge from a node to itself")
	ErrBadParseType = errors.New("unexpected parser result type")
	ErrTypeChange   = errors.New("nodes cannot be redefined with a different type")
	ErrTypeInAttrs  = errors.New("attributes called 'type' aren't allowed to avoid ambiguity")
	ErrCyclic       = errors.New("graph is cyclic")
)

func ParseFile(path string) (*Lilgraph, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lexCtx := &lexer.SourceContext{Filepath: path}
	return parse(src, lexCtx)
}

func Parse(src []byte) (*Lilgraph, error) {
	return parse(src, nil)
}

func parse(src []byte, lexCtx token.Context) (*Lilgraph, error) {
	// Comments at end, without a trailing newline, can cause errs. I'm not
	// smart enough to figure out the true way to express "newline or EOF" in
	// the grammar, so... hack it, by tacking on a newline if needed.
	if !bytes.HasSuffix(src, []byte("\n")) {
		src = append(src, byte('\n'))
	}
	lex := lexer.NewLexer(src)
	lex.Context = lexCtx
	p := parser.NewParser()
	rawAst, err := p.Parse(lex)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseFail, err)
	}
	astGraph, ok := rawAst.(*ast.Graph)
	if !ok {
		return nil, fmt.Errorf("%w: expected *ast.Graph, got %T", ErrBadParseType, rawAst)
	}
	return buildFromAst(astGraph)
}

type Lilgraph struct {
	nodes []*Node
	edges []*Edge

	nodesById map[string]*Node
	edgesById map[edgeIdentity]*Edge
}

func NewGraph() *Lilgraph {
	return &Lilgraph{
		nodes:     []*Node{},
		edges:     []*Edge{},
		nodesById: map[string]*Node{},
		edgesById: map[edgeIdentity]*Edge{},
	}
}

func (g *Lilgraph) SortTopo() error {
	return lexicalTopoSort(g.nodes)
}

func (g *Lilgraph) Find(id string) *Node {
	return g.nodesById[id]
}

func (g *Lilgraph) Nodes() iter.Seq[*Node] {
	return slices.Values(g.nodes)
}

func (g *Lilgraph) Edges() iter.Seq[*Edge] {
	return slices.Values(g.edges)
}

// NOTE: this must match the equivalent grammar settings.
var idRegexp = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func (g *Lilgraph) AddNode(id string, typ string) (*Node, bool, error) {
	if !idRegexp.MatchString(id) {
		return nil, false, fmt.Errorf("%w %s", ErrInvalidId, id)
	}
	if n, ok := g.nodesById[id]; ok {
		if typ != "" {
			if n.typ != "" && n.typ != typ {
				return nil, false, fmt.Errorf(
					"%w: node '%s' already has type '%s'",
					ErrTypeChange,
					n.id,
					n.typ,
				)
			}
			n.typ = typ
		}
		return n, true, nil
	}
	n := &Node{id: id, common: common{typ: typ}}
	g.nodes = append(g.nodes, n)
	g.nodesById[id] = n
	return n, false, nil
}

func (g *Lilgraph) FindEdges(from *Node, to *Node) iter.Seq[*Edge] {
	return func(yield func(*Edge) bool) {
		for _, e := range from.edgesFrom {
			if e.to == to {
				if !yield(e) {
					return
				}
			}
		}
	}
}

func (g *Lilgraph) FindEdge(from *Node, to *Node, edgeType string) (*Edge, bool) {
	e, ok := g.edgesById[edgeIdentity{from, to, edgeType}]
	return e, ok
}

func (g *Lilgraph) AddEdge(from *Node, to *Node, edgeType string) (*Edge, bool, error) {
	if from == to {
		return nil, false, ErrLoop
	}
	id := edgeIdentity{from, to, edgeType}
	e, ok := g.edgesById[id]
	if !ok {
		e = &Edge{from: from, to: to, common: common{typ: edgeType}}
		g.edges = append(g.edges, e)
		g.edgesById[id] = e
		from.edgesFrom = append(from.edgesFrom, e)
		to.edgesTo = append(to.edgesTo, e)
	}
	return e, ok, nil
}

func (g *Lilgraph) DeleteNode(n *Node) bool {
	i := slices.Index(g.nodes, n)
	if i < 0 {
		return false
	}
	g.nodes = slices.Delete(g.nodes, i, i+1)
	delete(g.nodesById, n.id)
	for i, e := range n.edgesFrom {
		e.from = nil
		n.edgesFrom[i] = nil
		g.deleteEdge(e, false, true)
	}
	for i, e := range n.edgesTo {
		e.to = nil
		n.edgesTo[i] = nil
		g.deleteEdge(e, true, false)
	}
	n.edgesFrom = nil
	n.edgesTo = nil
	return true
}

func (g *Lilgraph) DeleteEdge(e *Edge) bool {
	return g.deleteEdge(e, true, true)
}

func (g *Lilgraph) deleteEdge(e *Edge, purgeFrom, purgeTo bool) bool {
	i := slices.Index(g.edges, e)
	if i < 0 {
		return false
	}
	g.edges = slices.Delete(g.edges, i, i+1)
	delete(g.edgesById, edgeIdentity{from: e.from, to: e.to, typ: e.typ})
	delFn := func(other *Edge) bool { return e == other }
	if purgeFrom {
		e.from.edgesFrom = slices.DeleteFunc(e.from.edgesFrom, delFn)
	}
	if purgeTo {
		e.to.edgesTo = slices.DeleteFunc(e.to.edgesTo, delFn)
	}
	return true
}

func (g *Lilgraph) MarshalText() (text []byte, err error) {
	return marshalText(g)
}

type Node struct {
	common

	id        string
	edgesFrom []*Edge
	edgesTo   []*Edge

	// AST parser metadata about location of node's first freestanding
	// declaration (if any). Nodes that are only everdeclared by an edge chain
	// will not have this set.
	declPos *token.Pos

	// AST parser metadata about location where this nodes' type was first
	// declared (if any).
	typeFromPos *token.Pos
}

func (n *Node) Id() string                 { return n.id }
func (n *Node) EdgesFrom() iter.Seq[*Edge] { return slices.Values(n.edgesFrom) }
func (n *Node) EdgesTo() iter.Seq[*Edge]   { return slices.Values(n.edgesTo) }

type Edge struct {
	common

	from *Node
	to   *Node

	pos *token.Pos
}

func (e *Edge) Type() string { return e.typ }
func (e *Edge) From() *Node  { return e.from }
func (e *Edge) To() *Node    { return e.to }

type edgeIdentity struct {
	from *Node
	to   *Node
	typ  string
}

type common struct {
	typ   string
	attrs []attr
}

type attr struct {
	key   string
	value string
}

func (c *common) Type() string {
	return c.typ
}

func (c *common) SetAttr(key, value string) error {
	if strings.ToLower(key) == "type" {
		return ErrTypeInAttrs
	}
	if c.attrs == nil {
		c.attrs = []attr{}
	}
	for i, attr := range c.attrs {
		if attr.key == key {
			c.attrs[i].value = value
			return nil
		}
	}
	c.attrs = append(c.attrs, attr{key: key, value: value})
	return nil
}

func (c *common) GetAttr(key string) (string, bool) {
	for _, attr := range c.attrs {
		if attr.key == key {
			return attr.value, true
		}
	}
	return "", false
}

func (c *common) DeleteAttr(key string) {
	if c.attrs == nil {
		return
	}
	for i, attr := range c.attrs {
		if attr.key == key {
			c.attrs = slices.Delete(c.attrs, i, i+1)
			return
		}
	}
}

func (c *common) AttrsMap() map[string]string {
	if c.attrs == nil {
		return nil
	}
	m := make(map[string]string, len(c.attrs))
	for _, attr := range c.attrs {
		m[attr.key] = attr.value
	}
	return m
}

func (c *common) ReplaceAttrs(m map[string]string) {
	if c.attrs == nil {
		c.attrs = []attr{}
	}
	// Build new attrs, discarding any unwanted items not in m, but preserving
	// original order for any common keys.
	m = maps.Clone(m)
	newAttrs := make([]attr, 0, len(m))
	for _, attr := range c.attrs {
		if wantVal, ok := m[attr.key]; ok {
			attr.value = wantVal
			newAttrs = append(newAttrs, attr)
		}
		delete(m, attr.key)
	}
	for k, v := range m {
		newAttrs = append(newAttrs, attr{key: k, value: v})
	}
	c.attrs = newAttrs
}

func buildFromAst(astGraph *ast.Graph) (*Lilgraph, error) {
	g := &Lilgraph{
		nodes:     []*Node{},
		edges:     []*Edge{},
		nodesById: map[string]*Node{},
		edgesById: map[edgeIdentity]*Edge{},
	}

	upsertNodeFromAst := func(id string, pos *token.Pos, typ string) (*Node, error) {
		n, _, err := g.AddNode(id, typ)
		if err != nil {
			return nil, err
		}
		if n.declPos == nil {
			n.declPos = pos
		}
		if n.typeFromPos == nil && typ != "" {
			// ...then this is the decl that's first defining the type.
			n.typeFromPos = pos
		}
		return n, nil
	}

	for _, rawItem := range astGraph.AstItems {
		switch item := rawItem.(type) {

		case *ast.Node:
			n, err := upsertNodeFromAst(item.Id, &item.Pos, item.Type)
			if err != nil {
				if errors.Is(err, ErrTypeChange) {
					err = fmt.Errorf(
						"%w: attempted re-declaration to '%s' at %s",
						err,
						item.Type,
						item.Pos,
					)
				}
				return nil, err
			}
			if err := updateAttrs(&n.common, item.Attrs); err != nil {
				if errors.Is(err, ErrTypeInAttrs) {
					return nil, fmt.Errorf("%w; consider using a node type decl", err)
				}
				return nil, err
			}

		case *ast.EdgeChain:
			from, err := upsertNodeFromAst(item.From, nil, "")
			if err != nil {
				return nil, err
			}
			for _, step := range item.Steps {
				to, err := upsertNodeFromAst(step.To, nil, "")
				if err != nil {
					return nil, err
				}
				e, existed, err := g.AddEdge(from, to, step.Type)
				if err != nil {
					if errors.Is(err, ErrLoop) {
						err = fmt.Errorf(
							"%w: edge at %s forms a loop from '%s' to itself",
							ErrLoop,
							step.Pos,
							from.Id(),
						)
					}
					return nil, err
				}
				if !existed {
					e.pos = &step.Pos
				}
				if err := updateAttrs(&e.common, step.Attrs); err != nil {
					if errors.Is(err, ErrTypeInAttrs) {
						return nil, fmt.Errorf("%w; consider using an edge type decl", err)
					}
					return nil, err
				}
				from = to
			}
		}
	}

	return g, nil
}

func updateAttrs(obj *common, astAttrs ast.Attrs) error {
	for _, astAttr := range astAttrs {
		if err := obj.SetAttr(astAttr.Key, astAttr.Value); err != nil {
			if errors.Is(err, ErrTypeInAttrs) {
				return fmt.Errorf("%w (at %s)", err, astAttr.Pos)
			}
			return err
		}
	}
	return nil
}

func lexicalTopoSort(nodes []*Node) error {
	if len(nodes) == 0 {
		return nil
	}
	ranks := map[*Node]int{}

	var walkDf func(*Node, *Node, int, map[*Node]bool) error
	walkDf = func(s, n *Node, currRank int, path map[*Node]bool) error {
		if path[n] {
			return fmt.Errorf(
				"%w: node '%s' already seen in depth-first walk from '%s'",
				ErrCyclic,
				n.id,
				s.id,
			)
		}
		path[n] = true
		ranks[n] = max(ranks[n], currRank)
		for _, e := range n.edgesFrom {
			if err := walkDf(s, e.to, currRank+1, path); err != nil {
				return err
			}
		}
		delete(path, n)
		return nil
	}
	seenApex := false
	for _, n := range nodes {
		// Only walk from apex nodes
		if len(n.edgesTo) > 0 {
			continue
		}
		seenApex = true
		if err := walkDf(n, n, 0, map[*Node]bool{}); err != nil {
			return err
		}
	}
	if !seenApex {
		return fmt.Errorf("%w: failed to find a node without incoming edges", ErrCyclic)
	}
	slices.SortStableFunc(nodes, func(a, b *Node) int {
		return cmp.Compare(ranks[a], ranks[b])
	})
	return nil
}
