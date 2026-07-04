package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// BuildTarget 定义一个 Go 编译目标
type BuildTarget struct {
	Name       string // 目标名称, 如 "encv"
	SourcePath string // 源码路径, 如 "./cmd/encv"
}

// CopyTask 定义一个文件复制任务
type CopyTask struct {
	Name          string // 任务描述, 如 "User Config"
	SourceRelPath string // 源文件相对路径, 如 "config.user.json"
	DestFileName  string // 目标文件名, 通常与源文件名相同
}

func main() {
	// --- 【用户配置区】 ---
	// 默认的输出目录，可以修改它
	defaultOutputDir := "dist"

	// 在这里定义所有需要构建的目标
	buildTargets := []BuildTarget{
		{Name: "encv", SourcePath: "./cmd/encv"},
	}

	// 在这里定义所有需要复制的文件
	copyTasks := []CopyTask{
		{Name: "User Config", SourceRelPath: "config.user.json", DestFileName: "config.user.json"},
		{Name: "README", SourceRelPath: "README.md", DestFileName: "README.md"},
	}
	// --- 配置区结束 ---

	// 预先执行 schema 构建
	fmt.Println("🔧 Ensuring schema is up-to-date...")
	if err := runGoRun("./cmd/encv-schema"); err != nil {
		fmt.Printf("Error: Failed to build/update schema: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Schema is up-to-date.")

	// 生成 Makefile
	if err := generateMakefile(defaultOutputDir, buildTargets, copyTasks); err != nil {
		fmt.Printf("Error generating Makefile: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ Makefile generated successfully.")

	// 生成 PowerShell 脚本
	if err := generatePowerShellScript(defaultOutputDir, buildTargets, copyTasks); err != nil {
		fmt.Printf("Error generating build.ps1: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ build.ps1 generated successfully.")

	// 【新增】为 Windows 生成 .bat 批处理文件
	if err := generateBatchFile(defaultOutputDir); err != nil {
		fmt.Printf("Error generating build.bat: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✅ build.bat generated successfully.")

	fmt.Println("\n--- All build scripts generated! ---")
	fmt.Printf("Default output directory: %s\n", defaultOutputDir)
	fmt.Println("\nHow to build:")
	if runtime.GOOS == "windows" {
		fmt.Println("  - Double-click: build.bat")
		fmt.Println("  - Or run in PowerShell: .\\build.ps1")
		fmt.Println("  - To change output dir: .\\build.ps1 -OutputDir \"custom/path\"")
	} else {
		fmt.Println("  - In terminal: make build-all")
		fmt.Println("  - To change output dir: OUTPUT_DIR=\"custom/path\" make build-all")
	}
}

// runGoRun 执行 `go run` 命令
func runGoRun(path string) error {
	cmd := exec.Command("go", "run", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// generateMakefile 生成 Makefile 文件
func generateMakefile(outputDir string, targets []BuildTarget, copies []CopyTask) error {
	var sb strings.Builder

	// .PHONY 声明
	sb.WriteString(".PHONY:")
	for _, t := range targets {
		sb.WriteString(" " + t.Name)
	}
	// 【修改】添加新的伪目标
	sb.WriteString(" copy-files build-all build-artifacts run clean\n\n")

	// 定义输出目录变量，允许命令行覆盖
	sb.WriteString(fmt.Sprintf("OUTPUT_DIR ?= %s\n\n", outputDir))

	// clean 目标
	sb.WriteString("# 清理编译产物\nclean:\n")
	sb.WriteString("\t@echo \"Cleaning up...\"\n")
	sb.WriteString("\trm -rf $(OUTPUT_DIR)/\n")

	// 生成每个目标的构建规则
	for _, t := range targets {
		sb.WriteString(fmt.Sprintf("# 编译 %s\n%s:\n", t.Name, t.Name))
		sb.WriteString(fmt.Sprintf("\t@echo \"Building %s...\"\n", t.Name))
		sb.WriteString(fmt.Sprintf("\tgo build -o $(OUTPUT_DIR)/%s %s\n\n", t.Name, t.SourcePath))
	}

	// 生成文件复制规则
	sb.WriteString("# 复制配置和文档文件\ncopy-files:\n")
	sb.WriteString("\t@echo \"Copying necessary files...\"\n")
	sb.WriteString("\t@mkdir -p $(OUTPUT_DIR)\n")
	for _, c := range copies {
		sb.WriteString(fmt.Sprintf("\t@cp %s $(OUTPUT_DIR)/\n", c.SourceRelPath))
	}
	sb.WriteString("\n")

	// build-all 目标
	sb.WriteString("# 编译所有程序并复制文件\nbuild-all:")
	for _, t := range targets {
		sb.WriteString(" " + t.Name)
	}
	sb.WriteString(" copy-files\n")
	sb.WriteString("\t@echo \"All targets and files built successfully in ./$(OUTPUT_DIR)/\"\n")

	return os.WriteFile("Makefile", []byte(sb.String()), 0644)
}

// generatePowerShellScript 生成 build.ps1 文件
func generatePowerShellScript(outputDir string, targets []BuildTarget, copies []CopyTask) error {
	var sb strings.Builder

	// 文件头，定义参数
	sb.WriteString("# Windows 构建脚本\n")
	sb.WriteString("param(\n")
	sb.WriteString(fmt.Sprintf("    [string]$OutputDir = \"%s\",\n", outputDir))
	sb.WriteString("    [bool]$OpenExplorer = $true\n")
	sb.WriteString(")\n\n")

	sb.WriteString("Write-Host \"Starting build process... Output directory: $OutputDir\" -ForegroundColor Green\n")

	// 创建输出目录
	sb.WriteString("# 检查并创建输出目录\n")
	sb.WriteString("if (-not (Test-Path -Path $OutputDir)) {\n")
	sb.WriteString("    Write-Host \"Creating '$OutputDir' directory...\"\n")
	sb.WriteString("    New-Item -ItemType Directory -Force -Path $OutputDir\n")
	sb.WriteString("}\n\n")

	// 【修改】只构建交付产物
	sb.WriteString("# 构建所有交付产物\n")
	for _, t := range targets {
		sb.WriteString(fmt.Sprintf("# 编译 %s\n", t.Name))
		sb.WriteString(fmt.Sprintf("Write-Host \"Building %s.exe...\"\n", t.Name))
		sb.WriteString(fmt.Sprintf("go build -o \"$OutputDir\\%s.exe\" %s\n", t.Name, t.SourcePath))
		sb.WriteString("if ($LASTEXITCODE -ne 0) {\n")
		sb.WriteString(fmt.Sprintf("    Write-Host \"Failed to build %s.exe\" -ForegroundColor Red\n", t.Name))
		sb.WriteString("    exit 1\n")
		sb.WriteString("}\n\n")
	}

	// 生成文件复制命令
	sb.WriteString("# 复制配置和文档文件\n")
	sb.WriteString("Write-Host \"Copying necessary files...\"\n")
	for _, c := range copies {
		sb.WriteString(fmt.Sprintf("Copy-Item -Path \"%s\" -Destination \"$OutputDir\\%s\"\n", c.SourceRelPath, c.DestFileName))
	}
	sb.WriteString("\n")

	// 文件尾
	sb.WriteString("Write-Host \"--------------------------------------------------\" -ForegroundColor Green\n")
	sb.WriteString("Write-Host \"All binaries and files built successfully in ./$OutputDir/\" -ForegroundColor Green\n")
	sb.WriteString("Write-Host \"--------------------------------------------------\" -ForegroundColor Green\n")
	// 【关键修复】检查 $OpenExplorer 布尔值
	sb.WriteString("if ($OpenExplorer) {\n")
	sb.WriteString("    Write-Host \"Opening output folder...\"\n")
	sb.WriteString("    Invoke-Item \"$OutputDir\"\n")
	sb.WriteString("}\n")

	return os.WriteFile("build.ps1", []byte(sb.String()), 0644)
}

// 【新增】generateBatchFile 生成 build.bat 文件
func generateBatchFile(outputDir string) error {
	var sb strings.Builder

	sb.WriteString("@echo off\n")
	sb.WriteString("setlocal enabledelayedexpansion\n\n")
	sb.WriteString("echo Starting build via PowerShell...\n\n")
	// 调用 PowerShell 脚本，并传递 -OutputDir 和 -OpenExplorer 参数
	sb.WriteString(fmt.Sprintf("powershell.exe -ExecutionPolicy Bypass -File \"%%~dp0build.ps1\" -OutputDir \"%s\" -OpenExplorer\n", outputDir))
	sb.WriteString("\n")
	sb.WriteString("echo.\n")
	sb.WriteString("pause\n") // 暂停，以便用户可以看到输出

	return os.WriteFile("build.bat", []byte(sb.String()), 0644)
}
