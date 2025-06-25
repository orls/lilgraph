package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/orls/lilgraph"
	"github.com/orls/lilgraph/internal/ast"
	"github.com/orls/lilgraph/internal/gocc/lexer"
	"github.com/orls/lilgraph/internal/gocc/parser"
	"github.com/orls/lilgraph/internal/gocc/token"
	"github.com/tidwall/jsonc"
	"golang.org/x/tools/txtar"
)

var testCases fs.FS

func TestMain(m *testing.M) {
	testCases = loadTxtarFs("./test_cases.txtar")
	m.Run()
}

func TestReadmeExample(t *testing.T) {
	// Acts as a test of ParseFile public API method, as well as being the README content
	inputPath := "./readme_example.lilgraph"
	g, err := lilgraph.ParseFile(inputPath)
	if err != nil {
		t.Fatalf("expected readme example to succeed, but got err=%v", err)
	}
	if g == nil {
		t.Fatalf("expected readme example to produce graph obj, but got nil")
	}
}

func TestParseEmpty(t *testing.T) {
	cases := map[string][]byte{
		"nilbytes":     nil,
		"emptybytes":   []byte(""),
		"whitespaces":  []byte("  \n\n\t \n \t\n\t"),
		"onlycomments": []byte("\n  // this page\n #intentionally\n\t/*left \nblank\n*/"),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			// Check at internal AST level...
			lex := lexer.NewLexer(input)
			p := parser.NewParser()
			parseResult, err := p.Parse(lex)
			if err != nil {
				t.Fatalf("expected parsing AST from empty file to succeed, but got err: %v", err)
			}
			astG, ok := parseResult.(*ast.Graph)
			if !ok {
				t.Fatalf("expected top-level obj from AST parser to be an *ast.Graph, but got %T", astG)
			}
			if len(astG.AstItems) != 0 {
				t.Fatalf("expected parsing empty file to produce empty AST list, but got %d items", len(astG.AstItems))
			}
			// ..and at post-AST semantics level, via public api
			g, err := lilgraph.Parse(input)
			if err != nil {
				t.Fatalf("expected building graph from empty file to succeed, but got err: %v", err)
			}
			if !ok {
				t.Fatalf("expected top-level obj from public parser to be an *lilgraph.Graph, but got %T", g)
			}
			if len(g.Nodes) != 0 || len(g.Edges) != 0 {
				t.Fatalf("expected parsing empty file to produce empty graph, but got %d nodes/%d edges", len(g.Nodes), len(g.Edges))
			}
		})
	}
}

// TestParserHappyPaths tests examples that should parse successfuly to a know AST, and also then
// produce valid graph structure per chosen semantics.
func TestParserHappyPaths(t *testing.T) {
	cases := map[string]string{
		"happy/simple-nodes.lilgraph":             "happy/simple-nodes.expected.json",
		"happy/simple-edges.lilgraph":             "happy/simple-edges.expected.json",
		"dubious/simple-edges-multiline.lilgraph": "happy/simple-edges.expected.json",
		"happy/edge-attrs.lilgraph":               "happy/edge-attrs.expected.json",
	}
	for inputPath, expectAstJsonPath := range cases {
		t.Run(inputPath, func(t *testing.T) {
			input := readFsFile(t, testCases, inputPath)

			// Check at internal AST level...
			expectJson := readFsFile(t, testCases, expectAstJsonPath)
			var expectAst *ast.Graph
			if err := json.Unmarshal(expectJson, &expectAst); err != nil {
				t.Fatalf("failed unmarshaling test expectation AST from '%s': %v", expectAstJsonPath, err)
			}
			lex := lexer.NewLexer(input)
			p := parser.NewParser()
			parseResult, err := p.Parse(lex)
			if err != nil {
				t.Fatalf("expected parsing '%s' to succeed, but got err: %v", inputPath, err)
			}
			astG, ok := parseResult.(*ast.Graph)
			if !ok {
				t.Fatalf("expected top-level obj from parser to be an *ast.Graph, but got %T", parseResult)
			}
			if diff := cmp.Diff(expectAst, astG, ignorePositions()...); diff != "" {
				t.Errorf("parsing '%s' did not produce expected AST:\n%s", inputPath, diff)
			}

			// ..and at post-AST semantics level, via public api
			g, err := lilgraph.Parse(input)
			if err != nil {
				t.Fatalf("expected public parse to succeed, but got err=%v", err)
			}
			if g == nil {
				t.Fatalf("expected public parse to produce graph, but got nil")
			}
		})
	}
}

func TestNodeAttrs(t *testing.T) {
	inputPath := "happy/node-attrs.lilgraph"
	input := readFsFile(t, testCases, inputPath)
	g, err := lilgraph.Parse(input)
	if err != nil {
		t.Fatalf("expected node-attr example to succeed, but got err=%v", err)
	}
	if g == nil {
		t.Fatalf("expected node-attr example to produce graph obj, but got nil")
	}

	for i, id := range []string{"A", "B", "C", "D"} {
		n := g.Nodes[i]
		if n == nil {
			t.Fatalf("expected node '%s' to exist in graph, but it does not", id)
		}
		if n.Id != id {
			t.Fatalf("expected node '%s' to have id '%s', but got '%s'", id, id, n.Id)
		}
		if n.Type != "sometype" {
			t.Fatalf("expected node '%s' to have type='sometype', but got '%s'", id, n.Type)
		}
	}

	dNode := g.Nodes[3]
	if dNode.Attrs["foo"] != "fooval" {
		t.Fatalf("node D attr 'foo' did not match expectation")
	}
	if dNode.Attrs["bar"] != "barval" {
		t.Fatalf("node D attr 'foo' did not match expectation")
	}

	expectMultilineVal := `This is a quoted string that
spans
multiple
lines, but no quotes.`

	expectFancyVal := `This is a quoted string that also
spans multiple lines.
It also has some leading/trailing whitespace:
    <- 4 spaces ->    
	<- 1 tab ->	
It can contain "double-quotes" (if escaped) and 'single quotes' and backticks ` + "``" + `
and literal backslashes (¯\_(ツ)_/¯) (incl doubled: \\)
and unicode もしもし
and emoji 🥳`
	if diff := cmp.Diff(expectMultilineVal, dNode.Attrs["multiline"]); diff != "" {
		t.Fatalf("node D attr 'multiline' did not match expectation:\n%s", diff)
	}
	if diff := cmp.Diff(expectFancyVal, dNode.Attrs["fancy"]); diff != "" {
		t.Fatalf("node D attr 'fancy' did not match expectation:\n%s", diff)
	}
}

// Checks for a bug where a line-comment that ended in EOF (note: not cases ending in newline
// *then* EOF) caused a parse err.
func TestCommentAtEOF(t *testing.T) {
	// (By definition, can't express these in txtar data)
	cases := []string{
		`// To check for a bug, this comment is right at EOF`,
		`# To check for a bug, this comment is right at EOF`,
		`/*To check for a bug, this comment is right at EOF*/`,
		`foo // To check for a bug, this comment is right at EOF`,
		`foo # To check for a bug, this comment is right at EOF`,
		`foo /*To check for a bug, this comment is right at EOF*/`,
		`foo
		bar -> baz // To check for a bug, this comment is right at EOF`,
		`foo
		bar -> baz # To check for a bug, this comment is right at EOF`,
		`foo
		bar -> baz /*To check for a bug, this comment is right at EOF*/`,
	}

	for i, inputStr := range cases {
		t.Run(fmt.Sprintf("case#%d", i), func(t *testing.T) {
			_, err := lilgraph.Parse([]byte(inputStr))
			if err != nil {
				t.Fatalf("expected comment-at-eof case %d to succeed, but got err=%v", i, err)
			}
		})
	}
}

func TestCycleDetection(t *testing.T) {
	cases := []string{
		"bad/cyclic-1.lilgraph",
		"bad/cyclic-2.lilgraph",
		"bad/cyclic-3.lilgraph",
		"bad/cyclic-4.lilgraph",
	}
	for _, inputPath := range cases {
		t.Run(inputPath, func(t *testing.T) {
			input := readFsFile(t, testCases, inputPath)
			_, err := lilgraph.Parse(input)
			if !errors.Is(err, lilgraph.ErrCyclic) {
				t.Fatalf("expected graph build for %s to fail with ErrCyclic, but instead saw err=%v", inputPath, err)
			}
		})
	}
}

func TestTopoOrder(t *testing.T) {
	cases := map[string]string{
		"happy/multirank.lilgraph": "happy/multirank.expected-order.json",
	}
	for inputPath, expectationPath := range cases {
		t.Run(inputPath, func(t *testing.T) {
			input := readFsFile(t, testCases, inputPath)

			expectJson := readFsFile(t, testCases, expectationPath)
			var expected []string
			if err := json.Unmarshal(jsonc.ToJSON(expectJson), &expected); err != nil {
				t.Fatalf("failed unmarshaling test expectation AST from '%s': %v", expectationPath, err)
			}

			g, err := lilgraph.Parse(input)
			if err != nil {
				t.Fatalf("expected graph build for '%s' to succeed, but got err=%v", inputPath, err)
			}

			actual := []string{}
			for _, n := range g.Nodes {
				actual = append(actual, n.Id)
			}
			if diff := cmp.Diff(expected, actual); diff != "" {
				t.Fatalf("wrong sort order for '%s':\n%s", inputPath, diff)
			}
		})
	}
}

func ignorePositions() []cmp.Option {
	return []cmp.Option{
		cmpopts.IgnoreTypes(token.Pos{}),
	}
}

func loadTxtarFs(arPath string) fs.FS {
	ar, err := txtar.ParseFile(arPath)
	if err != nil {
		panic(fmt.Errorf("failed loading test-cases data from '%s': %w", arPath, err))
	}
	arFs, err := txtar.FS(ar)
	if err != nil {
		panic(fmt.Errorf("failed building in-mem FS from '%s' contents: %w", arPath, err))
	}
	return arFs
}

func readFsFile(t *testing.T, fs fs.FS, testCasesPath string) []byte {
	t.Helper()
	f, err := fs.Open(testCasesPath)
	if err != nil {
		t.Fatalf("failed opening '%s': %v", testCasesPath, err)
	}
	bs, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed reading '%s': %v", testCasesPath, err)
	}
	return bs
}
