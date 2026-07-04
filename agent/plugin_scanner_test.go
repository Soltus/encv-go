package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakePlugin is a hand-rolled PluginAdapter used by the scanner
// and handler tests. The zero value is not usable; tests must
// populate the fields they need.
type fakePlugin struct {
	name         string
	ext          string
	opts         PluginTaskOptions
	canDecryptFn func(string) bool
	encryptFn    func(io.Reader) (*EncryptionResult, error)
	decryptFn    func(ctx context.Context, containerPath, outputDir string) (string, error)
	extraFields  map[string]string
	preEncCalls  int
	postEncCalls int
	encCalls     int
	decCalls     int
}

func (f *fakePlugin) Name() string         { return f.name }
func (f *fakePlugin) GetContainerExtension() string { return f.ext }
func (f *fakePlugin) GetTaskOptions() PluginTaskOptions { return f.opts }
func (f *fakePlugin) SetTaskExtraFields(m map[string]string) {
	f.extraFields = m
}
func (f *fakePlugin) PreEncryptProcessor(ctx context.Context, ip, ird, od string) error {
	f.preEncCalls++
	return nil
}
func (f *fakePlugin) Encrypt(r io.Reader) (*EncryptionResult, error) {
	f.encCalls++
	if f.encryptFn != nil {
		return f.encryptFn(r)
	}
	// Drain the reader to ensure the handler closes it
	// properly.
	_, _ = io.Copy(io.Discard, r)
	return &EncryptionResult{TempPath: "/tmp/fake.encv", EncryptedPayloadSize: 42}, nil
}
func (f *fakePlugin) PostEncryptProcessor(ctx context.Context, r *EncryptionResult) (string, error) {
	f.postEncCalls++
	return "/tmp/fake_output_" + f.name + ".encv", nil
}
func (f *fakePlugin) CanDecrypt(p string) bool {
	if f.canDecryptFn != nil {
		return f.canDecryptFn(p)
	}
	return strings.HasSuffix(p, f.ext)
}
func (f *fakePlugin) PreDecryptProcessor(ctx context.Context, c, o string) error { return nil }
func (f *fakePlugin) Decrypt(ctx context.Context, c, o string) (string, error) {
	f.decCalls++
	if f.decryptFn != nil {
		return f.decryptFn(ctx, c, o)
	}
	return filepath.Join(o, "decrypted_"+filepath.Base(c)), nil
}
func (f *fakePlugin) PostDecryptProcessor(ctx context.Context, c string) error { return nil }

// TestScanPluginTools_SevenPluginsYieldsTwelveTools locks the
// 7-plugin, 12-tool contract from the spec.
func TestScanPluginTools_SevenPluginsYieldsTwelveTools(t *testing.T) {
	plugins := makeSevenPluginSet()
	tools, lookup, err := scanPluginTools(plugins)
	if err != nil {
		t.Fatalf("scanPluginTools: %v", err)
	}
	if len(tools) != 12 {
		t.Fatalf("expected 12 tools (6 plugins * 2 ops), got %d", len(tools))
	}
	// Lookup must contain 12 entries too.
	if len(lookup) != 12 {
		t.Errorf("lookup size: got %d want 12", len(lookup))
	}
	// alistencrypt must NOT appear in the lookup.
	for name := range lookup {
		if name == "alistencrypt_encrypt" || name == "alistencrypt_decrypt" {
			t.Errorf("alistencrypt tools should be skipped, found %q", name)
		}
	}
}

// TestScanPluginTools_AlistEncryptIsSkipped pins the spec rule
// that alistencrypt is exposed via OpenList tools, not as a
// plugin tool.
func TestScanPluginTools_AlistEncryptIsSkipped(t *testing.T) {
	plugins := []PluginAdapter{
		&fakePlugin{name: "video", ext: ".sccgv"},
		&fakePlugin{name: "alistencrypt", ext: ".bin"},
	}
	tools, lookup, err := scanPluginTools(plugins)
	if err != nil {
		t.Fatalf("scanPluginTools: %v", err)
	}
	if len(tools) != 2 {
		t.Errorf("expected 2 tools (video encrypt + decrypt), got %d", len(tools))
	}
	if _, ok := lookup["video_encrypt"]; !ok {
		t.Errorf("video_encrypt missing from lookup")
	}
	if _, ok := lookup["alistencrypt_encrypt"]; ok {
		t.Errorf("alistencrypt should be skipped")
	}
}

// TestScanPluginTools_ToolNames is a contract test: every tool
// emitted by the scanner must follow the
// `<pluginName>_<encrypt|decrypt>` convention.
func TestScanPluginTools_ToolNames(t *testing.T) {
	plugins := makeSevenPluginSet()
	_, lookup, err := scanPluginTools(plugins)
	if err != nil {
		t.Fatalf("scanPluginTools: %v", err)
	}
	for name := range lookup {
		if !strings.HasSuffix(name, "_encrypt") && !strings.HasSuffix(name, "_decrypt") {
			t.Errorf("tool name %q does not follow the suffix convention", name)
		}
	}
}

// TestScanPluginTools_SchemaContainsExpectedFields ensures the
// generated schema declares input_paths, output_path, and
// extra_fields.
func TestScanPluginTools_SchemaContainsExpectedFields(t *testing.T) {
	plugins := []PluginAdapter{
		&fakePlugin{
			name: "video",
			ext:  ".sccgv",
			opts: PluginTaskOptions{
				PasswordStrategy:     PasswordGlobal,
				SupportVersionSelect: true,
				SupportedVersions:    []int{1, 2, 3},
				DefaultVersion:       2,
				ExtraFields: []PluginTaskField{
					{Key: "stream_preset", Type: "select", Options: []string{"a", "b"}, Condition: "encrypt"},
				},
			},
		},
	}
	tools, _, err := scanPluginTools(plugins)
	if err != nil {
		t.Fatalf("scanPluginTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	// Take the encrypt tool.
	schemaMap, ok := tools[0].Schema.(map[string]any)
	if !ok {
		t.Fatalf("schema is %T, want map[string]any", tools[0].Schema)
	}
	fn := schemaMap["function"].(map[string]any)
	params := fn["parameters"].(map[string]any)
	props := params["properties"].(map[string]any)

	if _, ok := props["input_paths"]; !ok {
		t.Errorf("input_paths missing from schema")
	}
	if _, ok := props["output_path"]; !ok {
		t.Errorf("output_path missing from schema")
	}
	if _, ok := props["extra_fields"]; !ok {
		t.Errorf("extra_fields missing from schema")
	}
	// video has PasswordGlobal → password is hidden.
	if _, ok := props["password"]; ok {
		t.Errorf("PasswordGlobal must not surface password field")
	}
	// video has SupportVersionSelect=true → version is required.
	if _, ok := props["version"]; !ok {
		t.Errorf("version should be present when SupportVersionSelect is true")
	}
	required := params["required"].([]string)
	wantReq := map[string]bool{"input_paths": false, "output_path": false, "version": false}
	for _, r := range required {
		if _, ok := wantReq[r]; ok {
			wantReq[r] = true
		}
	}
	for k, seen := range wantReq {
		if !seen {
			t.Errorf("required missing %q", k)
		}
	}
}

// TestScanPluginTools_PasswordIndependentSurfacesPasswordField
// verifies the password-surfacing logic in buildPluginSchema. We
// use a non-skipped plugin name so the scanner emits the tool.
func TestScanPluginTools_PasswordIndependentSurfacesPasswordField(t *testing.T) {
	plugins := []PluginAdapter{
		&fakePlugin{
			name: "custom_aes",
			ext:  ".caes",
			opts: PluginTaskOptions{PasswordStrategy: PasswordIndependent},
		},
	}
	tools, _, err := scanPluginTools(plugins)
	if err != nil {
		t.Fatalf("scanPluginTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	// Check the encrypt tool.
	schema := tools[0].Schema.(map[string]any)
	props := schema["function"].(map[string]any)["parameters"].(map[string]any)["properties"].(map[string]any)
	pwd, ok := props["password"]
	if !ok {
		t.Errorf("PasswordIndependent must surface password field")
		return
	}
	pwdMap, ok := pwd.(map[string]any)
	if !ok {
		t.Fatalf("password prop: %T", pwd)
	}
	if pwdMap["type"] != "string" {
		t.Errorf("password type: got %v", pwdMap["type"])
	}
	required := schema["function"].(map[string]any)["parameters"].(map[string]any)["required"].([]string)
	found := false
	for _, r := range required {
		if r == "password" {
			found = true
		}
	}
	if !found {
		t.Errorf("password should be in required list when PasswordIndependent")
	}
}

// TestScanPluginTools_PasswordNoneOmitsPasswordField exercises
// the third password strategy.
func TestScanPluginTools_PasswordNoneOmitsPasswordField(t *testing.T) {
	plugins := []PluginAdapter{
		&fakePlugin{
			name: "no_pwd",
			ext:  ".np",
			opts: PluginTaskOptions{PasswordStrategy: PasswordNone},
		},
	}
	tools, _, err := scanPluginTools(plugins)
	if err != nil {
		t.Fatalf("scanPluginTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	schema := tools[0].Schema.(map[string]any)
	props := schema["function"].(map[string]any)["parameters"].(map[string]any)["properties"].(map[string]any)
	if _, ok := props["password"]; ok {
		t.Errorf("PasswordNone must not surface password field")
	}
}

// TestScanPluginTools_NoPluginsYieldsError is the error path.
func TestScanPluginTools_NoPluginsYieldsError(t *testing.T) {
	// Provide only the skipped alistencrypt so the registry is
	// "non-empty but no tools" — the scanner still errors.
	plugins := []PluginAdapter{&fakePlugin{name: "alistencrypt", ext: ".bin"}}
	_, _, err := scanPluginTools(plugins)
	if err == nil {
		t.Errorf("expected an error when all plugins are skipped")
	}
	// And truly empty input also errors.
	if _, _, err := scanPluginTools(nil); err == nil {
		t.Errorf("expected an error for empty input")
	}
}

// TestBuildPluginSchema_ExtraFields_AreFilteredByOperation ensures
// the schema only includes extra fields whose Condition matches
// the operation (encrypt / decrypt / both).
func TestBuildPluginSchema_ExtraFields_AreFilteredByOperation(t *testing.T) {
	opts := PluginTaskOptions{
		PasswordStrategy: PasswordGlobal,
		ExtraFields: []PluginTaskField{
			{Key: "enc_only", Type: "bool", Condition: "encrypt"},
			{Key: "dec_only", Type: "bool", Condition: "decrypt"},
			{Key: "both", Type: "string", Condition: "both"},
		},
	}
	enc := buildPluginSchema("video", "encrypt", opts, ".sccgv")
	dec := buildPluginSchema("video", "decrypt", opts, ".sccgv")

	encProps := enc["function"].(map[string]any)["parameters"].(map[string]any)["properties"].(map[string]any)["extra_fields"].(map[string]any)["properties"].(map[string]any)
	decProps := dec["function"].(map[string]any)["parameters"].(map[string]any)["properties"].(map[string]any)["extra_fields"].(map[string]any)["properties"].(map[string]any)

	if _, ok := encProps["enc_only"]; !ok {
		t.Errorf("encrypt schema missing enc_only")
	}
	if _, ok := encProps["dec_only"]; ok {
		t.Errorf("encrypt schema should NOT contain dec_only")
	}
	if _, ok := encProps["both"]; !ok {
		t.Errorf("encrypt schema missing 'both' field")
	}
	if _, ok := decProps["dec_only"]; !ok {
		t.Errorf("decrypt schema missing dec_only")
	}
	if _, ok := decProps["enc_only"]; ok {
		t.Errorf("decrypt schema should NOT contain enc_only")
	}
}

// TestPluginFieldType_FallsBackToString pins the safe default
// for unknown type names.
func TestPluginFieldType_FallsBackToString(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"string", "string"},
		{"password", "string"},
		{"int", "integer"},
		{"bool", "boolean"},
		{"unknown_type", "string"},
	}
	for _, c := range cases {
		if got := pluginFieldType(c.in); got != c.want {
			t.Errorf("pluginFieldType(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestMakePluginEncryptHandler_HappyPath runs the encrypt handler
// end-to-end against a fake plugin and a real temp file.
func TestMakePluginEncryptHandler_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in.txt")
	if err := os.WriteFile(in, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := &fakePlugin{
		name: "text",
		ext:  ".sccgt",
		opts: PluginTaskOptions{PasswordStrategy: PasswordGlobal},
	}
	handler := makePluginEncryptHandler(p)

	args := mustJSONIn(t, map[string]any{
		"input_paths":  []string{in},
		"output_path":  filepath.Join(tmp, "out.sccgt"),
		"extra_fields": map[string]string{"fn_rounds": "8"},
	})
	out, err := handler(args)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var res PluginOutput
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("result json: %v (raw: %s)", err, out)
	}
	if len(res.OutputPaths) != 1 {
		t.Errorf("expected 1 output, got %d", len(res.OutputPaths))
	}
	if res.Operation != "encrypt" {
		t.Errorf("operation: got %q", res.Operation)
	}
	if p.preEncCalls != 1 || p.encCalls != 1 || p.postEncCalls != 1 {
		t.Errorf("pre/encrypt/post counts: %d/%d/%d, want 1/1/1", p.preEncCalls, p.encCalls, p.postEncCalls)
	}
	// Extra fields must have been injected.
	if p.extraFields["fn_rounds"] != "8" {
		t.Errorf("extra fields not injected: %+v", p.extraFields)
	}
}

// TestMakePluginEncryptHandler_InvalidArgs returns a structured
// error when the LLM sends malformed JSON.
func TestMakePluginEncryptHandler_InvalidArgs(t *testing.T) {
	p := &fakePlugin{name: "x", ext: ".x"}
	h := makePluginEncryptHandler(p)
	out, err := h("not json at all")
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if !strings.Contains(out, "invalid_args") {
		t.Errorf("expected invalid_args, got %s", out)
	}
	if p.preEncCalls != 0 || p.encCalls != 0 {
		t.Errorf("handler should not invoke plugin on invalid args")
	}
}

// TestMakePluginEncryptHandler_MissingInputPaths covers the
// spec's "must have input" guard.
func TestMakePluginEncryptHandler_MissingInputPaths(t *testing.T) {
	p := &fakePlugin{name: "x", ext: ".x"}
	h := makePluginEncryptHandler(p)
	out, _ := h(`{"output_path":"/tmp/out"}`)
	if !strings.Contains(out, "missing_input_paths") {
		t.Errorf("expected missing_input_paths, got %s", out)
	}
}

// TestMakePluginEncryptHandler_PluginErrorSurfacesAsErrorJSON
// checks that an Encrypt error is reported as a structured JSON
// result, not as a Go error.
func TestMakePluginEncryptHandler_PluginErrorSurfacesAsErrorJSON(t *testing.T) {
	p := &fakePlugin{
		name: "x",
		ext:  ".x",
		encryptFn: func(r io.Reader) (*EncryptionResult, error) {
			return nil, errors.New("disk full")
		},
	}
	h := makePluginEncryptHandler(p)
	tmp := t.TempDir()
	in := filepath.Join(tmp, "in")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := h(mustJSONIn(t, map[string]any{
		"input_paths": []string{in},
		"output_path": filepath.Join(tmp, "out"),
	}))
	if !strings.Contains(out, "encrypt_failed") || !strings.Contains(out, "disk full") {
		t.Errorf("expected encrypt_failed error, got %s", out)
	}
}

// TestMakePluginDecryptHandler_CanDecryptMismatch checks the
// spec §3.4 self-check: the handler must NOT call Decrypt if the
// plugin reports CanDecrypt=false.
func TestMakePluginDecryptHandler_CanDecryptMismatch(t *testing.T) {
	p := &fakePlugin{
		name: "video",
		ext:  ".sccgv",
		canDecryptFn: func(p string) bool { return false },
	}
	h := makePluginDecryptHandler(p)
	out, _ := h(mustJSONIn(t, map[string]any{
		"input_paths": []string{"/some/path/actually_pdf.encv"},
		"output_path": t.TempDir(),
	}))
	if !strings.Contains(out, "container_format_mismatch") {
		t.Errorf("expected container_format_mismatch, got %s", out)
	}
	if p.decCalls != 0 {
		t.Errorf("Decrypt must not be called when CanDecrypt is false; got %d calls", p.decCalls)
	}
	if !strings.Contains(out, "suggested_tool") {
		t.Errorf("expected suggested_tool in payload, got %s", out)
	}
}

// TestMakePluginDecryptHandler_HappyPath exercises the full
// decrypt flow against a fake plugin.
func TestMakePluginDecryptHandler_HappyPath(t *testing.T) {
	tmp := t.TempDir()
	containerPath := filepath.Join(tmp, "foo.sccgv")
	if err := os.WriteFile(containerPath, []byte("encrypted"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(tmp, "out")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	p := &fakePlugin{
		name: "video",
		ext:  ".sccgv",
	}
	h := makePluginDecryptHandler(p)
	out, err := h(mustJSONIn(t, map[string]any{
		"input_paths": []string{containerPath},
		"output_path": outDir,
	}))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	var res PluginOutput
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("result json: %v (raw: %s)", err, out)
	}
	if res.Operation != "decrypt" {
		t.Errorf("operation: got %q", res.Operation)
	}
	if len(res.OutputPaths) != 1 {
		t.Errorf("expected 1 output, got %d", len(res.OutputPaths))
	}
	if p.decCalls != 1 {
		t.Errorf("Decrypt calls: got %d want 1", p.decCalls)
	}
}

// TestSuggestDecryptTool_KnownExtensions pins the suggestion
// table.
func TestSuggestDecryptTool_KnownExtensions(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"foo.sccgv", "video_decrypt"},
		{"foo.sccga", "audio_decrypt"},
		{"foo.sccgi", "image_decrypt"},
		{"foo.sccgwps", "wps_decrypt"},
		{"foo.sccgpdf", "pdf_decrypt"},
		{"foo.sccgt", "text_decrypt"},
		{"foo.bin", "(use OpenList alist tools, not a plugin decrypt tool)"},
		{"foo.unknownext", ""},
	}
	for _, c := range cases {
		if got := suggestDecryptTool(c.path); got != c.want {
			t.Errorf("suggestDecryptTool(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// makeSevenPluginSet returns the canonical 7-plugin set the
// agent demo registers: video / audio / image / wps / pdf / text
// / alistencrypt. The alistencrypt is intentionally included to
// verify it is skipped.
func makeSevenPluginSet() []PluginAdapter {
	mk := func(n, ext string) PluginAdapter {
		return &fakePlugin{
			name: n,
			ext:  ext,
			opts: PluginTaskOptions{
				PasswordStrategy: PasswordGlobal,
				ExtraFields: []PluginTaskField{
					{Key: "fn_rounds", Type: "select", Options: []string{"4", "8", "12", "16"}, DefaultValue: "8", Condition: "encrypt"},
				},
			},
		}
	}
	return []PluginAdapter{
		mk("video", ".sccgv"),
		mk("audio", ".sccga"),
		mk("image", ".sccgi"),
		mk("wps", ".sccgwps"),
		mk("pdf", ".sccgpdf"),
		mk("text", ".sccgt"),
		&fakePlugin{name: "alistencrypt", ext: ".bin", opts: PluginTaskOptions{PasswordStrategy: PasswordIndependent}},
	}
}
