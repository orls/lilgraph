package test

import (
	"errors"
	"slices"
	"testing"

	"github.com/orls/lilgraph"
)

// TestGraphMutation focuses on testing the functionality of the in-memory
// abstract graph datastructure used in this lib -- building a branch from
// scratch, and querying/modifying it in various ways. Unlike most other tests,
// this doesn't involve parsing or rendering to the lilgraph format at all.
func TestGraphMutation(t *testing.T) {

	testNodes := map[string]*lilgraph.Node{
		"A": nil,
		"B": nil,
		"C": nil,
		"D": nil,
	}
	g := lilgraph.NewGraph()

	// This is broken into sub-tests for readability, but each depends on the
	// previous one.

	// Seed the graph with four nodes
	if !t.Run("add-nodes", func(t *testing.T) {
		for id := range testNodes {
			n, exists, err := g.AddNode(id, "")
			if err != nil {
				t.Fatalf("expected first AddNode for '%s' to succeed, but got err=%v", id, err)
			}
			testNodes[id] = n
			if exists {
				t.Fatalf("first AddNode call for '%s' reported that node already existed", id)
			}
			if n.Id() != id {
				t.Fatalf("new node should have id '%s', but has '%s'", id, n.Id())
			}
			if n.Type() != "" {
				t.Fatalf("new node '%s' should have no type value, but has '%s'", id, n.Type())
			}
			attrs := n.AttrsMap()
			if len(attrs) != 0 {
				t.Fatalf("new node '%s' should have no attrs, but has %d", id, len(attrs))
			}
			from := slices.Collect(n.EdgesFrom())
			to := slices.Collect(n.EdgesTo())
			if len(from) > 0 || len(to) > 0 {
				t.Fatalf("new node '%s' should have no edges, but has %d", id, len(from)+len(to))
			}
		}

		// Attempt re-adding the same nodes; check the same items are returned.
		for id, n := range testNodes {
			n2, exists, err := g.AddNode(id, "")
			if err != nil {
				t.Fatalf("expected second AddNode for '%s' to succeed, but got err=%v", id, err)
			}
			if !exists {
				t.Fatalf("second AddNode call for '%s' didn't report that node already existed", id)
			}
			if n2 != n {
				t.Fatalf("second AddNode call for '%s' didn't return the original node", id)
			}
		}
	}) {
		return
	}

	// Update one node with a type value; check that it can't be updated again.
	if !t.Run("typed-node", func(t *testing.T) {

		a, _, err := g.AddNode("A", "type1")
		if err != nil {
			t.Fatalf("expected upserting a type value to node A to succeed, but got err=%v", err)
		}
		if a.Type() != "type1" {
			t.Fatalf("expected type of node A to be 'type1' now, but got '%s'", a.Type())
		}
		_, _, err = g.AddNode("A", "type2")
		if !errors.Is(err, lilgraph.ErrTypeChange) {
			t.Fatalf("expected upserting a different type value to node A to fail with ErrTypeChange, but got err=%v", err)
		}
		if a.Type() != "type1" {
			t.Fatalf("expected type of node A to still be 'type1', but got '%s'", a.Type())
		}
		_, _, err = g.AddNode("A", "")
		if err != nil {
			t.Fatalf("expected upserting with no type, to a typed node, to succeed")
		}
		if a.Type() != "type1" {
			t.Fatalf("expected type of node A to still be 'type1', but got '%s'", a.Type())
		}
	}) {
		return
	}

	if !t.Run("edge-basics", func(t *testing.T) {

		e, exists, err := g.AddEdge(testNodes["A"], testNodes["B"], "")
		if err != nil {
			t.Fatalf("expected AddEdge for A->B to succeed, but got err=%v", err)
		}
		if exists {
			t.Fatalf("first AddEdge call for A->B reported that edge already existed")
		}
		if e.Type() != "" {
			t.Fatalf("expected new edge A->B to have no type, but got '%s'", e.Type())
		}
		attrs := e.AttrsMap()
		if len(attrs) != 0 {
			t.Fatalf("expected new edge A->B to have no attrs, but got %d", len(attrs))
		}
		if e.From() != testNodes["A"] || e.To() != testNodes["B"] {
			t.Fatalf("new edge doesn't have expected endpoints: want A->B, got '%s'->'%s'", e.From().Id(), e.To().Id())
		}
		fromA := slices.Collect(testNodes["A"].EdgesFrom())
		toB := slices.Collect(testNodes["B"].EdgesTo())
		if len(fromA) != 1 || len(toB) != 1 || fromA[0] != e || toB[0] != e {
			t.Fatalf("expected new edge A->B to be reflected in edge relevant nodes' edge lists")
		}

		eAgain, exists, err := g.AddEdge(testNodes["A"], testNodes["B"], "")
		if eAgain != e || !exists || err != nil {
			t.Fatalf("expected clean result for second AddEdge call for A->B")
		}

		eSearch, eFound := g.FindEdge(testNodes["A"], testNodes["B"], "")
		if eSearch != e || !eFound {
			t.Fatalf("expected FindEdge to return the untyped A->B edge")
		}
	}) {
		return
	}

	if !t.Run("typed-edge", func(t *testing.T) {
		e2Search, e2Found := g.FindEdge(testNodes["A"], testNodes["B"], "sometype")
		if e2Search != nil || e2Found {
			t.Fatalf("expected FindEdge to return no edge (yet) for A->B with type 'sometype'")
		}

		e2, exists, err := g.AddEdge(testNodes["A"], testNodes["B"], "sometype")
		if e2 == nil || exists || err != nil {
			t.Fatalf("expected clean result for adding typed edge from A->B with type 'sometype'")
		}
		if e2.Type() != "sometype" {
			t.Fatalf("expected new edge A->B to have type 'sometype', but got '%s'", e2.Type())
		}

		e2Search, e2Found = g.FindEdge(testNodes["A"], testNodes["B"], "sometype")
		if e2Search != e2 || !e2Found {
			t.Fatalf("expected FindEdge to return the untyped A->B edge")
		}

		bothSearch := slices.Collect(g.FindEdges(testNodes["A"], testNodes["B"]))
		if len(bothSearch) != 2 {
			t.Fatalf("expected FindEdges to return both A->B edges, but got %d edges", len(bothSearch))
		}

		fromA := slices.Collect(testNodes["A"].EdgesFrom())
		toB := slices.Collect(testNodes["B"].EdgesTo())
		if !slices.Contains(fromA, e2) || !slices.Contains(toB, e2) {
			t.Fatalf("expected new edge A->B to be reflected in edge relevant nodes' edge lists")
		}
	}) {
		return
	}

	if !t.Run("delete-edge", func(t *testing.T) {
		e2, ok := g.FindEdge(testNodes["A"], testNodes["B"], "sometype")
		if e2 == nil || !ok {
			t.FailNow() // exhaustively tested above
		}

		if !g.DeleteEdge(e2) {
			t.Fatalf("expected deleting edge A->B  with type='sometype' to succeed")
		}
		fromA := slices.Collect(testNodes["A"].EdgesFrom())
		toB := slices.Collect(testNodes["B"].EdgesTo())
		if slices.Contains(fromA, e2) || slices.Contains(toB, e2) {
			t.Fatalf("deleted edge is still present in nodes' edge lists")
		}
		if g.DeleteEdge(e2) {
			t.Fatalf("second call to delete edge A->B with type='sometype' should have been a no-op, but returned true")
		}
	}) {
		return
	}

	if !t.Run("delete-node", func(t *testing.T) {
		e3, exists, err := g.AddEdge(testNodes["B"], testNodes["C"], "")
		if e3 == nil || exists || err != nil {
			t.Fatalf("expected clean result for adding edge from B->C")
		}

		if !g.DeleteNode(testNodes["B"]) {
			t.Fatalf("expected deleting node B to succeed")
		}
		if g.DeleteNode(testNodes["B"]) {
			t.Fatalf("second call to delete node B should have been a no=-op,m but returned true")
		}
		if g.Find("B") != nil {
			t.Fatalf("expected node B to be deleted, but it still exists in graph")
		}
		// We still have a pointer to the original node B -- but its edge
		// connections should have been severed.
		fromA := slices.Collect(testNodes["A"].EdgesFrom())
		if len(fromA) != 0 {
			t.Fatalf("expected node A to have no edges after deletion, but it has %d", len(fromA))
		}
		toB := slices.Collect(testNodes["B"].EdgesTo())
		if len(toB) != 0 {
			t.Fatalf("expected node B to have no edges after deleting node B, but it has %d", len(toB))
		}
		allNodes := slices.Collect(g.Nodes())
		if len(allNodes) != 3 {
			t.Fatalf("expected graph to have three remaining nodes after deleting B")
		}
		allEdges := slices.Collect(g.Edges())
		if len(allEdges) != 0 {
			t.Fatalf("expected graph to have no remaining edges after deleting node B, but it has %d", len(allEdges))
		}
	}) {
		return
	}
}
