package codegen

import (
	"os"
	"path/filepath"
	"testing"

	"astra/internal/tools"
)

func TestGatherContext(t *testing.T) {
	dir := t.TempDir()

	// Write a package.json
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0644); err != nil {
		t.Fatal(err)
	}
	// Write a file listed in outputFiles
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main"), 0644); err != nil {
		t.Fatal(err)
	}

	ctx := gatherContext(dir, []string{"main.go", "missing.go"})

	if _, ok := ctx["package.json"]; !ok {
		t.Error("expected package.json in context")
	}
	if _, ok := ctx["main.go"]; !ok {
		t.Error("expected main.go in context")
	}
	if _, ok := ctx["missing.go"]; ok {
		t.Error("missing.go should not be in context (file doesn't exist)")
	}
}

func TestGatherContextEmptyDir(t *testing.T) {
	dir := t.TempDir()
	ctx := gatherContext(dir, nil)
	if len(ctx) != 0 {
		t.Errorf("expected empty context for empty dir, got %v", ctx)
	}
}

func TestWorkspaceScopedRuntimeNoWorkspace(t *testing.T) {
	dir := t.TempDir()
	base := tools.NewWorkspaceRuntime(dir)
	scoped := workspaceScopedRuntime(base, "")
	expected := filepath.Join(base.Root, "_global")
	// Clean both for comparison
	if filepath.Clean(scoped.Root) != filepath.Clean(expected) {
		t.Errorf("root: got %q, want %q", scoped.Root, expected)
	}
}

func TestWorkspaceScopedRuntimeRelativeWorkspace(t *testing.T) {
	dir := t.TempDir()
	base := tools.NewWorkspaceRuntime(dir)
	scoped := workspaceScopedRuntime(base, "my-workspace")
	expected := filepath.Join(base.Root, "_global", "my-workspace")
	if filepath.Clean(scoped.Root) != filepath.Clean(expected) {
		t.Errorf("root: got %q, want %q", scoped.Root, expected)
	}
}

func TestWorkspaceScopedRuntimeAbsoluteWorkspace(t *testing.T) {
	dir := t.TempDir()
	base := tools.NewWorkspaceRuntime(dir)
	absWs := t.TempDir()
	scoped := workspaceScopedRuntime(base, absWs)
	if filepath.Clean(scoped.Root) != filepath.Clean(absWs) {
		t.Errorf("root: got %q, want %q", scoped.Root, absWs)
	}
}

func TestFindBlockEnd(t *testing.T) {
	// content with two blocks
	content := "```ts:a.ts\ncontent_a\n```\n```ts:b.ts\ncontent_b\n```"
	locs := fileBlockRe.FindAllStringSubmatchIndex(content, -1)

	if len(locs) < 2 {
		t.Fatalf("expected at least 2 block locations, got %d", len(locs))
	}

	// End of first block should be before start of second
	end := findBlockEnd(content, locs[0][1], locs, 0)
	if end >= locs[1][0]+len(content) {
		t.Errorf("findBlockEnd for first block: got %d, should be before second block at %d", end, locs[1][0])
	}
}

func TestFindBlockEndLastBlock(t *testing.T) {
	content := "```ts:a.ts\ncontent_a\n```"
	locs := fileBlockRe.FindAllStringSubmatchIndex(content, -1)
	if len(locs) == 0 {
		t.Fatal("no block locations found")
	}
	end := findBlockEnd(content, locs[0][1], locs, 0)
	// Should find the closing ```
	if end >= len(content) {
		t.Errorf("findBlockEnd: got %d, should be less than len(content)=%d", end, len(content))
	}
}

func TestParseFallbackBlocks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantLen int
	}{
		{
			name: "with path comment",
			content: "```typescript\n// src/app.ts\nconst x = 1;\n```",
			wantLen: 1,
		},
		{
			name:    "no path comment",
			content: "```typescript\njust code without path\n```",
			wantLen: 0,
		},
		{
			name:    "empty content",
			content: "",
			wantLen: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFallbackBlocks(tc.content)
			if len(got) != tc.wantLen {
				t.Errorf("parseFallbackBlocks: got %d files, want %d; result: %v", len(got), tc.wantLen, got)
			}
		})
	}
}

func TestExtractPathFromBlock(t *testing.T) {
	tests := []struct {
		name     string
		block    string
		wantPath string
		wantBody bool
	}{
		{
			name:     "with slash path comment",
			block:    "typescript\n// src/components/App.tsx\nconst x = 1;\n",
			wantPath: "src/components/App.tsx",
			wantBody: true,
		},
		{
			name:     "no slash in path",
			block:    "typescript\n// justfilename.ts\nconst x = 1;\n",
			wantPath: "",
		},
		{
			name:     "path with spaces",
			block:    "typescript\n// path with spaces/file.ts\ncode\n",
			wantPath: "",
		},
		{
			name:     "empty block",
			block:    "",
			wantPath: "",
		},
		{
			name:     "single line block",
			block:    "typescript",
			wantPath: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path, body := extractPathFromBlock(tc.block)
			if path != tc.wantPath {
				t.Errorf("path: got %q, want %q", path, tc.wantPath)
			}
			if tc.wantBody && body == "" {
				t.Error("expected non-empty body")
			}
		})
	}
}

func TestDetectLanguageFromTaskAllBranches(t *testing.T) {
	tests := []struct {
		name         string
		instructions string
		outputFiles  []string
		want         string
	}{
		{"csharp .cs", "write code", []string{"Program.cs"}, "C#"},
		{"csharp c# keyword", "write c# code", []string{}, "C#"},
		{"cpp .cpp", "implement", []string{"main.cpp"}, "C++"},
		{"cpp .cc", "build", []string{"lib.cc"}, "C++"},
		{"c .c file", "implement in c .c test", []string{}, "C"},
		{"scala keyword", "write scala code", []string{}, "Scala"},
		{"jsx ext", "write component", []string{"app.jsx"}, "JavaScript"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := TaskPayload{Instructions: tc.instructions, OutputFiles: tc.outputFiles}
			got := detectLanguageFromTask(p)
			if got != tc.want {
				t.Errorf("detectLanguageFromTask = %q, want %q", got, tc.want)
			}
		})
	}
}
