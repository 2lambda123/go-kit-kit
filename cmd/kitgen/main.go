package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"log"
	"os"

	"github.com/pkg/errors"
)

// go get github.com/nyarly/inlinefiles
//go:generate inlinefiles --package=main --vfs=ASTTemplates ./templates ast_templates.go

func usage() string {
	return fmt.Sprintf("Usage: %s <filename>", os.Args[0])
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal(usage())
	}
	filename := os.Args[1]
	file, err := os.Open(filename)
	if err != nil {
		log.Fatalf("error while opening %q: %v", filename, err)
	}

	buf, err := process(filename, file)
	if err != nil {
		log.Fatal(err)
	}

	io.Copy(os.Stdout, buf)
}

func process(filename string, source io.Reader) (io.Reader, error) {
	f, err := parseFile(filename, source)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing input %q", filename)
	}

	context, err := extractContext(f)
	if err != nil {
		return nil, errors.Wrapf(err, "examining input file %q", filename)
	}

	dest, err := transformAST(context)
	if err != nil {
		return nil, errors.Wrapf(err, "generating AST")
	}

	buf, err := formatNode(dest)
	if err != nil {
		return nil, errors.Wrapf(err, "formatting")
	}
	return buf, nil
}

func parseFile(fname string, source io.Reader) (ast.Node, error) {
	f, err := parser.ParseFile(token.NewFileSet(), fname, source, parser.DeclarationErrors)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func extractContext(f ast.Node) (*sourceContext, error) {
	context := &sourceContext{}
	visitor := &parseVisitor{src: context}

	ast.Walk(visitor, f)

	return context, context.validate()
}

func formatNode(node ast.Node) (*bytes.Buffer, error) {
	outfset := token.NewFileSet()
	buf := &bytes.Buffer{}
	err := format.Node(buf, outfset, node)
	if err != nil {
		return nil, err
	}
	return buf, nil
}