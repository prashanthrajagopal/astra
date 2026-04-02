package codegen

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTaskPayloadSerialization(t *testing.T) {
	tests := []struct {
		name    string
		payload TaskPayload
	}{
		{
			name: "full payload",
			payload: TaskPayload{
				Description:  "Generate a user service",
				Instructions: "Write a REST API for users",
				OutputFiles:  []string{"src/user.go", "src/user_test.go"},
				Workspace:    "my-workspace",
				AgentContext: json.RawMessage(`{"system_prompt":"be helpful"}`),
			},
		},
		{
			name: "minimal payload",
			payload: TaskPayload{
				Description:  "hello",
				Instructions: "do something",
			},
		},
		{
			name:    "zero value",
			payload: TaskPayload{},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			var got TaskPayload
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if got.Description != tc.payload.Description {
				t.Errorf("Description: got %q, want %q", got.Description, tc.payload.Description)
			}
			if got.Instructions != tc.payload.Instructions {
				t.Errorf("Instructions: got %q, want %q", got.Instructions, tc.payload.Instructions)
			}
			if got.Workspace != tc.payload.Workspace {
				t.Errorf("Workspace: got %q, want %q", got.Workspace, tc.payload.Workspace)
			}
			if len(got.OutputFiles) != len(tc.payload.OutputFiles) {
				t.Errorf("OutputFiles len: got %d, want %d", len(got.OutputFiles), len(tc.payload.OutputFiles))
			}
		})
	}
}

func TestResultSerialization(t *testing.T) {
	r := Result{
		FilesWritten: []string{"main.go", "handler.go"},
		GeneratedFiles: []GeneratedFile{
			{Path: "main.go", Content: "package main"},
		},
		LLMModel:    "gpt-4",
		TokensIn:    100,
		TokensOut:   200,
		ArtifactURI: "gs://bucket/artifact.json",
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	var got Result
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got.LLMModel != r.LLMModel {
		t.Errorf("LLMModel: got %q, want %q", got.LLMModel, r.LLMModel)
	}
	if got.TokensIn != r.TokensIn {
		t.Errorf("TokensIn: got %d, want %d", got.TokensIn, r.TokensIn)
	}
	if got.TokensOut != r.TokensOut {
		t.Errorf("TokensOut: got %d, want %d", got.TokensOut, r.TokensOut)
	}
	if len(got.FilesWritten) != 2 {
		t.Errorf("FilesWritten len: got %d, want 2", len(got.FilesWritten))
	}
	if len(got.GeneratedFiles) != 1 {
		t.Errorf("GeneratedFiles len: got %d, want 1", len(got.GeneratedFiles))
	}
}

func TestGeneratedFileSerialization(t *testing.T) {
	gf := GeneratedFile{Path: "src/app.ts", Content: "const x = 1;"}
	data, err := json.Marshal(gf)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got GeneratedFile
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Path != gf.Path {
		t.Errorf("Path: got %q, want %q", got.Path, gf.Path)
	}
	if got.Content != gf.Content {
		t.Errorf("Content: got %q, want %q", got.Content, gf.Content)
	}
}

func TestAgentContextSerialization(t *testing.T) {
	content := "follow these rules"
	ac := AgentContext{
		SystemPrompt: "be helpful",
		Rules:        []AgentDocument{{Name: "rule1", Content: &content}},
		Skills:       []AgentDocument{{Name: "skill1"}},
		ContextDocs:  []AgentDocument{{Name: "doc1", Content: &content}},
	}
	data, err := json.Marshal(ac)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got AgentContext
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.SystemPrompt != ac.SystemPrompt {
		t.Errorf("SystemPrompt: got %q, want %q", got.SystemPrompt, ac.SystemPrompt)
	}
	if len(got.Rules) != 1 {
		t.Fatalf("Rules len: got %d, want 1", len(got.Rules))
	}
	if got.Rules[0].Name != "rule1" {
		t.Errorf("Rules[0].Name: got %q, want rule1", got.Rules[0].Name)
	}
}

func TestDetectLanguageFromTask(t *testing.T) {
	tests := []struct {
		name         string
		instructions string
		outputFiles  []string
		wantLang     string
	}{
		{"python by ext", "generate script", []string{"main.py"}, "Python"},
		{"python by keyword", "write a python script", []string{}, "Python"},
		{"go by ext", "implement", []string{"main.go"}, "Go"},
		{"go by keyword", "write golang code", []string{}, "Go"},
		{"typescript by ext", "create component", []string{"app.tsx"}, "TypeScript with React and Next.js"},
		{"typescript ts ext", "create types", []string{"types.ts"}, "TypeScript with React and Next.js"},
		{"javascript by ext", "build module", []string{"index.js"}, "JavaScript"},
		{"rust by keyword", "write rust code", []string{}, "Rust"},
		{"rust by ext", "implement", []string{"main.rs"}, "Rust"},
		{"ruby by ext", "build a script", []string{"script.rb"}, "Ruby"},
		{"java by ext", "create service", []string{"Service.java"}, "Java"},
		{"shell by keyword", "write a bash script", []string{}, "Bash/Shell"},
		{"default no match", "do something complex", []string{}, "Python"},
		{"php by ext", "create handler", []string{"handler.php"}, "PHP"},
		{"swift by keyword", "write swift code", []string{}, "Swift"},
		{"kotlin by ext", "build feature", []string{"Main.kt"}, "Kotlin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := TaskPayload{Instructions: tc.instructions, OutputFiles: tc.outputFiles}
			got := detectLanguageFromTask(p)
			if got != tc.wantLang {
				t.Errorf("detectLanguageFromTask(%q, %v) = %q, want %q", tc.instructions, tc.outputFiles, got, tc.wantLang)
			}
		})
	}
}

func TestParseFileBlocks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantLen int
		wantKey string
	}{
		{
			name:    "single ts block",
			content: "```ts:src/main.go\npackage main\n\nfunc main() {}\n```",
			wantLen: 1,
			wantKey: "src/main.go",
		},
		{
			name:    "no blocks",
			content: "here is some text without any code blocks",
			wantLen: 0,
		},
		{
			name:    "typescript block",
			content: "```ts:src/app.tsx\nconst x = 1;\n```",
			wantLen: 1,
			wantKey: "src/app.tsx",
		},
		{
			name:    "empty content",
			content: "",
			wantLen: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseFileBlocks(tc.content)
			if len(got) != tc.wantLen {
				t.Errorf("parseFileBlocks: len = %d, want %d; got %v", len(got), tc.wantLen, got)
			}
			if tc.wantKey != "" {
				if _, ok := got[tc.wantKey]; !ok {
					t.Errorf("expected key %q in result %v", tc.wantKey, got)
				}
			}
		})
	}
}

func TestParseLooseContent(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		outputFiles []string
		wantNil     bool
		wantContent string
	}{
		{
			name:        "single output file plain text",
			content:     "package main\n\nfunc main() {}",
			outputFiles: []string{"main.go"},
			wantNil:     false,
			wantContent: "package main\n\nfunc main() {}",
		},
		{
			name:        "single output file with backtick wrapper",
			content:     "```go\npackage main\n```",
			outputFiles: []string{"main.go"},
			wantNil:     false,
		},
		{
			name:        "multiple output files returns nil",
			content:     "some content",
			outputFiles: []string{"a.go", "b.go"},
			wantNil:     true,
		},
		{
			name:        "empty content returns nil",
			content:     "   ",
			outputFiles: []string{"main.go"},
			wantNil:     true,
		},
		{
			name:        "no output files returns nil",
			content:     "some content",
			outputFiles: []string{},
			wantNil:     true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseLooseContent(tc.content, tc.outputFiles)
			if tc.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil result")
			}
			if tc.wantContent != "" {
				if val := got[tc.outputFiles[0]]; val != tc.wantContent {
					t.Errorf("content: got %q, want %q", val, tc.wantContent)
				}
			}
		})
	}
}

func TestCleanShellCommand(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no prefix", "ls -la", "ls -la"},
		{"Run: prefix", "Run: ls -la", "ls -la"},
		{"run: prefix", "run: echo hello", "echo hello"},
		{"Execute: prefix", "Execute: go build", "go build"},
		{"execute: prefix", "execute: npm install", "npm install"},
		{"Command: prefix", "Command: make test", "make test"},
		{"command: prefix", "command: docker ps", "docker ps"},
		{"dollar prefix", "$ git status", "git status"},
		{"multiline takes first", "echo hello\necho world", "echo hello"},
		{"whitespace trimmed", "  ls  ", "ls"},
		{"empty", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanShellCommand(tc.input)
			if got != tc.want {
				t.Errorf("cleanShellCommand(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		maxLen int
		want   string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"exactly max", "hello", 5, "hello"},
		{"longer than max", "hello world", 5, "hello\n... (truncated)"},
		{"empty string", "", 10, ""},
		{"zero max", "hi", 0, "\n... (truncated)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := truncate(tc.input, tc.maxLen)
			if got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.input, tc.maxLen, got, tc.want)
			}
		})
	}
}

func TestBuildPromptContainsKeyParts(t *testing.T) {
	payload := TaskPayload{
		Instructions: "Build a user service",
		OutputFiles:  []string{"user.go"},
		Workspace:    "ws1",
	}
	prompt := buildPrompt(payload, nil, nil)

	mustContain := []string{
		"senior full-stack developer",
		"Build a user service",
		"user.go",
	}
	for _, s := range mustContain {
		if !strings.Contains(prompt, s) {
			t.Errorf("prompt missing %q", s)
		}
	}
}

func TestBuildPromptWithAgentContext(t *testing.T) {
	systemPrompt := "You are a Go expert."
	ruleContent := "Always use error wrapping."
	skillContent := "Use slog for logging."
	ctxContent := "Project uses pgx."

	ac := &AgentContext{
		SystemPrompt: systemPrompt,
		Rules:        []AgentDocument{{Name: "r1", Content: &ruleContent}},
		Skills:       []AgentDocument{{Name: "s1", Content: &skillContent}},
		ContextDocs:  []AgentDocument{{Name: "c1", Content: &ctxContent}},
	}
	payload := TaskPayload{
		Instructions: "do the thing",
	}
	prompt := buildPrompt(payload, nil, ac)

	for _, s := range []string{systemPrompt, ruleContent, skillContent, ctxContent} {
		if !strings.Contains(prompt, s) {
			t.Errorf("prompt missing %q", s)
		}
	}
}

func TestBuildPromptWithExistingContext(t *testing.T) {
	existing := map[string]string{
		"package.json": `{"name":"myapp"}`,
	}
	payload := TaskPayload{Instructions: "add feature"}
	prompt := buildPrompt(payload, existing, nil)

	if !strings.Contains(prompt, "EXISTING PROJECT FILES") {
		t.Error("prompt missing EXISTING PROJECT FILES section")
	}
	if !strings.Contains(prompt, "package.json") {
		t.Error("prompt missing package.json")
	}
}
