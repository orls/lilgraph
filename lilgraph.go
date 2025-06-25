package lilgraph

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/orls/lilgraph/internal/ast"
	"github.com/orls/lilgraph/internal/gocc/lexer"
	"github.com/orls/lilgraph/internal/gocc/parser"
	"github.com/orls/lilgraph/internal/gocc/token"
)

var (
	ErrParseFail    = errors.New("failed parsing")
	ErrBadParseType = errors.New("unexpected parser result type")
	ErrTypeChange   = errors.New("nodes cannot be redefined with a different type")
	ErrCyclic       = errors.New("graph is cyclic")
)

func ParseFile(path string) (*Lilgraph, error) {
	lex, err := lexer.NewLexerFile(path)
	if err != nil {
		return nil, err
	}
	return parse(lex)
}

func Parse(bytes []byte) (*Lilgraph, error) {
	return parse(lexer.NewLexer(bytes))
}

func parse(lex *lexer.Lexer) (*Lilgraph, error) {
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

type Attrs map[string]string

type Lilgraph struct {
	Nodes []*Node
	Edges []*Edge
}

type Node struct {
	Id        string
	Type      string
	Attrs     Attrs
	EdgesFrom []*Edge
	EdgesTo   []*Edge

	firstPos    token.Pos
	typeFromPos *token.Pos
}

type Edge struct {
	Type  string
	Attrs Attrs
	From  *Node
	To    *Node

	pos token.Pos
}

type edgeIdentity struct {
	from *Node
	to   *Node
	typ  string
}

func buildFromAst(astGraph *ast.Graph) (*Lilgraph, error) {
	g := &Lilgraph{
		Nodes: []*Node{},
		Edges: []*Edge{},
	}

	lexOrder := []*Node{}
	nodesById := map[string]*Node{}
	edgesById := map[edgeIdentity]*Edge{}
	edgesByFrom := map[*Node][]*Edge{}
	edgesByTo := map[*Node][]*Edge{}

	upsertNodeId := func(id string, pos token.Pos) *Node {
		if n, ok := nodesById[id]; ok {
			return n
		}
		n := &Node{Id: id, firstPos: pos}
		lexOrder = append(lexOrder, n)
		nodesById[id] = n
		return n
	}

	upsertNodeDeclAst := func(astN *ast.Node) error {
		n := upsertNodeId(astN.Id, astN.Pos)
		return updateNode(n, astN)
	}

	for _, rawItem := range astGraph.AstItems {
		switch item := rawItem.(type) {

		case *ast.Node:
			if err := upsertNodeDeclAst(item); err != nil {
				return nil, err
			}

		case *ast.EdgeChain:
			from := upsertNodeId(item.From, item.Pos)
			for _, step := range item.Steps {
				to := upsertNodeId(step.To, step.ToPos)
				edgeId := edgeIdentity{from: from, to: to, typ: step.Type}

				if e, ok := edgesById[edgeId]; ok {
					updateEdge(e, step)
				} else {
					e = &Edge{From: from, To: to, Type: step.Type, pos: step.ArrowPos}
					edgesById[edgeId] = e
					edgesByFrom[from] = append(edgesByFrom[from], e)
					edgesByTo[to] = append(edgesByTo[to], e)
				}

				from = to
			}
		}
	}

	// Tell each node about its commections
	for n, edgesFrom := range edgesByFrom {
		n.EdgesFrom = edgesFrom
	}
	for n, edgesTo := range edgesByTo {
		n.EdgesTo = edgesTo
	}
	g.Nodes = lexOrder
	g.Edges = slices.Collect(maps.Values(edgesById))

	// Force topo order, and hence acyclic too
	if err := lexicalTopoSort(g.Nodes); err != nil {
		return nil, err
	}

	return g, nil
}

func updateNode(n *Node, astN *ast.Node) error {
	if astN.Type != "" {
		if n.Type == "" {
			n.Type = astN.Type
			n.typeFromPos = &astN.Pos
		} else if astN.Type != n.Type {
			return fmt.Errorf(
				"%w: node at %s is redefining '%s' nodes' type from declaration at %s",
				ErrTypeChange,
				astN.Pos,
				astN.Id,
				n.typeFromPos,
			)
		}
	}

	if astN.Attrs != nil {
		if n.Attrs == nil {
			n.Attrs = map[string]string{}
		}
		for k, v := range astN.Attrs {
			n.Attrs[k] = v.Value
		}
	}

	return nil
}

func updateEdge(e *Edge, astStep *ast.EdgeStep) {
	if astStep.Attrs != nil {
		if e.Attrs == nil {
			e.Attrs = map[string]string{}
		}
		for k, v := range astStep.Attrs {
			e.Attrs[k] = v.Value
		}
	}
}

func lexicalTopoSort(nodes []*Node) error {
	ranks := map[*Node]int{}

	var walkDf func(*Node, *Node, int, map[*Node]bool) error
	walkDf = func(s, n *Node, currRank int, path map[*Node]bool) error {
		if path[n] {
			return fmt.Errorf(
				"%w: node '%s' already seen in depth-first walk from '%s'",
				ErrCyclic,
				n.Id,
				s.Id,
			)
		}
		path[n] = true
		ranks[n] = max(ranks[n], currRank)
		for _, e := range n.EdgesFrom {
			if err := walkDf(s, e.To, currRank+1, path); err != nil {
				return err
			}

		}
		delete(path, n)
		return nil
	}
	seenApex := false
	for _, n := range nodes {
		// Only walk from apex nodes
		if len(n.EdgesTo) > 0 {
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
