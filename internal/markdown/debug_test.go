package markdown

import (
	"fmt"
	"testing"
	"github.com/ayn2op/discordo/internal/config"
	"github.com/gdamore/tcell/v3"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestDebugList(t *testing.T) {
	cfg, _ := config.Load("")
	r := NewRenderer(cfg)
	source := []byte("1. first\n2. second\n3. third\n")
	root := goldmark.New().Parser().Parse(text.NewReader(source))

	fmt.Println("Walking root nodes:")
	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			fmt.Printf("Node: %T\n", n)
		}
		return ast.WalkContinue, nil
	})

	r.RenderLines(source, root, tcell.StyleDefault)
}