//go:build ignore

// scripts/extract_agent_mock_extras.go
// 从原 agent_api.go 提取被遗漏的 mock 相关声明
//
// 输出：internal/server/agent_mock_extras.go
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
)

const srcPath = "internal/server/agent_api.go"

func main() {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, srcPath, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse: %v\n", err)
		os.Exit(1)
	}
	srcBytes, _ := os.ReadFile(srcPath)
	src := string(srcBytes)

	wantNames := map[string]bool{
		"chatMsg":                    true,
		"scenarioPickerEntry":        true,
		"handleMockResume":           true,
		"handleAgentMockPresets":     true,
		"lastUserTextFromLoopMessages": true,
		"classifyAgentToolError":     true,
		"truncateForLog":             true,
	}

	// 提取 import 块（用 ast.File 的位置）
	var impStart, impEnd int
	for _, imp := range f.Imports {
		s := fset.Position(imp.Pos()).Offset
		e := fset.Position(imp.End()).Offset
		if s < impStart || impStart == 0 {
			impStart = s
		}
		if e > impEnd {
			impEnd = e
		}
	}
	// 找 import 块的开始（"import" 关键字）和结束（"）"）
	importKeyword := strings.Index(src, "\nimport ")
	if importKeyword < 0 {
		importKeyword = strings.Index(src, "import ")
	}
	if importKeyword < 0 {
		fmt.Fprintln(os.Stderr, "no import block found")
		os.Exit(1)
	}
	// 调整到 'i' 字符
	for importKeyword < len(src) && src[importKeyword] != 'i' {
		importKeyword++
	}
	// 找匹配的 ) — 假设 import ( ... ) 形式
	parenStart := strings.Index(src[importKeyword:], "(")
	if parenStart < 0 {
		fmt.Fprintln(os.Stderr, "not ( ... ) import block")
		os.Exit(1)
	}
	absParenStart := importKeyword + parenStart
	depth := 0
	parenEnd := -1
	for i := absParenStart; i < len(src); i++ {
		if src[i] == '(' {
			depth++
		} else if src[i] == ')' {
			depth--
			if depth == 0 {
				parenEnd = i
				break
			}
		}
	}
	if parenEnd < 0 {
		fmt.Fprintln(os.Stderr, "no closing )")
		os.Exit(1)
	}
	importBlock := src[importKeyword : parenEnd+1]
	_ = impStart
	_ = impEnd

	pkgDecl := fmt.Sprintf("package %s\n", f.Name.Name)

	// 收集 decl
	var bodies []string
	for _, d := range f.Decls {
		name := declName(d)
		if name == "" || !wantNames[name] {
			continue
		}
		s := fset.Position(d.Pos()).Offset
		e := fset.Position(d.End()).Offset
		bodies = append(bodies, src[s:e])
	}

	out := pkgDecl + "\n// agent_mock_extras.go — 从原 agent_api.go 提取的 mock 相关声明\n//（chatMsg / scenarioPickerEntry / handleMockResume / handleAgentMockPresets / lastUserTextFromLoopMessages / classifyAgentToolError / truncateForLog）\n\n" + importBlock + "\n\n" + strings.Join(bodies, "\n\n") + "\n"
	if err := os.WriteFile("internal/server/agent_mock_extras.go", []byte(out), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  agent_mock_extras.go  %d decls  %d lines\n", len(bodies), strings.Count(out, "\n"))
}

func declName(d ast.Decl) string {
	switch d := d.(type) {
	case *ast.FuncDecl:
		return d.Name.Name
	case *ast.GenDecl:
		if d.Tok == token.IMPORT {
			return ""
		}
		if len(d.Specs) == 0 {
			return ""
		}
		switch s := d.Specs[0].(type) {
		case *ast.TypeSpec:
			return s.Name.Name
		case *ast.ValueSpec:
			if len(s.Names) > 0 {
				return s.Names[0].Name
			}
		}
	}
	return ""
}
