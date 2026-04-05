package main

import (
	"bytes"
	"compress/gzip"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

type parityFixture struct {
	name  string
	level int
	input []byte
}

const helperMain = `package main

import (
	"bytes"
	"flag"
	"io"
	"os"

	"github.com/nijaru/brotli"
)

func main() {
	level := flag.Int("level", 0, "compression level")
	flag.Parse()

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	var out bytes.Buffer
	w := brotli.NewWriterLevel(&out, *level)
	if _, err := w.Write(input); err != nil {
		panic(err)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}

	if _, err := os.Stdout.Write(out.Bytes()); err != nil {
		panic(err)
	}
}
`

func main() {
	baselineDir := flag.String("baseline-dir", getenv("BASELINE_DIR", "/tmp/brotli-baseline-4936706"), "baseline worktree")
	outDir := flag.String("out", "testdata/parity", "output directory")
	flag.Parse()

	root, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	if err := ensureBaselineWorktree(root, *baselineDir); err != nil {
		log.Fatal(err)
	}

	fixtures, err := loadParityFixtures(root)
	if err != nil {
		log.Fatal(err)
	}

	tmpDir, err := os.MkdirTemp("", "brotli-paritygen-*")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := writeHelperModule(tmpDir, *baselineDir); err != nil {
		log.Fatal(err)
	}

	helper := filepath.Join(tmpDir, "paritygen")
	build := exec.Command("go", "build", "-o", helper, ".")
	build.Dir = tmpDir
	build.Env = append(os.Environ(), "GOWORK=off")
	if out, err := build.CombinedOutput(); err != nil {
		log.Fatalf("build helper: %v\n%s", err, out)
	}

	if err := os.RemoveAll(*outDir); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	for _, fx := range fixtures {
		data, err := encodeFixture(helper, fx.level, fx.input)
		if err != nil {
			log.Fatalf("%s/q%d: %v", fx.name, fx.level, err)
		}
		path := filepath.Join(*outDir, fmt.Sprintf("%s-q%d.br", fx.name, fx.level))
		if err := os.WriteFile(path, data, 0o644); err != nil {
			log.Fatalf("write %s: %v", path, err)
		}
	}
}

func ensureBaselineWorktree(root, baselineDir string) error {
	if _, err := os.Stat(filepath.Join(baselineDir, ".git")); err == nil {
		return nil
	}
	if info, err := os.Stat(baselineDir); err == nil {
		if info.IsDir() {
			return fmt.Errorf("baseline path exists but is not a git worktree: %s", baselineDir)
		}
		return fmt.Errorf("baseline path exists but is not a directory: %s", baselineDir)
	}

	if err := os.MkdirAll(filepath.Dir(baselineDir), 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", root, "worktree", "add", "--detach", baselineDir, "493670659b4731f109f5be807aa2062f4ec61668")
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add: %w\n%s", err, out)
	}
	return nil
}

func loadParityFixtures(root string) ([]parityFixture, error) {
	opticks, err := os.ReadFile(filepath.Join(root, "testdata", "Isaac.Newton-Opticks.txt"))
	if err != nil {
		return nil, err
	}
	if len(opticks) > 1<<16 {
		opticks = opticks[:1<<16]
	}

	issue22, err := os.Open(filepath.Join(root, "testdata", "issue22.gz"))
	if err != nil {
		return nil, err
	}
	defer issue22.Close()
	zr, err := gzip.NewReader(issue22)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	issue22Data, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if len(issue22Data) > 1<<16 {
		issue22Data = issue22Data[:1<<16]
	}

	return []parityFixture{
		{name: "empty", level: 0, input: nil},
		{name: "empty", level: 6, input: nil},
		{name: "empty", level: 9, input: nil},
		{name: "empty", level: 11, input: nil},
		{name: "short-text", level: 0, input: []byte("hello world hello world hello world")},
		{name: "short-text", level: 6, input: []byte("hello world hello world hello world")},
		{name: "short-text", level: 9, input: []byte("hello world hello world hello world")},
		{name: "short-text", level: 11, input: []byte("hello world hello world hello world")},
		{name: "html-repeat", level: 0, input: bytes.Repeat([]byte("<html><body><H1>Hello world</H1></body></html>"), 32)},
		{name: "html-repeat", level: 6, input: bytes.Repeat([]byte("<html><body><H1>Hello world</H1></body></html>"), 32)},
		{name: "html-repeat", level: 9, input: bytes.Repeat([]byte("<html><body><H1>Hello world</H1></body></html>"), 32)},
		{name: "html-repeat", level: 11, input: bytes.Repeat([]byte("<html><body><H1>Hello world</H1></body></html>"), 32)},
		{name: "opticks-prefix", level: 0, input: opticks},
		{name: "opticks-prefix", level: 6, input: opticks},
		{name: "opticks-prefix", level: 9, input: opticks},
		{name: "opticks-prefix", level: 11, input: opticks},
		{name: "issue22-prefix", level: 0, input: issue22Data},
		{name: "issue22-prefix", level: 6, input: issue22Data},
		{name: "issue22-prefix", level: 9, input: issue22Data},
		{name: "issue22-prefix", level: 11, input: issue22Data},
	}, nil
}

func writeHelperModule(dir, baselineDir string) error {
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(helperMain), 0o644); err != nil {
		return err
	}
	mod := fmt.Sprintf(`module paritygen

go 1.26

require github.com/nijaru/brotli v0.0.0

replace github.com/nijaru/brotli => %s
`, baselineDir)
	return os.WriteFile(filepath.Join(dir, "go.mod"), []byte(mod), 0o644)
}

func encodeFixture(helper string, level int, input []byte) ([]byte, error) {
	cmd := exec.Command(helper, "-level", strconv.Itoa(level))
	cmd.Env = append(os.Environ(), "GOWORK=off")
	cmd.Stdin = bytes.NewReader(input)
	var out bytes.Buffer
	cmd.Stdout = &out
	var errOut bytes.Buffer
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("helper failed: %w\n%s", err, errOut.Bytes())
	}
	return out.Bytes(), nil
}

func getenv(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
