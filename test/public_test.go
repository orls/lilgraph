package test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/orls/lilgraph"
	"github.com/tidwall/jsonc"
	"golang.org/x/tools/txtar"
)

var testCases fs.FS
var parserCases fs.FS

func TestMain(m *testing.M) {
	testCases = loadTxtarFs("./test_cases.txtar")
	// Re-use the parser cases too (though we're testing ast -> returned objs here)
	parserCases = loadTxtarFs("../internal/test/parser_test_cases.txtar")

	m.Run()
}

func TestReadmeExample(t *testing.T) {
	inputPath := "./readme_example.lilgraph"
	g, err := lilgraph.ParseFile(inputPath)
	if err != nil {
		t.Fatalf("expected readme example to succeed, but got err=%v", err)
	}
	if g == nil {
		t.Fatalf("expected readme example to produce graph obj, but got nil")
	}
}

func TestParserHappyPaths(t *testing.T) {
	// Re-use some of the simple AST parsing happy-path test cases: these ones are all expected to
	// yield valid final graph objs (though note that's not true for all ASTSs).
	cases := []string{
		"happy/simple-nodes.lilgraph",
		"happy/simple-edges.lilgraph",
		"dubious/simple-edges-multiline.lilgraph",
		"happy/edge-attrs.lilgraph",
	}
	for _, inputPath := range cases {
		t.Run(inputPath, func(t *testing.T) {
			input := readFsFile(t, parserCases, inputPath)
			g, err := lilgraph.Parse(input)
			if err != nil {
				t.Fatalf("expected graph build for parser's txtar case '%s' to succeed, but got err=%v", inputPath, err)
			}
			if g == nil {
				t.Fatalf("expected graph build for parser's txtar case '%s' to produce graph, but got  nil", inputPath)
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
