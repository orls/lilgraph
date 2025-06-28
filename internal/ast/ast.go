package ast

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/orls/lilgraph/internal/gocc/token"
)

// gocc always hands us interface{}. Aliasing here for clarity.
type ParserProduct interface{}

type TopLevel interface {
	TopLevel() // Marker method
}

type Graph struct {
	AstItems []TopLevel `json:"ast_items"`
}

func NewGraph(itemPP ParserProduct) (*Graph, error) {
	g := &Graph{AstItems: []TopLevel{}}
	if itemPP != nil {
		first, ok := itemPP.(TopLevel)
		if !ok {
			return nil, fmt.Errorf("invalid top-level parser product, expected impl of TopLevel iface, but got %T", itemPP)
		}
		g.AstItems = append(g.AstItems, first)
	}
	return g, nil
}

func (g *Graph) UnmarshalJSON(bytes []byte) error {
	// TODO: if Graph gains any other fields, have to duplicate the field definitions and struct
	// tags here, then copy the values over. The pains of polymorphic json in go.
	tmp := &struct {
		RawItemJsons []json.RawMessage `json:"ast_items"`
	}{}
	if err := json.Unmarshal(bytes, tmp); err != nil {
		return err
	}
	if tmp.RawItemJsons == nil {
		g.AstItems = nil
		return nil
	}
	type asttypecheck struct {
		AstType string `json:"ast_type"`
	}
	atc := &asttypecheck{}
	g.AstItems = make([]TopLevel, 0, len(tmp.RawItemJsons))
	for i, rawJson := range tmp.RawItemJsons {
		if err := json.Unmarshal(rawJson, atc); err != nil {
			return err
		}
		var target TopLevel
		switch atc.AstType {
		case "node_def":
			target = &Node{}
		case "edge_chain":
			target = &EdgeChain{}
		default:
			return fmt.Errorf("can't unmarshal json 'ast_items' #%d: unknown ast type '%s'", i, atc.AstType)
		}
		if err := json.Unmarshal(rawJson, target); err != nil {
			return err
		}
		g.AstItems = append(g.AstItems, target)
	}
	return nil
}

func AppendGraphItem(gPP, itemPP ParserProduct) (*Graph, error) {
	g, ok := gPP.(*Graph)
	if !ok {
		return nil, fmt.Errorf("can't append to a non-graph! expected *Graph, but got %T", gPP)
	}
	item, ok := itemPP.(TopLevel)
	if !ok {
		return nil, fmt.Errorf("invalid top-level parser product, expected impl of TopLevel iface, but got %T", itemPP)
	}
	g.AstItems = append(g.AstItems, item)
	return g, nil
}

type Node struct {
	Id    string `json:"id"`
	Type  string `json:"_type,omitempty"`
	Attrs Attrs  `json:"attrs,omitempty"`
	Pos   token.Pos
}

func NewNode(idPP, typePP, attrsPP ParserProduct) (*Node, error) {
	id, pos, err := getTokVal(idPP)
	if err != nil {
		return nil, fmt.Errorf("failed getting value for node id: %v", err)
	}
	typ, err := getTokOrLiteralStr(typePP)
	if err != nil {
		return nil, fmt.Errorf("failed getting value for node type pseudoattr: %v", err)
	}
	node := &Node{
		Id:   id,
		Type: typ,
		Pos:  pos,
	}
	if attrsPP != nil {
		attrs, ok := attrsPP.(Attrs)
		if !ok {
			return nil, fmt.Errorf("expected Attrs instance for node attrs, but got %T", attrsPP)
		}
		node.Attrs = attrs
	}
	return node, nil
}

func (n *Node) TopLevel() {}

func (n *Node) MarshalJson() ([]byte, error) {
	return json.Marshal(&struct {
		AstType string `json:"ast_type"`
		*Node
	}{
		AstType: "node",
		Node:    n,
	})
}

type EdgeChain struct {
	From  string      `json:"from"`
	Steps []*EdgeStep `json:"steps"`
}

func NewEdgeChain(fromPP, stepPP ParserProduct) (*EdgeChain, error) {
	from, _, err := getTokVal(fromPP)
	if err != nil {
		return nil, fmt.Errorf("failed getting value for edge 'from' node id: %v", err)
	}
	step, ok := stepPP.(*EdgeStep)
	if !ok {
		return nil, fmt.Errorf("expected *EdgeStep for edge rhs, but got %T", stepPP)
	}
	return &EdgeChain{
		From:  from,
		Steps: []*EdgeStep{step},
	}, nil
}

func ExtendEdgeChain(chainPP, extendPP ParserProduct) (*EdgeChain, error) {
	chain, ok := chainPP.(*EdgeChain)
	if !ok {
		return nil, fmt.Errorf("can't extend chain; expected *EdgeChain, but got %T", chainPP)
	}
	step, ok := extendPP.(*EdgeStep)
	if !ok {
		return nil, fmt.Errorf("expected *EdgeStep to extend edge chain, but got %T", extendPP)
	}
	chain.Steps = append(chain.Steps, step)
	return chain, nil
}

func (e *EdgeChain) TopLevel() {}

func (e *EdgeChain) MarshalJson() ([]byte, error) {
	return json.Marshal(&struct {
		AstType string `json:"ast_type"`
		*EdgeChain
	}{
		AstType:   "edge_chain",
		EdgeChain: e,
	})
}

type EdgeStep struct {
	To    string
	Type  string
	Attrs Attrs
	Pos   token.Pos
}

func NewEdgeStep(arrowPP, toPP, typePP, attrsPP ParserProduct) (*EdgeStep, error) {
	to, _, err := getTokVal(toPP)
	if err != nil {
		return nil, fmt.Errorf("failed getting value for edge 'to'-node id: %v", err)
	}
	typ, err := getTokOrLiteralStr(typePP)
	if err != nil {
		return nil, fmt.Errorf("failed getting value for edge type pseudoattr: %v", err)
	}
	step := &EdgeStep{
		To:   to,
		Type: typ,
		Pos:  arrowPP.(*token.Token).Pos,
	}
	if attrsPP != nil {
		if attrs, ok := attrsPP.(Attrs); ok {
			step.Attrs = attrs
		} else {
			return nil, fmt.Errorf("expected Attrs instance for edge attrs, but got %T", attrsPP)
		}
	}
	return step, nil
}

type AttrPos struct {
	Value string
	Pos   token.Pos
}

type Attrs map[string]AttrPos

func (a *Attrs) UnmarshalJSON(bytes []byte) error {
	var tmp map[string]string
	if err := json.Unmarshal(bytes, &tmp); err != nil {
		return err
	}
	var newA Attrs
	if tmp != nil {
		newA = Attrs{}
		for k, v := range tmp {
			newA[k] = AttrPos{Value: v}
		}
	}
	*a = newA
	return nil
}

func MergeAttrs(lPP, rPP ParserProduct) (Attrs, error) {
	l, ok := lPP.(Attrs)
	if !ok {
		return nil, fmt.Errorf("can't merge attrs; expected left-hand arg to be Attrs but got %T", lPP)
	}
	r, ok := rPP.(Attrs)
	if !ok {
		return nil, fmt.Errorf("can't merge attrs; expected right-hand arg to be Attrs but got %T", rPP)
	}

	for k, v := range r {
		l[k] = v
	}
	return l, nil
}

func NewAttrs(kPP, vPP ParserProduct) (Attrs, error) {
	k, pos, err := getTokVal(kPP)
	if err != nil {
		return nil, fmt.Errorf("failed getting attr key name: %v", err)
	}
	v, err := getTokOrLiteralStr(vPP)
	if err != nil {
		return nil, fmt.Errorf("failed getting value for attr '%s': %v", k, err)
	}
	// Always use the position metadata of the key, not value.
	return Attrs{k: AttrPos{
		Value: v,
		Pos:   pos,
	}}, nil
}

func Unquote(quotedValPP ParserProduct) (string, error) {
	quotedVal, _, err := getTokVal(quotedValPP)
	if err != nil {
		return "", err
	}
	val := quotedVal[1 : len(quotedVal)-1]
	// un-escape any quotes.
	// (No other escape sequences are supported, to keep things simple)
	return strings.ReplaceAll(val, `\"`, `"`), nil
}

// getTokVal is a util for getting string values from parser tokens. Mostly a noise-saver for type
// assertions. This only works for proper parsed tokens, not cases where the production rule hands
// us a go string from a literal.
func getTokVal(arg ParserProduct) (string, token.Pos, error) {
	tok, ok := arg.(*token.Token)
	if !ok {
		return "", token.Pos{}, fmt.Errorf("expected *token.Token, but got %T", arg)
	}
	return string(tok.Lit), tok.Pos, nil
}

// getTokOrLiteralStr is a util for getting string values in cases where the parser may either
// hand us a string literal (e.g. when it's specified as a literan directly in a BNF production
// rule) or a parsed Token.
func getTokOrLiteralStr(arg ParserProduct) (string, error) {
	var result string
	switch coerced := arg.(type) {
	// if it's a literal "" in the grammar, we get string
	case string:
		result = coerced
	// if it's been parsed as a token then fed to use, we get *token.Token
	case *token.Token:
		result = string(coerced.Lit)
	default:
		return "", fmt.Errorf("want string|*token.Token, but got %T", arg)
	}
	return result, nil
}
