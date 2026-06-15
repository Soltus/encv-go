// Stage 10 (borrow-nuclear-boy-2026q2)：项目脚手架（README + .gitignore）。
//
// 借鉴自 /tmp/nuclear-boy/file/.../FileOperations.kt L428-605。
//
// 4 种 techStack 模板：
//   - Python
//   - Kotlin
//   - JavaScript
//   - Go
package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TechStack 4 种支持的技术栈。
type TechStack string

const (
	TechStackPython TechStack = "python"
	TechStackKotlin TechStack = "kotlin"
	TechStackJS     TechStack = "javascript"
	TechStackGo     TechStack = "go"
)

// BuildReadme 生成 README.md 模板。
// 借鉴 nuclear-boy FileOperations.kt L428-530。
func BuildReadme(projectName string, stack TechStack) string {
	var sb strings.Builder
	sb.WriteString("# " + projectName + "\n\n")

	switch stack {
	case TechStackPython:
		sb.WriteString("A Python project.\n\n")
		sb.WriteString("## Setup\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("python -m venv .venv\n")
		sb.WriteString("source .venv/bin/activate\n")
		sb.WriteString("pip install -r requirements.txt\n")
		sb.WriteString("```\n\n")
		sb.WriteString("## Run\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("python main.py\n")
		sb.WriteString("```\n")
	case TechStackKotlin:
		sb.WriteString("A Kotlin project (Gradle).\n\n")
		sb.WriteString("## Setup\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("./gradlew build\n")
		sb.WriteString("```\n\n")
		sb.WriteString("## Run\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("./gradlew run\n")
		sb.WriteString("```\n")
	case TechStackJS:
		sb.WriteString("A JavaScript / Node.js project.\n\n")
		sb.WriteString("## Setup\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("npm install\n")
		sb.WriteString("```\n\n")
		sb.WriteString("## Run\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("npm start\n")
		sb.WriteString("```\n")
	case TechStackGo:
		sb.WriteString("A Go project.\n\n")
		sb.WriteString("## Setup\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("go mod download\n")
		sb.WriteString("```\n\n")
		sb.WriteString("## Run\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("go run main.go\n")
		sb.WriteString("```\n\n")
		sb.WriteString("## Test\n\n")
		sb.WriteString("```bash\n")
		sb.WriteString("# ✅ 模块化（沙箱推荐）：单包跑\n")
		sb.WriteString("bash ../../scripts/test-go.sh .\n")
		sb.WriteString("\n")
		sb.WriteString("# ✅ CI 模式：全包跑\n")
		sb.WriteString("ENCV_TEST_FULL=1 bash ../../scripts/test-all-go.sh\n")
		sb.WriteString("```\n")
	default:
		sb.WriteString("(unspecified tech stack)\n")
	}
	return sb.String()
}

// BuildGitignore 生成 .gitignore 模板。
// 借鉴 nuclear-boy FileOperations.kt L533-605。
func BuildGitignore(stack TechStack) string {
	var sb strings.Builder

	switch stack {
	case TechStackPython:
		sb.WriteString("# Python\n")
		sb.WriteString("__pycache__/\n")
		sb.WriteString("*.py[cod]\n")
		sb.WriteString("*$py.class\n")
		sb.WriteString(".venv/\n")
		sb.WriteString("venv/\n")
		sb.WriteString("env/\n")
		sb.WriteString(".pytest_cache/\n")
		sb.WriteString(".mypy_cache/\n")
		sb.WriteString("*.egg-info/\n")
		sb.WriteString("dist/\n")
		sb.WriteString("build/\n")
	case TechStackKotlin:
		sb.WriteString("# Kotlin / Gradle\n")
		sb.WriteString(".gradle/\n")
		sb.WriteString("build/\n")
		sb.WriteString("!gradle/wrapper/gradle-wrapper.jar\n")
		sb.WriteString("*.iml\n")
		sb.WriteString(".idea/\n")
		sb.WriteString("local.properties\n")
		sb.WriteString(".kotlin/\n")
	case TechStackJS:
		sb.WriteString("# Node / JavaScript\n")
		sb.WriteString("node_modules/\n")
		sb.WriteString("dist/\n")
		sb.WriteString("build/\n")
		sb.WriteString(".next/\n")
		sb.WriteString(".nuxt/\n")
		sb.WriteString("coverage/\n")
		sb.WriteString(".env\n")
		sb.WriteString(".env.local\n")
		sb.WriteString("npm-debug.log*\n")
		sb.WriteString("yarn-debug.log*\n")
		sb.WriteString("yarn-error.log*\n")
	case TechStackGo:
		sb.WriteString("# Go\n")
		sb.WriteString("*.exe\n")
		sb.WriteString("*.dll\n")
		sb.WriteString("*.so\n")
		sb.WriteString("*.dylib\n")
		sb.WriteString("vendor/\n")
		sb.WriteString(".env\n")
		sb.WriteString("coverage.out\n")
		sb.WriteString("*.test\n")
		sb.WriteString("*.out\n")
	default:
		sb.WriteString("# Default\n")
		sb.WriteString(".DS_Store\n")
		sb.WriteString("Thumbs.db\n")
	}
	return sb.String()
}

// ScaffoldResult 项目脚手架结果。
type ScaffoldResult struct {
	ProjectName string    `json:"projectName"`
	TechStack   TechStack `json:"techStack"`
	Files       []string  `json:"files"`
}

// ScaffoldProject 在 destDir 创建一个项目骨架（README + .gitignore）。
//
// 借鉴 nuclear-boy FileOperations.kt L428-605。
func ScaffoldProject(destDir, projectName string, stack TechStack) (*ScaffoldResult, error) {
	if projectName == "" {
		return nil, fmt.Errorf("project name required")
	}
	if !IsValidTechStack(stack) {
		return nil, fmt.Errorf("unsupported tech stack: %s", stack)
	}
	// 确保 destDir 存在
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir project: %w", err)
	}

	files := []string{"README.md", ".gitignore"}
	for _, name := range files {
		var content string
		if name == "README.md" {
			content = BuildReadme(projectName, stack)
		} else {
			content = BuildGitignore(stack)
		}
		full := filepath.Join(destDir, name)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			return nil, fmt.Errorf("write %s: %w", name, err)
		}
	}
	return &ScaffoldResult{
		ProjectName: projectName,
		TechStack:   stack,
		Files:       files,
	}, nil
}

// IsValidTechStack 是否为支持的 tech stack。
func IsValidTechStack(stack TechStack) bool {
	switch stack {
	case TechStackPython, TechStackKotlin, TechStackJS, TechStackGo:
		return true
	}
	return false
}
