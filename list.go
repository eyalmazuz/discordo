package main

import (
	"fmt"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func main() {
	source := []byte("1. first\n2. second\n3. third\n")
	root := goldmark.New().Parser().Parse(text.NewReader(source))
	
	ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if list, ok := n.(*ast.List); ok {
				fmt.Printf("List node: IsOrdered=%v, Start=%v\n", list.IsOrdered(), list.Start)
			}
		}
		return ast.WalkContinue, nil
	})
}