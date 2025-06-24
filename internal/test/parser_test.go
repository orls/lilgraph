package test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/orls/lilgraph/internal/ast"
	"github.com/orls/lilgraph/internal/gocc/lexer"
	"github.com/orls/lilgraph/internal/gocc/parser"
	"github.com/orls/lilgraph/internal/gocc/token"
	"golang.org/x/tools/txtar"
)

var parserTestData fs.FS

func TestMain(m *testing.M) {
	arPath := "parser_test_cases.txtar"
	ar, err := txtar.ParseFile(arPath)
	if err != nil {
		panic(fmt.Errorf("failed loading test-cases data from '%s': %w", arPath, err))
	}
	parserTestData, err = txtar.FS(ar)
	if err != nil {
		panic(fmt.Errorf("failed building in-mem FS from '%s' contents: %w", ar, err))
	}
	m.Run()
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
			lex := lexer.NewLexer(input)
			p := parser.NewParser()
			parseResult, err := p.Parse(lex)
			if err != nil {
				t.Fatalf("expected parsing empty file to succeed, but got err: %v", err)
			}
			g, ok := parseResult.(*ast.Graph)
			if !ok {
				t.Fatalf("expected top-level obj from parser to be an *ast.Graph, but got %T", g)
			}
			if len(g.AstItems) != 0 {
				t.Fatalf("expected parsing empty file to produce empty graph, but got a %d-item graph", len(g.AstItems))
			}
		})
	}
}

func TestParserHappyPaths(t *testing.T) {
	cases := map[string]string{
		"happy/simple-nodes.lilgraph":             "happy/simple-nodes.expected.json",
		"happy/simple-edges.lilgraph":             "happy/simple-edges.expected.json",
		"dubious/simple-edges-multiline.lilgraph": "happy/simple-edges.expected.json",
		"happy/edge-attrs.lilgraph":               "happy/edge-attrs.expected.json",
	}
	for inputPath, expectJsonPath := range cases {
		t.Run(inputPath, func(t *testing.T) {
			input := readFsFile(t, parserTestData, inputPath)
			expectJson := readFsFile(t, parserTestData, expectJsonPath)
			var expectAst *ast.Graph
			if err := json.Unmarshal(expectJson, &expectAst); err != nil {
				t.Fatalf("failed unmarshaling test expectation AST from '%s': %v", expectJsonPath, err)
			}
			lex := lexer.NewLexer(input)
			p := parser.NewParser()
			parseResult, err := p.Parse(lex)
			if err != nil {
				t.Fatalf("expected parsing '%s' to succeed, but got err: %v", inputPath, err)
			}
			g, ok := parseResult.(*ast.Graph)
			if !ok {
				t.Fatalf("expected top-level obj from parser to be an *ast.Graph, but got %T", parseResult)
			}
			if diff := cmp.Diff(expectAst, g, ignorePositions()...); diff != "" {
				t.Errorf("parsing '%s' did not produce expected AST:\n%s", inputPath, diff)
			}
		})
	}
}

func ignorePositions() []cmp.Option {
	return []cmp.Option{
		cmpopts.IgnoreTypes(token.Pos{}),
		// cmpopts.IgnoreFields(ast.AttrPos{}, "KeyNearPos"),
		// cmpopts.IgnoreFields(ast.Node{}, "NearPos"),
		// cmpopts.IgnoreFields(ast.Node{}, "NearPos"),
	}
}

func readFsFile(t *testing.T, fs fs.FS, testdataPath string) []byte {
	t.Helper()
	f, err := fs.Open(testdataPath)
	if err != nil {
		t.Fatalf("failed opening '%s': %v", testdataPath, err)
	}
	bs, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("failed reading '%s': %v", testdataPath, err)
	}
	return bs
}
