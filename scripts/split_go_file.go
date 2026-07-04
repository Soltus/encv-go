//go:build ignore

// scripts/split_go_file.go
// 通用 Go 文件拆分器：按顶层声明（func/type/const/var）切分到多个子文件
//
// 用法：
//   cd <dir>  &&  go run scripts/split_go_file.go <src> <out1> <names1> [<out2> <names2> ...]
//   names 用逗号分隔
//
// 行为：
//   1. 用 go/parser 解析 AST，识别所有顶层 Decl
//   2. 按 names 列表分配到对应 <outN> 文件
//   3. 未在列表中的声明留在 <src>
//   4. 每个新文件继承原 imports
//   5. 运行后用 goimports 清理
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func mustGetwd() string {
	d, _ := os.Getwd()
	return d
}

type declSpan struct {
	name  string
	start token.Pos
	end   token.Pos
}

func main() {
	if len(os.Args) < 4 || (len(os.Args)-2)%2 != 0 {
		fmt.Fprintln(os.Stderr, "Usage: split_go_file <src> <out1> <names1_csv> [<out2> <names2_csv> ...]")
		os.Exit(1)
	}
	srcPath := os.Args[1]
	abs, _ := filepath.Abs(srcPath)
	fmt.Fprintf(os.Stderr, "srcPath=%s abs=%s cwd=%s\n", srcPath, abs, mustGetwd())
	type group struct {
		out   string
		names map[string]bool
	}
	var groups []group
	for i := 2; i < len(os.Args); i += 2 {
		set := map[string]bool{}
		for _, n := range strings.Split(os.Args[i+1], ",") {
			set[strings.TrimSpace(n)] = true
		}
		groups = append(groups, group{os.Args[i], set})
	}

	// 1) 解析 AST
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, srcPath, nil, parser.ParseComments)
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse %s: %v\n", srcPath, err)
		os.Exit(1)
	}
	srcBytes, err := os.ReadFile(srcPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read %s: %v\n", srcPath, err)
		os.Exit(1)
	}
	src := string(srcBytes)

	// 2) 提取 imports
	importBlock := extractImportBlock(src)

	pkgDecl := fmt.Sprintf("package %s\n", f.Name.Name)

	// 3) 收集 decl spans
	var decls []declSpan
	for _, d := range f.Decls {
		name := declName(d)
		if name == "" {
			continue
		}
		start := fset.Position(d.Pos()).Offset
		end := fset.Position(d.End()).Offset
		decls = append(decls, declSpan{name, token.Pos(start), token.Pos(end)})
	}

	// 4) 路由
	groupBodies := map[string][]string{}
	keptBodies := []string{}
	for _, d := range decls {
		body := src[d.start:d.end]
		matched := false
		for _, g := range groups {
			if g.names[d.name] {
				groupBodies[g.out] = append(groupBodies[g.out], body)
				matched = true
				break
			}
		}
		if !matched {
			keptBodies = append(keptBodies, body)
		}
	}

	// 5) 写每个新文件
	for _, g := range groups {
		bodies := groupBodies[g.out]
		if len(bodies) == 0 {
			continue
		}
		header := fmt.Sprintf("%s\n// %s — 拆分自 %s\n\n%s\n\n",
			pkgDecl, g.out, srcPath, importBlock)
		body := strings.Join(bodies, "\n\n") + "\n"
		if err := os.WriteFile(g.out, []byte(header+body), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "write %s: %v\n", g.out, err)
			os.Exit(1)
		}
		fmt.Printf("  %-50s  %2d decls  %4d lines\n", g.out, len(bodies), strings.Count(header+body, "\n"))
	}

	// 6) 覆盖 <src>
	newHeader := fmt.Sprintf("%s\n// %s — 拆分后保留\n\n%s\n\n", pkgDecl, srcPath, importBlock)
	newBody := ""
	if len(keptBodies) > 0 {
		newBody = strings.Join(keptBodies, "\n\n") + "\n"
	}
	if err := os.WriteFile(srcPath, []byte(newHeader+newBody), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", srcPath, err)
		os.Exit(1)
	}
	fmt.Printf("  %-50s  %2d decls (kept)  %4d lines\n", srcPath, len(keptBodies), strings.Count(newHeader+newBody, "\n"))
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

// extractImportBlock 从 src 里抠出完整的 import ( ... ) 块（或单行 import "x"）
func extractImportBlock(src string) string {
	// 找 "import" 关键字起始
	idx := strings.Index(src, "\nimport ")
	if idx < 0 {
		idx = strings.Index(src, "import ")
		if idx < 0 {
			return ""
		}
	}
	// 跳到 import 后第一个非空白字符
	start := idx
	for start < len(src) && src[start] != 'i' {
		start++
	}
	// 检查是 ( 还是 "x"
	pos := start + len("import")
	for pos < len(src) && (src[pos] == ' ' || src[pos] == '\t') {
		pos++
	}
	if pos < len(src) && src[pos] == '(' {
		// 多行 import ( ... )
		// 找匹配的 )
		depth := 0
		for i := pos; i < len(src); i++ {
			if src[i] == '(' {
				depth++
			} else if src[i] == ')' {
				depth--
				if depth == 0 {
					return strings.TrimRight(src[start:i+1], "\n") + "\n"
				}
			}
		}
		return ""
	}
	// 单行 import "x"
	end := strings.Index(src[pos:], "\n")
	if end < 0 {
		return src[start:] + "\n"
	}
	return src[start : pos+end] + "\n"
}
