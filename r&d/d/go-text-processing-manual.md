# Go 1.22 Technical Manual: Text Processing, Lexical Scanning & Tokenization

> *"Simplicity is prerequisite for reliability."* — Edsger Dijkstra, but Pike would have said it if Dijkstra hadn't gotten there first.

**Go Version:** 1.22 | **Target Audience:** DevOps engineers, systems programmers, infrastructure developers  
**Scope:** NLP primitives, lexical scanning, tokenization across text, strings, JSON, YAML, byte sequences, and Unix files  
**Philosophy:** Do one thing well. Text is the universal interface. Compose small tools into large systems.

---

## Table of Contents

- [Part 1 — Basic Operations](#part-1--basic-operations)
- [Part 2 — Advanced Operations](#part-2--advanced-operations)
- [Part 3 — Cheatsheets](#part-3--cheatsheets)
- [Part 4 — Patterns & Anti-Patterns](#part-4--patterns--anti-patterns)
- [Part 5 — Deployment](#part-5--deployment)
- [Part 6 — Use Cases](#part-6--use-cases)
- [Part 7 — Appendix](#part-7--appendix)

---

## Part 1 — Basic Operations

### 1.1 The String–Byte–Rune Trinity

Go's type system for text rests on three primitives. Understanding their relationship is non-negotiable before doing any text processing work.

A `string` in Go is an immutable sequence of bytes. Not characters. Not runes. Bytes. This distinction matters because Go source files are UTF-8, and a single Unicode code point can span 1–4 bytes.

```go
package main

import (
    "fmt"
    "unicode/utf8"
)

func main() {
    s := "Hello, 世界"

    // Length in bytes vs runes
    fmt.Println(len(s))                    // 13 (bytes)
    fmt.Println(utf8.RuneCountInString(s)) // 9  (runes)

    // Iterate by byte
    for i := 0; i < len(s); i++ {
        fmt.Printf("byte[%d] = 0x%02x\n", i, s[i])
    }

    // Iterate by rune (the correct way for text processing)
    for i, r := range s {
        fmt.Printf("rune[%d] = %c (U+%04X)\n", i, r, r)
    }

    // Convert between types
    bs := []byte(s)       // string → []byte (copies)
    rs := []rune(s)       // string → []rune (copies)
    s2 := string(bs)      // []byte → string (copies)
    s3 := string(rs)      // []rune → string (copies)
    _ = s2
    _ = s3

    // Unsafe zero-copy (Go 1.22+): use only when lifetime is bounded
    // import "unsafe"
    // bs := unsafe.Slice(unsafe.StringData(s), len(s))
}
```

**Key insight:** `range` over a string yields runes, not bytes. A `for i := 0; i < len(s); i++` loop yields bytes. Pick the wrong one and your tokenizer breaks on any non-ASCII input.

### 1.2 The `strings` Package — Your Bread and Butter

The `strings` package is the workhorse for text processing. Every function here operates on UTF-8 encoded strings and handles multi-byte runes correctly.

```go
package main

import (
    "fmt"
    "strings"
)

func main() {
    text := "  ERROR: connection refused [host=db.prod.internal, port=5432]  "

    // --- Trimming ---
    clean := strings.TrimSpace(text)
    // "ERROR: connection refused [host=db.prod.internal, port=5432]"

    prefix := strings.TrimPrefix(clean, "ERROR: ")
    // "connection refused [host=db.prod.internal, port=5432]"

    // --- Splitting ---
    // Split on any whitespace-delimited tokens
    fields := strings.Fields(text)
    // ["ERROR:", "connection", "refused", "[host=db.prod.internal,", "port=5432]"]

    // Split on a specific delimiter
    parts := strings.SplitN(clean, ": ", 2) // at most 2 parts
    // ["ERROR", "connection refused [host=db.prod.internal, port=5432]"]

    // Split with separator retained
    afterParts := strings.SplitAfter("key=val;key2=val2;key3=val3", ";")
    // ["key=val;", "key2=val2;", "key3=val3"]

    // --- Searching ---
    fmt.Println(strings.Contains(clean, "refused"))     // true
    fmt.Println(strings.HasPrefix(clean, "ERROR"))      // true
    fmt.Println(strings.Index(clean, "host="))          // 23
    fmt.Println(strings.Count(clean, "="))              // 2

    // --- Replacement ---
    masked := strings.ReplaceAll(clean, "db.prod.internal", "***REDACTED***")

    // --- Building strings efficiently ---
    var b strings.Builder
    for _, f := range fields {
        b.WriteString(f)
        b.WriteByte(' ')
    }
    result := strings.TrimSpace(b.String())

    // --- Functional transforms ---
    upper := strings.Map(func(r rune) rune {
        if r >= 'a' && r <= 'z' {
            return r - 32
        }
        return r
    }, "hello world")
    // "HELLO WORLD"

    _ = prefix
    _ = parts
    _ = afterParts
    _ = masked
    _ = result
    _ = upper
}
```

### 1.3 The `bytes` Package — When You Work at the Wire Level

The `bytes` package mirrors `strings` but operates on `[]byte`. Use it when processing raw I/O, network buffers, or file content where you want to avoid allocating strings.

```go
package main

import (
    "bytes"
    "fmt"
    "os"
)

func main() {
    // Read a file as raw bytes
    data, err := os.ReadFile("/etc/hostname")
    if err != nil {
        fmt.Fprintf(os.Stderr, "read: %v\n", err)
        os.Exit(1)
    }

    // Trim trailing newline (common in Unix files)
    data = bytes.TrimRight(data, "\n")

    // Split on newlines for line-by-line processing
    lines := bytes.Split(data, []byte("\n"))
    for i, line := range lines {
        fmt.Printf("line %d: %s\n", i, line)
    }

    // bytes.Buffer for efficient assembly
    var buf bytes.Buffer
    buf.WriteString("token: ")
    buf.Write(data)
    buf.WriteByte('\n')
    os.Stdout.Write(buf.Bytes())

    // bytes.Reader for io.Reader from a byte slice (zero-copy)
    r := bytes.NewReader(data)
    fmt.Printf("reader len: %d\n", r.Len())

    // bytes.Contains, HasPrefix, etc. — same API as strings
    if bytes.HasPrefix(data, []byte("prod-")) {
        fmt.Println("production host detected")
    }

    // bytes.Cut (Go 1.18+) — split on first occurrence, cleaner than Index+slice
    before, after, found := bytes.Cut([]byte("key=value=extra"), []byte("="))
    if found {
        fmt.Printf("key=%s val=%s\n", before, after) // key=key val=value=extra
    }
}
```

### 1.4 `bufio.Scanner` — Line-Oriented Tokenization

`bufio.Scanner` is Go's answer to Unix line processing. By default it splits on `\n`, but it accepts custom split functions, making it the primary tool for structured text tokenization.

```go
package main

import (
    "bufio"
    "fmt"
    "os"
    "strings"
)

func main() {
    // --- Line scanning (default) ---
    input := "line one\nline two\nline three\n"
    scanner := bufio.NewScanner(strings.NewReader(input))
    for scanner.Scan() {
        fmt.Printf("LINE: %q\n", scanner.Text())
    }
    if err := scanner.Err(); err != nil {
        fmt.Fprintf(os.Stderr, "scan error: %v\n", err)
    }

    // --- Word scanning ---
    scanner = bufio.NewScanner(strings.NewReader("  hello   world   foo  "))
    scanner.Split(bufio.ScanWords)
    for scanner.Scan() {
        fmt.Printf("WORD: %q\n", scanner.Text())
    }

    // --- Rune scanning ---
    scanner = bufio.NewScanner(strings.NewReader("Go 世界"))
    scanner.Split(bufio.ScanRunes)
    for scanner.Scan() {
        fmt.Printf("RUNE: %q\n", scanner.Text())
    }

    // --- Custom split function: scan on semicolons ---
    scanner = bufio.NewScanner(strings.NewReader("tok1;tok2;tok3"))
    scanner.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
        // Search for semicolon
        for i := 0; i < len(data); i++ {
            if data[i] == ';' {
                return i + 1, data[:i], nil
            }
        }
        // At EOF, return remaining data as final token
        if atEOF && len(data) > 0 {
            return len(data), data, nil
        }
        // Request more data
        return 0, nil, nil
    })
    for scanner.Scan() {
        fmt.Printf("SEMI-TOKEN: %q\n", scanner.Text())
    }

    // --- Increase buffer for large lines (default is 64KB) ---
    big := bufio.NewScanner(os.Stdin)
    big.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024) // 10MB max
}
```

### 1.5 `text/scanner` — Proper Lexical Scanning

The `text/scanner` package provides a scanner for UTF-8-encoded text that recognizes Go-like tokens: identifiers, integers, floats, chars, strings, and raw strings. It is configurable and designed for building parsers.

```go
package main

import (
    "fmt"
    "strings"
    "text/scanner"
)

func main() {
    src := `name = "hello world"
count = 42
ratio = 3.14
enabled = true`

    var s scanner.Scanner
    s.Init(strings.NewReader(src))
    s.Filename = "config"

    // Configure what to scan (default scans Go tokens)
    // Enable scanning of identifiers, ints, floats, chars, strings, comments
    s.Mode = scanner.ScanIdents | scanner.ScanInts | scanner.ScanFloats | scanner.ScanStrings

    for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
        pos := s.Position
        switch tok {
        case scanner.Ident:
            fmt.Printf("%s: IDENT  %q\n", pos, s.TokenText())
        case scanner.Int:
            fmt.Printf("%s: INT    %q\n", pos, s.TokenText())
        case scanner.Float:
            fmt.Printf("%s: FLOAT  %q\n", pos, s.TokenText())
        case scanner.String:
            fmt.Printf("%s: STRING %q\n", pos, s.TokenText())
        default:
            fmt.Printf("%s: PUNCT  %q\n", pos, s.TokenText())
        }
    }
}
```

Output:
```
config:1:1:  IDENT  "name"
config:1:6:  PUNCT  "="
config:1:8:  STRING "\"hello world\""
config:2:1:  IDENT  "count"
config:2:9:  PUNCT  "="
config:2:11: INT    "42"
config:3:1:  IDENT  "ratio"
config:3:9:  PUNCT  "="
config:3:11: FLOAT  "3.14"
config:4:1:  IDENT  "enabled"
config:4:11: PUNCT  "="
config:4:13: IDENT  "true"
```

### 1.6 `regexp` — Regular Expression Tokenization

Go uses RE2 syntax (guaranteed linear-time, no backtracking catastrophes). The `regexp` package provides compiled regex patterns for extraction, splitting, and replacement.

```go
package main

import (
    "fmt"
    "regexp"
    "strings"
)

func main() {
    // Compile once, use many times (panics on bad pattern — use for constants)
    ipPattern := regexp.MustCompile(`(\d{1,3}\.){3}\d{1,3}`)
    kvPattern := regexp.MustCompile(`(\w+)=("(?:[^"\\]|\\.)*"|\S+)`)
    logPattern := regexp.MustCompile(
        `^(\d{4}-\d{2}-\d{2}T[\d:.]+Z?)\s+(\w+)\s+(.+)$`,
    )

    // --- Find all matches ---
    text := "servers: 10.0.1.5, 192.168.1.100, 10.0.2.7"
    ips := ipPattern.FindAllString(text, -1) // -1 = find all
    fmt.Println("IPs:", ips)
    // IPs: [10.0.1.5 192.168.1.100 10.0.2.7]

    // --- Named capture groups ---
    namedPattern := regexp.MustCompile(
        `(?P<level>INFO|WARN|ERROR)\s+(?P<msg>.+)`,
    )
    match := namedPattern.FindStringSubmatch("ERROR disk full on /dev/sda1")
    if match != nil {
        for i, name := range namedPattern.SubexpNames() {
            if name != "" {
                fmt.Printf("  %s = %q\n", name, match[i])
            }
        }
    }

    // --- Extract key=value pairs ---
    config := `host="db.prod" port=5432 user="admin" timeout=30s`
    matches := kvPattern.FindAllStringSubmatch(config, -1)
    kv := make(map[string]string, len(matches))
    for _, m := range matches {
        kv[m[1]] = strings.Trim(m[2], `"`)
    }
    fmt.Println("kv:", kv)

    // --- Split on a pattern ---
    csv := "one,,two,,,three"
    tokens := regexp.MustCompile(`,+`).Split(csv, -1)
    fmt.Println("tokens:", tokens) // [one two three]

    // --- Replace with function ---
    logLine := "2024-01-15T10:30:00Z ERROR connection refused"
    result := logPattern.ReplaceAllStringFunc(logLine, func(s string) string {
        parts := logPattern.FindStringSubmatch(s)
        return fmt.Sprintf("[%s] <%s> %s", parts[1], parts[2], parts[3])
    })
    fmt.Println(result)
}
```

### 1.7 `unicode` — Rune Classification

The `unicode` package provides functions for classifying runes by category. Essential for building tokenizers that handle international text correctly.

```go
package main

import (
    "fmt"
    "unicode"
)

func main() {
    testRunes := []rune{'A', '5', ' ', '!', '世', 'é', '\t', '\n', '_'}

    for _, r := range testRunes {
        var categories []string
        if unicode.IsLetter(r)  { categories = append(categories, "Letter") }
        if unicode.IsDigit(r)   { categories = append(categories, "Digit") }
        if unicode.IsSpace(r)   { categories = append(categories, "Space") }
        if unicode.IsPunct(r)   { categories = append(categories, "Punct") }
        if unicode.IsUpper(r)   { categories = append(categories, "Upper") }
        if unicode.IsLower(r)   { categories = append(categories, "Lower") }
        if unicode.IsControl(r) { categories = append(categories, "Control") }
        if unicode.IsSymbol(r)  { categories = append(categories, "Symbol") }
        fmt.Printf("  %q (U+%04X): %v\n", r, r, categories)
    }

    // Check specific Unicode ranges
    r := '漢'
    fmt.Println(unicode.Is(unicode.Han, r))      // true — CJK ideograph
    fmt.Println(unicode.Is(unicode.Latin, 'A'))   // true
    fmt.Println(unicode.Is(unicode.Cyrillic, 'Д')) // true
}
```

### 1.8 JSON Tokenization with `encoding/json`

Go's `json.Decoder` provides a streaming tokenizer via its `Token()` method. This is how you process JSON without loading the entire structure into memory.

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
    "strings"
)

func main() {
    jsonData := `{
        "servers": [
            {"host": "db-01.prod", "port": 5432, "primary": true},
            {"host": "db-02.prod", "port": 5432, "primary": false}
        ],
        "timeout": 30,
        "name": "production-cluster"
    }`

    dec := json.NewDecoder(strings.NewReader(jsonData))

    // Token-by-token streaming
    depth := 0
    for {
        tok, err := dec.Token()
        if err != nil {
            break // io.EOF or error
        }
        indent := strings.Repeat("  ", depth)

        switch v := tok.(type) {
        case json.Delim:
            switch v {
            case '{', '[':
                fmt.Printf("%sDELIM_OPEN %c\n", indent, v)
                depth++
            case '}', ']':
                depth--
                indent = strings.Repeat("  ", depth)
                fmt.Printf("%sDELIM_CLOSE %c\n", indent, v)
            }
        case string:
            fmt.Printf("%sSTRING %q\n", indent, v)
        case float64:
            fmt.Printf("%sNUMBER %g\n", indent, v)
        case bool:
            fmt.Printf("%sBOOL   %v\n", indent, v)
        case nil:
            fmt.Printf("%sNULL\n", indent)
        }
    }

    // --- Structured decode for known schemas ---
    type Server struct {
        Host    string `json:"host"`
        Port    int    `json:"port"`
        Primary bool   `json:"primary"`
    }
    type Cluster struct {
        Servers []Server `json:"servers"`
        Timeout int      `json:"timeout"`
        Name    string   `json:"name"`
    }

    var cluster Cluster
    if err := json.NewDecoder(strings.NewReader(jsonData)).Decode(&cluster); err != nil {
        fmt.Fprintf(os.Stderr, "decode: %v\n", err)
        os.Exit(1)
    }
    fmt.Printf("cluster: %+v\n", cluster)
}
```

### 1.9 YAML Processing with `gopkg.in/yaml.v3`

YAML parsing in Go uses the `yaml.v3` package, which provides both structured decoding and a node-level API for when you need to walk the YAML tree.

```go
package main

import (
    "fmt"
    "os"

    "gopkg.in/yaml.v3"
)

func main() {
    yamlData := []byte(`
name: platform-team
description: Core infrastructure team
repositories:
  - name: gh-iac
    visibility: private
    topics:
      - terraform
      - github
      - iac
    branch_protection:
      required_reviews: 2
      dismiss_stale: true
  - name: docker-base
    visibility: public
    topics:
      - docker
      - containers
members:
  - login: alice
    role: maintainer
  - login: bob
    role: member
`)

    // --- Structured decode ---
    type BranchProtection struct {
        RequiredReviews int  `yaml:"required_reviews"`
        DismissStale    bool `yaml:"dismiss_stale"`
    }
    type Repo struct {
        Name             string           `yaml:"name"`
        Visibility       string           `yaml:"visibility"`
        Topics           []string         `yaml:"topics"`
        BranchProtection BranchProtection `yaml:"branch_protection"`
    }
    type Member struct {
        Login string `yaml:"login"`
        Role  string `yaml:"role"`
    }
    type Team struct {
        Name         string   `yaml:"name"`
        Description  string   `yaml:"description"`
        Repositories []Repo   `yaml:"repositories"`
        Members      []Member `yaml:"members"`
    }

    var team Team
    if err := yaml.Unmarshal(yamlData, &team); err != nil {
        fmt.Fprintf(os.Stderr, "yaml unmarshal: %v\n", err)
        os.Exit(1)
    }
    fmt.Printf("Team: %s (%d repos, %d members)\n",
        team.Name, len(team.Repositories), len(team.Members))

    // --- Node-level walking (for generic YAML processing) ---
    var root yaml.Node
    if err := yaml.Unmarshal(yamlData, &root); err != nil {
        fmt.Fprintf(os.Stderr, "yaml node parse: %v\n", err)
        os.Exit(1)
    }
    walkNode(&root, 0)
}

func walkNode(node *yaml.Node, depth int) {
    indent := ""
    for i := 0; i < depth; i++ {
        indent += "  "
    }

    switch node.Kind {
    case yaml.DocumentNode:
        fmt.Printf("%sDOCUMENT\n", indent)
        for _, child := range node.Content {
            walkNode(child, depth+1)
        }
    case yaml.MappingNode:
        fmt.Printf("%sMAPPING (line %d)\n", indent, node.Line)
        for i := 0; i < len(node.Content)-1; i += 2 {
            key := node.Content[i]
            val := node.Content[i+1]
            fmt.Printf("%s  KEY: %s\n", indent, key.Value)
            walkNode(val, depth+2)
        }
    case yaml.SequenceNode:
        fmt.Printf("%sSEQUENCE (line %d)\n", indent, node.Line)
        for _, item := range node.Content {
            walkNode(item, depth+1)
        }
    case yaml.ScalarNode:
        fmt.Printf("%sSCALAR: %q (tag=%s)\n", indent, node.Value, node.Tag)
    case yaml.AliasNode:
        fmt.Printf("%sALIAS -> %s\n", indent, node.Value)
    }
}
```

### 1.10 Reading Unix Files — The Right Way

Unix files have conventions: newline-terminated lines, `#` comments, `/etc`-style configs, stdin as default input. Go handles all of them.

```go
package main

import (
    "bufio"
    "bytes"
    "fmt"
    "io"
    "os"
    "strings"
)

// readLines reads a Unix text file, stripping comments and blank lines.
func readLines(r io.Reader) ([]string, error) {
    var lines []string
    scanner := bufio.NewScanner(r)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" || strings.HasPrefix(line, "#") {
            continue
        }
        // Strip inline comments
        if idx := strings.Index(line, " #"); idx >= 0 {
            line = strings.TrimSpace(line[:idx])
        }
        lines = append(lines, line)
    }
    return lines, scanner.Err()
}

// readInput returns a reader from a file path or stdin if path is "-" or empty
func readInput(path string) (io.ReadCloser, error) {
    if path == "" || path == "-" {
        return os.Stdin, nil
    }
    return os.Open(path)
}

// readBinaryChunks reads a binary file in fixed-size chunks
func readBinaryChunks(path string, chunkSize int) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()

    buf := make([]byte, chunkSize)
    offset := 0
    for {
        n, err := f.Read(buf)
        if n > 0 {
            // Process chunk: buf[:n]
            fmt.Printf("offset=%08x len=%d first4=%x\n",
                offset, n, buf[:min(4, n)])
            offset += n
        }
        if err == io.EOF {
            break
        }
        if err != nil {
            return err
        }
    }
    return nil
}

func main() {
    // Example: parse /etc/hosts style file
    hostsData := `# /etc/hosts
127.0.0.1   localhost
::1         localhost ip6-localhost
10.0.1.5    db.prod.internal    # database primary
10.0.1.6    db.replica.internal # database replica
`
    lines, err := readLines(strings.NewReader(hostsData))
    if err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
    for _, line := range lines {
        fields := strings.Fields(line)
        if len(fields) >= 2 {
            fmt.Printf("  IP=%-16s HOSTS=%s\n", fields[0], strings.Join(fields[1:], ", "))
        }
    }

    // Example: detect file type by magic bytes
    detectFileType([]byte("\x89PNG\r\n\x1a\n"))
    detectFileType([]byte("PK\x03\x04"))
    detectFileType([]byte("{\"key\":"))
    detectFileType([]byte("---\nname:"))
}

func detectFileType(data []byte) {
    switch {
    case bytes.HasPrefix(data, []byte("\x89PNG")):
        fmt.Println("detected: PNG image")
    case bytes.HasPrefix(data, []byte("PK\x03\x04")):
        fmt.Println("detected: ZIP archive (or docx/xlsx/jar)")
    case bytes.HasPrefix(data, []byte("\x1f\x8b")):
        fmt.Println("detected: gzip compressed")
    case bytes.HasPrefix(data, []byte("GIF8")):
        fmt.Println("detected: GIF image")
    case data[0] == '{' || data[0] == '[':
        fmt.Println("detected: likely JSON")
    case bytes.HasPrefix(data, []byte("---")):
        fmt.Println("detected: likely YAML")
    default:
        fmt.Println("detected: unknown")
    }
}

func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}
```

---

## Part 2 — Advanced Operations

### 2.1 Hand-Rolled Lexer — State Machine Pattern

When `text/scanner` or regex aren't enough, you build a lexer from scratch. This is the pattern used by Go's own compiler, Rob Pike's template engine, and every serious parser. The core idea: a state function that returns the next state function.

```go
package main

import (
    "fmt"
    "strings"
    "unicode"
    "unicode/utf8"
)

// TokenType represents the type of a lexed token.
type TokenType int

const (
    TokenError TokenType = iota
    TokenEOF
    TokenIdent
    TokenNumber
    TokenString
    TokenOperator
    TokenLParen
    TokenRParen
    TokenComma
    TokenNewline
)

func (t TokenType) String() string {
    names := [...]string{
        "ERROR", "EOF", "IDENT", "NUMBER", "STRING",
        "OPERATOR", "LPAREN", "RPAREN", "COMMA", "NEWLINE",
    }
    if int(t) < len(names) {
        return names[t]
    }
    return "UNKNOWN"
}

// Token is a lexed token with position info.
type Token struct {
    Type  TokenType
    Value string
    Pos   int
    Line  int
    Col   int
}

// Lexer holds the state of the scanner.
type Lexer struct {
    input  string
    pos    int    // current position in input
    start  int    // start of current token
    width  int    // width of last rune read
    line   int    // current line number
    col    int    // current column number
    tokens []Token
}

const eof = -1

func NewLexer(input string) *Lexer {
    return &Lexer{input: input, line: 1, col: 1}
}

func (l *Lexer) next() rune {
    if l.pos >= len(l.input) {
        l.width = 0
        return eof
    }
    r, w := utf8.DecodeRuneInString(l.input[l.pos:])
    l.width = w
    l.pos += w
    if r == '\n' {
        l.line++
        l.col = 1
    } else {
        l.col++
    }
    return r
}

func (l *Lexer) backup() {
    l.pos -= l.width
    if l.input[l.pos] == '\n' {
        l.line--
    }
    l.col--
}

func (l *Lexer) peek() rune {
    r := l.next()
    l.backup()
    return r
}

func (l *Lexer) emit(t TokenType) {
    l.tokens = append(l.tokens, Token{
        Type:  t,
        Value: l.input[l.start:l.pos],
        Pos:   l.start,
    })
    l.start = l.pos
}

func (l *Lexer) ignore() {
    l.start = l.pos
}

func (l *Lexer) errorf(format string, args ...any) stateFn {
    l.tokens = append(l.tokens, Token{
        Type:  TokenError,
        Value: fmt.Sprintf(format, args...),
        Pos:   l.start,
    })
    return nil
}

// stateFn is a state function: each state returns the next state.
type stateFn func(*Lexer) stateFn

// Run executes the lexer and returns all tokens.
func (l *Lexer) Run() []Token {
    for state := lexStart; state != nil; {
        state = state(l)
    }
    return l.tokens
}

// --- State functions ---

func lexStart(l *Lexer) stateFn {
    for {
        r := l.next()
        switch {
        case r == eof:
            l.emit(TokenEOF)
            return nil
        case r == '\n':
            l.emit(TokenNewline)
        case unicode.IsSpace(r):
            l.ignore() // skip whitespace
        case r == '"':
            return lexString
        case r == '(' :
            l.emit(TokenLParen)
        case r == ')':
            l.emit(TokenRParen)
        case r == ',':
            l.emit(TokenComma)
        case r == '+' || r == '-' || r == '*' || r == '/' ||
            r == '=' || r == '<' || r == '>' || r == '!':
            // peek for two-char operators
            if next := l.peek(); (r == '=' || r == '!' || r == '<' || r == '>') && next == '=' {
                l.next()
            }
            l.emit(TokenOperator)
        case unicode.IsDigit(r):
            l.backup()
            return lexNumber
        case unicode.IsLetter(r) || r == '_':
            l.backup()
            return lexIdent
        default:
            return l.errorf("unexpected character: %c", r)
        }
    }
}

func lexString(l *Lexer) stateFn {
    for {
        r := l.next()
        switch {
        case r == eof:
            return l.errorf("unterminated string")
        case r == '\\':
            l.next() // skip escaped char
        case r == '"':
            l.emit(TokenString)
            return lexStart
        }
    }
}

func lexNumber(l *Lexer) stateFn {
    digits := "0123456789"
    if l.peek() == '0' {
        l.next()
        if p := l.peek(); p == 'x' || p == 'X' {
            l.next()
            digits = "0123456789abcdefABCDEF"
        }
    }
    for strings.ContainsRune(digits, l.peek()) {
        l.next()
    }
    if l.peek() == '.' {
        l.next()
        for strings.ContainsRune("0123456789", l.peek()) {
            l.next()
        }
    }
    l.emit(TokenNumber)
    return lexStart
}

func lexIdent(l *Lexer) stateFn {
    for {
        r := l.peek()
        if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' {
            break
        }
        l.next()
    }
    l.emit(TokenIdent)
    return lexStart
}

func main() {
    input := `count = 42
name = "hello world"
result = add(count, 10) * 2`

    lex := NewLexer(input)
    tokens := lex.Run()
    for _, tok := range tokens {
        fmt.Printf("  %-10s %q\n", tok.Type, tok.Value)
    }
}
```

This is Rob Pike's lexer pattern from the 2011 talk "Lexical Scanning in Go." The key insight is that state functions eliminate the switch-in-a-loop pattern and make the lexer naturally composable. Each state function is a self-contained unit that knows how to lex one kind of token and which state to transition to next.

### 2.2 Go's Own Lexer as Reference: `go/scanner` and `go/token`

Go ships its own lexer in the standard library. Studying it is the single best way to learn lexer design in Go.

```go
package main

import (
    "fmt"
    "go/scanner"
    "go/token"
)

func main() {
    src := []byte(`
package main

import "fmt"

func main() {
    x := 42 + 3.14
    fmt.Println("hello, 世界", x)
}
`)

    fset := token.NewFileSet()
    file := fset.AddFile("example.go", fset.Base(), len(src))

    var s scanner.Scanner
    s.Init(file, src, func(pos token.Position, msg string) {
        fmt.Printf("ERROR at %s: %s\n", pos, msg)
    }, scanner.ScanComments)

    for {
        pos, tok, lit := s.Scan()
        if tok == token.EOF {
            break
        }
        position := fset.Position(pos)
        if lit != "" {
            fmt.Printf("%-20s %-12s %q\n", position, tok, lit)
        } else {
            fmt.Printf("%-20s %-12s\n", position, tok)
        }
    }
}
```

### 2.3 Streaming JSON Processing with `json.Decoder`

For large JSON files (multi-GB log dumps, API responses), stream with `Decoder` to avoid loading everything into memory.

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "os"
    "strings"
)

type LogEntry struct {
    Timestamp string                 `json:"ts"`
    Level     string                 `json:"level"`
    Message   string                 `json:"msg"`
    Fields    map[string]interface{} `json:"fields"`
}

func processJSONStream(r io.Reader) error {
    dec := json.NewDecoder(r)

    // If the stream is a JSON array, consume the opening bracket
    tok, err := dec.Token()
    if err != nil {
        return fmt.Errorf("read opening token: %w", err)
    }
    if delim, ok := tok.(json.Delim); ok && delim != '[' {
        return fmt.Errorf("expected '[', got %v", tok)
    }

    count := 0
    errors := 0
    for dec.More() {
        var entry LogEntry
        if err := dec.Decode(&entry); err != nil {
            fmt.Fprintf(os.Stderr, "skip bad entry: %v\n", err)
            errors++
            continue
        }
        count++
        if entry.Level == "ERROR" {
            fmt.Printf("[%s] %s: %s\n", entry.Timestamp, entry.Level, entry.Message)
        }
    }
    fmt.Fprintf(os.Stderr, "processed %d entries (%d errors)\n", count, errors)
    return nil
}

func main() {
    jsonStream := `[
        {"ts":"2024-01-15T10:00:00Z","level":"INFO","msg":"server started","fields":{"port":8080}},
        {"ts":"2024-01-15T10:00:05Z","level":"ERROR","msg":"connection refused","fields":{"host":"db.prod","port":5432}},
        {"ts":"2024-01-15T10:00:10Z","level":"WARN","msg":"high latency","fields":{"p99_ms":250}},
        {"ts":"2024-01-15T10:00:15Z","level":"ERROR","msg":"disk full","fields":{"mount":"/data","pct":99.2}}
    ]`

    if err := processJSONStream(strings.NewReader(jsonStream)); err != nil {
        fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
        os.Exit(1)
    }
}
```

### 2.4 YAML Multi-Document Streaming

YAML supports multiple documents in a single stream, separated by `---`. The `yaml.Decoder` handles this naturally.

```go
package main

import (
    "fmt"
    "io"
    "os"
    "strings"

    "gopkg.in/yaml.v3"
)

func main() {
    multiDoc := `---
kind: Deployment
metadata:
  name: api-server
spec:
  replicas: 3
---
kind: Service
metadata:
  name: api-service
spec:
  type: ClusterIP
  port: 8080
---
kind: ConfigMap
metadata:
  name: api-config
data:
  log_level: info
  max_conns: "100"
`

    dec := yaml.NewDecoder(strings.NewReader(multiDoc))
    docNum := 0
    for {
        var doc map[string]interface{}
        err := dec.Decode(&doc)
        if err == io.EOF {
            break
        }
        if err != nil {
            fmt.Fprintf(os.Stderr, "yaml decode error: %v\n", err)
            os.Exit(1)
        }
        docNum++

        kind, _ := doc["kind"].(string)
        meta, _ := doc["metadata"].(map[string]interface{})
        name := ""
        if meta != nil {
            name, _ = meta["name"].(string)
        }
        fmt.Printf("doc %d: kind=%s name=%s\n", docNum, kind, name)
    }
}
```

### 2.5 Byte Sequence Tokenization — Binary Protocols and Formats

When parsing binary formats, network protocols, or byte-level data, you work with `io.Reader`, `binary.Read`, and manual byte slicing.

```go
package main

import (
    "bytes"
    "encoding/binary"
    "fmt"
    "io"
)

// TLV (Type-Length-Value) parser — common in network protocols
type TLVRecord struct {
    Type   uint16
    Length uint16
    Value  []byte
}

func parseTLV(data []byte) ([]TLVRecord, error) {
    r := bytes.NewReader(data)
    var records []TLVRecord

    for {
        var rec TLVRecord
        if err := binary.Read(r, binary.BigEndian, &rec.Type); err != nil {
            if err == io.EOF {
                break
            }
            return nil, fmt.Errorf("read type: %w", err)
        }
        if err := binary.Read(r, binary.BigEndian, &rec.Length); err != nil {
            return nil, fmt.Errorf("read length: %w", err)
        }
        rec.Value = make([]byte, rec.Length)
        if _, err := io.ReadFull(r, rec.Value); err != nil {
            return nil, fmt.Errorf("read value: %w", err)
        }
        records = append(records, rec)
    }
    return records, nil
}

// Fixed-width record parser (mainframe-style flat files)
type FixedRecord struct {
    Name    string // positions 0-19
    Account string // positions 20-29
    Amount  string // positions 30-39
}

func parseFixedWidth(data []byte) []FixedRecord {
    var records []FixedRecord
    lines := bytes.Split(data, []byte("\n"))
    for _, line := range lines {
        if len(line) < 40 {
            continue
        }
        records = append(records, FixedRecord{
            Name:    string(bytes.TrimSpace(line[0:20])),
            Account: string(bytes.TrimSpace(line[20:30])),
            Amount:  string(bytes.TrimSpace(line[30:40])),
        })
    }
    return records
}

func main() {
    // Build sample TLV data
    var buf bytes.Buffer
    // Record 1: Type=1 (hostname), Value="db.prod"
    binary.Write(&buf, binary.BigEndian, uint16(1))
    binary.Write(&buf, binary.BigEndian, uint16(7))
    buf.WriteString("db.prod")
    // Record 2: Type=2 (port), Value="5432"
    binary.Write(&buf, binary.BigEndian, uint16(2))
    binary.Write(&buf, binary.BigEndian, uint16(4))
    buf.WriteString("5432")

    records, err := parseTLV(buf.Bytes())
    if err != nil {
        fmt.Printf("tlv error: %v\n", err)
        return
    }
    for _, rec := range records {
        fmt.Printf("TLV type=%d len=%d val=%q\n", rec.Type, rec.Length, rec.Value)
    }

    // Fixed-width parsing
    fixedData := []byte(
        "Alice Johnson       ACC-001234 0000152.50\n" +
        "Bob Smith           ACC-005678 0001024.00\n")
    for _, rec := range parseFixedWidth(fixedData) {
        fmt.Printf("FIXED name=%-20s acct=%s amt=%s\n", rec.Name, rec.Account, rec.Amount)
    }
}
```

### 2.6 Natural Language Processing Primitives

Go doesn't have a scikit-learn, but for DevOps-grade NLP — tokenizing log messages, extracting entities from alert text, computing similarity between incident descriptions — you can build effective tools from the standard library and `golang.org/x/text`.

```go
package main

import (
    "fmt"
    "math"
    "sort"
    "strings"
    "unicode"
)

// --- Whitespace tokenizer with normalization ---

func tokenize(text string) []string {
    text = strings.ToLower(text)
    var tokens []string
    var current strings.Builder

    for _, r := range text {
        if unicode.IsLetter(r) || unicode.IsDigit(r) {
            current.WriteRune(r)
        } else if current.Len() > 0 {
            tokens = append(tokens, current.String())
            current.Reset()
        }
    }
    if current.Len() > 0 {
        tokens = append(tokens, current.String())
    }
    return tokens
}

// --- Stopword removal ---

var defaultStopwords = map[string]bool{
    "a": true, "an": true, "the": true, "is": true, "are": true,
    "was": true, "were": true, "be": true, "been": true, "being": true,
    "have": true, "has": true, "had": true, "do": true, "does": true,
    "did": true, "will": true, "would": true, "could": true, "should": true,
    "may": true, "might": true, "shall": true, "can": true,
    "in": true, "on": true, "at": true, "to": true, "for": true,
    "of": true, "with": true, "by": true, "from": true, "as": true,
    "into": true, "through": true, "during": true, "before": true,
    "after": true, "and": true, "but": true, "or": true, "nor": true,
    "not": true, "so": true, "yet": true, "it": true, "its": true,
    "this": true, "that": true, "these": true, "those": true,
}

func removeStopwords(tokens []string) []string {
    var result []string
    for _, t := range tokens {
        if !defaultStopwords[t] {
            result = append(result, t)
        }
    }
    return result
}

// --- N-gram generation ---

func ngrams(tokens []string, n int) []string {
    if len(tokens) < n {
        return nil
    }
    var result []string
    for i := 0; i <= len(tokens)-n; i++ {
        result = append(result, strings.Join(tokens[i:i+n], " "))
    }
    return result
}

// --- Term Frequency (TF) ---

func termFrequency(tokens []string) map[string]float64 {
    counts := make(map[string]int)
    for _, t := range tokens {
        counts[t]++
    }
    tf := make(map[string]float64, len(counts))
    total := float64(len(tokens))
    for term, count := range counts {
        tf[term] = float64(count) / total
    }
    return tf
}

// --- Cosine similarity between two documents ---

func cosineSimilarity(a, b map[string]float64) float64 {
    var dot, normA, normB float64
    for term, va := range a {
        if vb, ok := b[term]; ok {
            dot += va * vb
        }
        normA += va * va
    }
    for _, vb := range b {
        normB += vb * vb
    }
    if normA == 0 || normB == 0 {
        return 0
    }
    return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// --- Levenshtein distance (edit distance) ---

func levenshtein(a, b string) int {
    la, lb := len([]rune(a)), len([]rune(b))
    d := make([][]int, la+1)
    for i := range d {
        d[i] = make([]int, lb+1)
        d[i][0] = i
    }
    for j := 1; j <= lb; j++ {
        d[0][j] = j
    }
    ra, rb := []rune(a), []rune(b)
    for i := 1; i <= la; i++ {
        for j := 1; j <= lb; j++ {
            cost := 1
            if ra[i-1] == rb[j-1] {
                cost = 0
            }
            d[i][j] = min3(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
        }
    }
    return d[la][lb]
}

func min3(a, b, c int) int {
    if a < b {
        if a < c { return a }
        return c
    }
    if b < c { return b }
    return c
}

func main() {
    doc1 := "ERROR: connection timeout to database server db-01.prod on port 5432"
    doc2 := "CRITICAL: database connection failed, timeout reached for db-01.prod:5432"
    doc3 := "INFO: deployment completed successfully for api-server v2.3.1"

    // Tokenize and clean
    t1 := removeStopwords(tokenize(doc1))
    t2 := removeStopwords(tokenize(doc2))
    t3 := removeStopwords(tokenize(doc3))

    fmt.Println("Tokens doc1:", t1)
    fmt.Println("Tokens doc2:", t2)
    fmt.Println("Tokens doc3:", t3)

    // TF vectors
    tf1 := termFrequency(t1)
    tf2 := termFrequency(t2)
    tf3 := termFrequency(t3)

    // Similarity
    fmt.Printf("\nSimilarity(doc1, doc2) = %.4f  (related incidents)\n", cosineSimilarity(tf1, tf2))
    fmt.Printf("Similarity(doc1, doc3) = %.4f  (unrelated)\n", cosineSimilarity(tf1, tf3))
    fmt.Printf("Similarity(doc2, doc3) = %.4f  (unrelated)\n", cosineSimilarity(tf2, tf3))

    // N-grams
    bigrams := ngrams(t1, 2)
    fmt.Println("\nBigrams doc1:", bigrams)

    // Edit distance
    fmt.Printf("\nEditDistance(\"database\", \"databaes\") = %d\n",
        levenshtein("database", "databaes"))

    // Top terms by frequency
    fmt.Println("\nTop terms in doc1:")
    type kv struct{ k string; v float64 }
    var sorted []kv
    for k, v := range tf1 {
        sorted = append(sorted, kv{k, v})
    }
    sort.Slice(sorted, func(i, j int) bool { return sorted[i].v > sorted[j].v })
    for _, item := range sorted {
        fmt.Printf("  %-15s %.4f\n", item.k, item.v)
    }
}
```

---

## Part 3 — Cheatsheets

### 3.1 String Operations Quick Reference

```
╔══════════════════════════════════════════════════════════════════════╗
║                    STRING OPERATIONS CHEATSHEET                     ║
╠═══════════════════════╦══════════════════════════════════════════════╣
║ OPERATION             ║ CODE                                       ║
╠═══════════════════════╬══════════════════════════════════════════════╣
║ Split on whitespace   ║ strings.Fields(s)                          ║
║ Split on delimiter    ║ strings.Split(s, ",")                      ║
║ Split N times         ║ strings.SplitN(s, "=", 2)                  ║
║ Cut on first match    ║ before, after, ok := strings.Cut(s, "=")   ║
║ Join                  ║ strings.Join(parts, ", ")                  ║
║ Trim whitespace       ║ strings.TrimSpace(s)                       ║
║ Trim chars            ║ strings.Trim(s, "\"'")                     ║
║ Trim prefix           ║ strings.TrimPrefix(s, "http://")           ║
║ Contains              ║ strings.Contains(s, "error")               ║
║ Has prefix            ║ strings.HasPrefix(s, "ERROR")              ║
║ Find index            ║ strings.Index(s, "=")                      ║
║ Count occurrences     ║ strings.Count(s, "\n")                     ║
║ Replace all           ║ strings.ReplaceAll(s, old, new)            ║
║ Replace N             ║ strings.Replace(s, old, new, 2)            ║
║ To upper/lower        ║ strings.ToUpper(s) / strings.ToLower(s)    ║
║ Title case            ║ cases.Title(language.English).String(s)    ║
║ Repeat                ║ strings.Repeat("-", 40)                    ║
║ Compare (ordinal)     ║ strings.Compare(a, b)                      ║
║ Compare (fold case)   ║ strings.EqualFold(a, b)                    ║
║ Map runes             ║ strings.Map(func(r rune) rune{...}, s)     ║
║ New replacer          ║ r := strings.NewReplacer(old1,new1,...)    ║
║ Builder (efficient)   ║ var b strings.Builder; b.WriteString(...)  ║
║ Reader (io.Reader)    ║ strings.NewReader(s)                       ║
║ Rune count            ║ utf8.RuneCountInString(s)                  ║
║ Valid UTF-8           ║ utf8.ValidString(s)                        ║
╚═══════════════════════╩══════════════════════════════════════════════╝
```

### 3.2 Regex Quick Reference (RE2 Syntax)

```
╔══════════════════════════════════════════════════════════════════════╗
║                       REGEX CHEATSHEET (RE2)                        ║
╠════════════════╦═════════════════════════════════════════════════════╣
║ PATTERN        ║ MEANING                                            ║
╠════════════════╬═════════════════════════════════════════════════════╣
║ .              ║ Any character except newline                       ║
║ \d  \D         ║ Digit / non-digit                                  ║
║ \w  \W         ║ Word char [A-Za-z0-9_] / non-word                  ║
║ \s  \S         ║ Whitespace / non-whitespace                        ║
║ ^  $           ║ Start / end of line (with (?m) flag)               ║
║ \b             ║ Word boundary                                      ║
║ [abc]          ║ Character class                                    ║
║ [^abc]         ║ Negated class                                      ║
║ [a-z]          ║ Range                                              ║
║ a*  a+  a?     ║ 0+, 1+, 0-or-1 (greedy)                           ║
║ a*? a+? a??    ║ Non-greedy versions                                ║
║ a{3}  a{3,5}   ║ Exactly 3 / 3 to 5                                ║
║ (abc)          ║ Capture group                                      ║
║ (?:abc)        ║ Non-capturing group                                ║
║ (?P<name>abc)  ║ Named capture group                                ║
║ a|b            ║ Alternation                                        ║
║ (?i)           ║ Case-insensitive flag                              ║
║ (?m)           ║ Multi-line (^ and $ match line boundaries)         ║
║ (?s)           ║ Dot matches newline                                ║
╠════════════════╩═════════════════════════════════════════════════════╣
║                                                                      ║
║  NOTE: RE2 does NOT support: backreferences (\1), lookahead          ║
║  (?=), lookbehind (?<=), possessive quantifiers (a++).               ║
║  This is by design — RE2 guarantees O(n) matching.                   ║
║                                                                      ║
╠══════════════════════════════════════════════════════════════════════╣
║ FUNCTION                  ║ USE                                      ║
╠═══════════════════════════╬══════════════════════════════════════════╣
║ MustCompile(pat)          ║ Compile; panic on error (for constants) ║
║ Compile(pat)              ║ Compile; return error                   ║
║ MatchString(s)            ║ Does pattern match string?              ║
║ FindString(s)             ║ First match                             ║
║ FindAllString(s, n)       ║ All matches (n=-1 for unlimited)        ║
║ FindStringSubmatch(s)     ║ First match + capture groups            ║
║ FindAllStringSubmatch(s,n)║ All matches + capture groups            ║
║ ReplaceAllString(s, repl) ║ Replace all matches                     ║
║ ReplaceAllStringFunc(s,f) ║ Replace with function                   ║
║ Split(s, n)               ║ Split string on pattern                 ║
║ SubexpNames()             ║ Named group names                       ║
╚═══════════════════════════╩══════════════════════════════════════════╝
```

### 3.3 `bufio.Scanner` Quick Reference

```
╔══════════════════════════════════════════════════════════════════════╗
║                   BUFIO.SCANNER CHEATSHEET                          ║
╠══════════════════════════════════════════════════════════════════════╣
║                                                                      ║
║  scanner := bufio.NewScanner(reader)                                 ║
║                                                                      ║
║  BUILT-IN SPLIT FUNCTIONS:                                           ║
║  ┌──────────────────┬────────────────────────────────────┐           ║
║  │ bufio.ScanLines  │ Split on \n (default). Strips \r.  │           ║
║  │ bufio.ScanWords  │ Split on whitespace runs           │           ║
║  │ bufio.ScanRunes  │ One UTF-8 rune per token           │           ║
║  │ bufio.ScanBytes  │ One byte per token                 │           ║
║  └──────────────────┴────────────────────────────────────┘           ║
║                                                                      ║
║  scanner.Split(splitFunc)  // set custom splitter                    ║
║  scanner.Buffer(buf, max)  // set buffer size (before Scan)          ║
║                                                                      ║
║  SPLIT FUNCTION SIGNATURE:                                           ║
║  func(data []byte, atEOF bool) (advance int, token []byte, err)     ║
║                                                                      ║
║  RETURN VALUES:                                                      ║
║  advance > 0, token != nil  → emit token, advance input             ║
║  advance == 0, token == nil → request more data                      ║
║  any, any, err != nil       → stop with error                        ║
║  atEOF && len(data) == 0    → signal to stop (return 0, nil, nil)   ║
║                                                                      ║
║  LOOP PATTERN:                                                       ║
║  for scanner.Scan() {                                                ║
║      line := scanner.Text()   // string (copies)                     ║
║      raw  := scanner.Bytes()  // []byte (no copy, volatile!)         ║
║  }                                                                   ║
║  if err := scanner.Err(); err != nil { handle(err) }                ║
║                                                                      ║
║  DEFAULT MAX TOKEN SIZE: 64 * 1024 (65536 bytes)                     ║
║  Override: scanner.Buffer(make([]byte, 0, size), maxSize)            ║
║                                                                      ║
╚══════════════════════════════════════════════════════════════════════╝
```

### 3.4 JSON/YAML Processing Quick Reference

```
╔══════════════════════════════════════════════════════════════════════╗
║               JSON / YAML PROCESSING CHEATSHEET                     ║
╠══════════════════════════════════════════════════════════════════════╣
║                                                                      ║
║  encoding/json                                                       ║
║  ┌─────────────────────────────────────────────────────────────┐     ║
║  │ json.Marshal(v)           → []byte, error                   │     ║
║  │ json.MarshalIndent(v,p,i) → []byte, error (pretty)          │     ║
║  │ json.Unmarshal(data, &v)  → error                           │     ║
║  │ json.NewDecoder(r)        → *Decoder (streaming)            │     ║
║  │ json.NewEncoder(w)        → *Encoder (streaming)            │     ║
║  │ dec.Token()               → Token, error (lexer-level)      │     ║
║  │ dec.Decode(&v)            → error (one object)              │     ║
║  │ dec.More()                → bool (more in array/object)      │     ║
║  │ json.Valid(data)          → bool                            │     ║
║  │ json.RawMessage           → deferred decode (keep raw JSON)  │     ║
║  │ json.Number               → string (lossless numbers)        │     ║
║  └─────────────────────────────────────────────────────────────┘     ║
║                                                                      ║
║  STRUCT TAGS:                                                        ║
║  `json:"field_name"`          ║ rename field                         ║
║  `json:"field,omitempty"`     ║ skip zero values                     ║
║  `json:"-"`                   ║ always skip                          ║
║  `json:",string"`             ║ encode number as JSON string         ║
║                                                                      ║
║  gopkg.in/yaml.v3                                                    ║
║  ┌─────────────────────────────────────────────────────────────┐     ║
║  │ yaml.Marshal(v)           → []byte, error                   │     ║
║  │ yaml.Unmarshal(data, &v)  → error                           │     ║
║  │ yaml.NewDecoder(r)        → *Decoder (multi-doc stream)     │     ║
║  │ yaml.NewEncoder(w)        → *Encoder                        │     ║
║  │ yaml.Node                 → tree-level access                │     ║
║  └─────────────────────────────────────────────────────────────┘     ║
║                                                                      ║
║  YAML NODE KINDS:                                                    ║
║  yaml.DocumentNode  yaml.MappingNode  yaml.SequenceNode             ║
║  yaml.ScalarNode    yaml.AliasNode                                   ║
║                                                                      ║
║  STRUCT TAGS:                                                        ║
║  `yaml:"field_name"`          ║ rename field                         ║
║  `yaml:"field,omitempty"`     ║ skip zero values                     ║
║  `yaml:"-"`                   ║ always skip                          ║
║  `yaml:",flow"`               ║ inline style [a, b, c]              ║
║  `yaml:",inline"`             ║ inline struct fields into parent     ║
║                                                                      ║
╚══════════════════════════════════════════════════════════════════════╝
```

---

## Part 4 — Patterns & Anti-Patterns

### 4.1 Patterns — Do These

**Pattern 1: Accept `io.Reader`, Return Concrete Types**

The fundamental Go interface pattern. Your tokenizer should accept an `io.Reader` so it works with files, strings, network connections, pipes, and stdin interchangeably.

```go
// GOOD — accepts any reader
func Tokenize(r io.Reader) ([]Token, error) {
    scanner := bufio.NewScanner(r)
    // ...
}

// Usage:
Tokenize(os.Stdin)                          // pipe
Tokenize(strings.NewReader("hello world"))  // string
f, _ := os.Open("data.txt"); Tokenize(f)   // file
Tokenize(resp.Body)                         // HTTP response
```

**Pattern 2: Use `strings.Cut` Over `strings.SplitN`**

Go 1.18 introduced `strings.Cut`. It's clearer, returns a boolean, and is the idiomatic way to split on first occurrence.

```go
// GOOD
key, value, ok := strings.Cut(line, "=")
if !ok {
    return fmt.Errorf("missing '=' in %q", line)
}

// AVOID (works but less clear)
parts := strings.SplitN(line, "=", 2)
if len(parts) != 2 {
    return fmt.Errorf("missing '=' in %q", line)
}
key, value := parts[0], parts[1]
```

**Pattern 3: Pre-compile Regex Patterns as Package-Level `var`**

```go
// GOOD — compiled once at init time
var (
    reTimestamp = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T[\d:.]+Z?`)
    reIPAddr    = regexp.MustCompile(`\b(\d{1,3}\.){3}\d{1,3}\b`)
)

func extractTimestamp(line string) string {
    return reTimestamp.FindString(line)
}

// BAD — recompiled on every call
func extractTimestampBad(line string) string {
    re := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T[\d:.]+Z?`)
    return re.FindString(line)
}
```

**Pattern 4: Use `strings.Builder` for Assembly, Not `+` Concatenation**

```go
// GOOD — O(n) total allocation
var b strings.Builder
b.Grow(estimatedSize) // optional pre-alloc hint
for _, token := range tokens {
    b.WriteString(token)
    b.WriteByte(' ')
}
result := b.String()

// BAD — O(n²) allocations from string immutability
result := ""
for _, token := range tokens {
    result += token + " " // new string allocated every iteration
}
```

**Pattern 5: Stream Large Files, Don't Read into Memory**

```go
// GOOD — constant memory usage regardless of file size
func processLargeFile(path string) error {
    f, err := os.Open(path)
    if err != nil {
        return err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024) // 1MB max line
    for scanner.Scan() {
        processLine(scanner.Bytes()) // process line-by-line
    }
    return scanner.Err()
}

// BAD — loads entire file into RAM
func processLargeFileBad(path string) error {
    data, err := os.ReadFile(path) // 10GB file? 10GB of RAM.
    if err != nil {
        return err
    }
    lines := strings.Split(string(data), "\n")
    // ...
}
```

**Pattern 6: Use `encoding/json.Decoder` with `UseNumber()` for Precision**

```go
dec := json.NewDecoder(r)
dec.UseNumber()  // numbers decode as json.Number (string), not float64

var data map[string]interface{}
dec.Decode(&data)

// Now numbers are preserved exactly
if n, ok := data["id"].(json.Number); ok {
    id, _ := n.Int64()  // no float64 precision loss
}
```

### 4.2 Anti-Patterns — Don't Do These

**Anti-Pattern 1: Indexing Strings by Byte When You Mean Rune**

```go
s := "café"
// BAD — 'é' is 2 bytes, this truncates the string mid-rune
first4 := s[:4] // "caf\xc3" — broken UTF-8!

// GOOD — use rune slice for character-level operations
rs := []rune(s)
first4runes := string(rs[:4]) // "café"
```

**Anti-Pattern 2: Using `scanner.Bytes()` After the Next `Scan()` Call**

```go
scanner := bufio.NewScanner(r)
var allLines [][]byte
for scanner.Scan() {
    // BAD — scanner.Bytes() returns a slice into the scanner's internal
    // buffer. After the next Scan(), the data is overwritten!
    allLines = append(allLines, scanner.Bytes()) // all entries will be the last line

    // GOOD — copy the bytes or use Text() (which copies internally)
    line := make([]byte, len(scanner.Bytes()))
    copy(line, scanner.Bytes())
    allLines = append(allLines, line)
}
```

**Anti-Pattern 3: Ignoring `scanner.Err()`**

```go
// BAD — silently drops I/O errors and buffer overflows
for scanner.Scan() {
    process(scanner.Text())
}
// Missing: if err := scanner.Err(); err != nil { ... }

// Lines longer than the buffer are silently truncated without Err() check.
// bufio.ErrTooLong is the typical error.
```

**Anti-Pattern 4: Regex Where `strings` Functions Suffice**

```go
// BAD — regex for simple containment
if regexp.MustCompile(`error`).MatchString(line) { ... }

// GOOD — 10x faster, zero allocations
if strings.Contains(line, "error") { ... }

// BAD — regex for simple prefix
if regexp.MustCompile(`^ERROR`).MatchString(line) { ... }

// GOOD
if strings.HasPrefix(line, "ERROR") { ... }
```

**Anti-Pattern 5: Unmarshalling JSON/YAML Into `interface{}` When You Know the Schema**

```go
// BAD — type assertions everywhere, runtime panics
var data interface{}
json.Unmarshal(raw, &data)
name := data.(map[string]interface{})["user"].(map[string]interface{})["name"].(string)

// GOOD — compile-time type safety, cleaner code
type User struct {
    Name string `json:"name"`
}
type Response struct {
    User User `json:"user"`
}
var resp Response
json.Unmarshal(raw, &resp)
name := resp.User.Name
```

**Anti-Pattern 6: Building Parsers with `fmt.Sscanf`**

```go
// BAD — fragile, no error detail, poor performance
var host string
var port int
n, _ := fmt.Sscanf(line, "connect %s %d", &host, &port)

// GOOD — explicit, debuggable, handles edge cases
parts := strings.Fields(line)
if len(parts) != 3 || parts[0] != "connect" {
    return fmt.Errorf("invalid connect line: %q", line)
}
host := parts[1]
port, err := strconv.Atoi(parts[2])
```

---

## Part 5 — Deployment

### 5.1 Docker Container

The canonical multi-stage Docker build for a Go text processing tool. Produces a scratch-based image measured in single-digit megabytes.

```dockerfile
# Build stage
FROM golang:1.22-bookworm AS builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -trimpath \
    -o /bin/textproc ./cmd/textproc

# Runtime stage — from scratch, nothing but the binary
FROM scratch

# Copy CA certificates for HTTPS (if the tool fetches URLs)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy timezone data (if the tool parses timestamps)
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

COPY --from=builder /bin/textproc /textproc

ENTRYPOINT ["/textproc"]
```

Build and run:

```bash
# Build
docker build -t textproc:latest .

# Run with stdin pipe
echo '{"msg":"hello"}' | docker run -i textproc:latest tokenize -f json

# Run with mounted file
docker run -v /var/log:/data:ro textproc:latest analyze /data/syslog

# Run with environment variables
docker run -e OUTPUT_FORMAT=json -e LOG_LEVEL=debug textproc:latest scan
```

Alpine variant (when you need a shell for debugging):

```dockerfile
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/textproc ./cmd/textproc

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /bin/textproc /usr/local/bin/textproc
ENTRYPOINT ["textproc"]
```

### 5.2 Standalone Binary

Go compiles to a single static binary. No runtime, no dependencies, no "install Python 3.11 first."

```bash
# Build for current platform
go build -o textproc ./cmd/textproc

# Build static binary explicitly
CGO_ENABLED=0 go build -ldflags="-s -w" -o textproc ./cmd/textproc

# Cross-compile for Linux AMD64 (from macOS or Windows)
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o textproc-linux-amd64 ./cmd/textproc

# Cross-compile for Linux ARM64 (Raspberry Pi, Graviton)
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o textproc-linux-arm64 ./cmd/textproc

# Build with version info baked in
go build -ldflags="-s -w \
  -X main.version=1.2.3 \
  -X main.commit=$(git rev-parse --short HEAD) \
  -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o textproc ./cmd/textproc

# Install to $GOPATH/bin (or $GOBIN)
go install ./cmd/textproc
```

### 5.3 `go run` Scripts

For one-off scripts and pipeline tools, `go run` compiles and executes in one step. In Go 1.22, you can even use `go run` with module-aware single-file programs.

```bash
# Run a single file directly
go run tokenizer.go < input.txt > output.json

# Run a file with external dependencies
# (requires go.mod in the directory)
go run ./cmd/quick-scan/ --input data.yaml

# Run a remote module directly (useful in CI)
go run github.com/user/textproc@latest tokenize < input.txt
```

Single-file script pattern (no go.mod needed for stdlib-only programs):

```go
//go:build ignore
// +build ignore

// Usage: go run tokenize.go < input.txt
package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "strings"
)

func main() {
    scanner := bufio.NewScanner(os.Stdin)
    enc := json.NewEncoder(os.Stdout)
    lineNum := 0
    for scanner.Scan() {
        lineNum++
        tokens := strings.Fields(scanner.Text())
        enc.Encode(map[string]interface{}{
            "line":   lineNum,
            "tokens": tokens,
            "count":  len(tokens),
        })
    }
    if err := scanner.Err(); err != nil {
        fmt.Fprintf(os.Stderr, "error: %v\n", err)
        os.Exit(1)
    }
}
```

### 5.4 Makefile Integration

```makefile
BINARY    := textproc
VERSION   := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE      := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS   := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)
GOFLAGS   := -trimpath

.PHONY: build test lint clean docker run

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" $(GOFLAGS) -o bin/$(BINARY) ./cmd/$(BINARY)

test:
	go test -race -cover ./...

lint:
	golangci-lint run ./...

clean:
	rm -rf bin/

docker:
	docker build -t $(BINARY):$(VERSION) .

run: build
	./bin/$(BINARY)

cross:
	GOOS=linux   GOARCH=amd64 go build -ldflags="$(LDFLAGS)" $(GOFLAGS) -o bin/$(BINARY)-linux-amd64 ./cmd/$(BINARY)
	GOOS=linux   GOARCH=arm64 go build -ldflags="$(LDFLAGS)" $(GOFLAGS) -o bin/$(BINARY)-linux-arm64 ./cmd/$(BINARY)
	GOOS=darwin  GOARCH=arm64 go build -ldflags="$(LDFLAGS)" $(GOFLAGS) -o bin/$(BINARY)-darwin-arm64 ./cmd/$(BINARY)
	sha256sum bin/$(BINARY)-* > bin/checksums.txt
```

---

## Part 6 — Use Cases

### Use Case 1: Structured Log Tokenizer (Standalone Binary)

A tool that reads structured log lines (syslog, JSON, key=value), tokenizes them into normalized records, and outputs JSON. Demonstrates `bufio.Scanner`, custom split functions, regex, and JSON encoding.

**Directory structure:**

```
logtoken/
├── go.mod
├── main.go
├── lexer/
│   └── lexer.go
├── testdata/
│   └── sample.log
└── Makefile
```

**go.mod:**

```
module github.com/example/logtoken

go 1.22
```

**lexer/lexer.go:**

```go
package lexer

import (
    "fmt"
    "regexp"
    "strconv"
    "strings"
    "time"
    "unicode"
)

// LogRecord is a normalized log entry.
type LogRecord struct {
    Timestamp time.Time         `json:"timestamp"`
    Level     string            `json:"level"`
    Message   string            `json:"message"`
    Fields    map[string]string `json:"fields,omitempty"`
    Source    string            `json:"source"`
    Raw       string            `json:"raw"`
}

// Format detection
type LogFormat int

const (
    FormatUnknown LogFormat = iota
    FormatJSON
    FormatKV        // key=value pairs
    FormatSyslog    // traditional syslog
    FormatCommon    // common log format (web servers)
)

var (
    reKV       = regexp.MustCompile(`(\w[\w.-]*)=("(?:[^"\\]|\\.)*"|[^\s,;]+)`)
    reSyslog   = regexp.MustCompile(`^(\w{3}\s+\d+\s+[\d:]+)\s+(\S+)\s+(\S+?)(?:\[(\d+)\])?:\s+(.+)$`)
    reISO      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[T ][\d:.]+Z?`)
    reLevel    = regexp.MustCompile(`(?i)\b(TRACE|DEBUG|INFO|NOTICE|WARN(?:ING)?|ERROR|CRITICAL|FATAL|EMERG(?:ENCY)?|ALERT|PANIC)\b`)
    reCLF      = regexp.MustCompile(`^(\S+)\s+\S+\s+(\S+)\s+\[([^\]]+)\]\s+"(\S+)\s+(\S+)\s+\S+"\s+(\d+)\s+(\d+)`)
)

// DetectFormat guesses the log line format.
func DetectFormat(line string) LogFormat {
    trimmed := strings.TrimSpace(line)
    if len(trimmed) == 0 {
        return FormatUnknown
    }
    if trimmed[0] == '{' {
        return FormatJSON
    }
    if reCLF.MatchString(trimmed) {
        return FormatCommon
    }
    if reSyslog.MatchString(trimmed) {
        return FormatSyslog
    }
    if reKV.MatchString(trimmed) {
        return FormatKV
    }
    return FormatUnknown
}

// TokenizeKV extracts key=value pairs from a log line.
func TokenizeKV(line string) LogRecord {
    rec := LogRecord{
        Fields: make(map[string]string),
        Source: "kv",
        Raw:    line,
    }

    matches := reKV.FindAllStringSubmatch(line, -1)
    for _, m := range matches {
        key := m[1]
        val := m[2]
        // Unquote if needed
        if len(val) >= 2 && val[0] == '"' {
            if unq, err := strconv.Unquote(val); err == nil {
                val = unq
            }
        }
        rec.Fields[key] = val

        switch strings.ToLower(key) {
        case "ts", "time", "timestamp", "t":
            if t, err := time.Parse(time.RFC3339Nano, val); err == nil {
                rec.Timestamp = t
            }
        case "level", "lvl", "severity":
            rec.Level = strings.ToUpper(val)
        case "msg", "message":
            rec.Message = val
        }
    }

    // Fallback: extract level from raw line if not in fields
    if rec.Level == "" {
        if m := reLevel.FindString(line); m != "" {
            rec.Level = strings.ToUpper(m)
        }
    }

    return rec
}

// TokenizeSyslog parses traditional syslog format.
func TokenizeSyslog(line string) LogRecord {
    rec := LogRecord{
        Fields: make(map[string]string),
        Source: "syslog",
        Raw:    line,
    }

    m := reSyslog.FindStringSubmatch(line)
    if m == nil {
        rec.Message = line
        return rec
    }

    // Parse syslog timestamp (assume current year)
    tsStr := m[1]
    if t, err := time.Parse("Jan  2 15:04:05", tsStr); err == nil {
        t = t.AddDate(time.Now().Year(), 0, 0)
        rec.Timestamp = t
    }

    rec.Fields["hostname"] = m[2]
    rec.Fields["program"] = m[3]
    if m[4] != "" {
        rec.Fields["pid"] = m[4]
    }
    rec.Message = m[5]

    // Extract level from message if present
    if lvl := reLevel.FindString(rec.Message); lvl != "" {
        rec.Level = strings.ToUpper(lvl)
    }

    return rec
}

// Tokenize breaks any text into word-level tokens with classification.
type WordToken struct {
    Text     string `json:"text"`
    Type     string `json:"type"` // "word", "number", "ip", "path", "email", "punct"
    Position int    `json:"pos"`
}

var (
    reIP    = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}(:\d+)?$`)
    rePath  = regexp.MustCompile(`^/[\w./-]+$`)
    reEmail = regexp.MustCompile(`^[\w.+-]+@[\w.-]+\.\w+$`)
)

func TokenizeWords(text string) []WordToken {
    var tokens []WordToken
    var current strings.Builder
    startPos := 0

    flush := func(pos int) {
        if current.Len() == 0 {
            return
        }
        word := current.String()
        current.Reset()

        tok := WordToken{Text: word, Position: startPos}
        switch {
        case reIP.MatchString(word):
            tok.Type = "ip"
        case rePath.MatchString(word):
            tok.Type = "path"
        case reEmail.MatchString(word):
            tok.Type = "email"
        case isNumber(word):
            tok.Type = "number"
        default:
            tok.Type = "word"
        }
        tokens = append(tokens, tok)
    }

    for i, r := range text {
        if unicode.IsSpace(r) {
            flush(i)
            startPos = i + 1
        } else if current.Len() == 0 {
            startPos = i
            current.WriteRune(r)
        } else {
            current.WriteRune(r)
        }
    }
    flush(len(text))

    return tokens
}

func isNumber(s string) bool {
    _, err := strconv.ParseFloat(s, 64)
    return err == nil
}

// Describe returns a human-readable summary of token analysis.
func Describe(tokens []WordToken) string {
    counts := make(map[string]int)
    for _, t := range tokens {
        counts[t.Type]++
    }
    var parts []string
    for typ, count := range counts {
        parts = append(parts, fmt.Sprintf("%d %s", count, typ))
    }
    return fmt.Sprintf("%d tokens: %s", len(tokens), strings.Join(parts, ", "))
}
```

**main.go:**

```go
package main

import (
    "bufio"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "os"

    "github.com/example/logtoken/lexer"
)

var version = "dev"

func main() {
    var (
        format  = flag.String("format", "auto", "input format: auto, kv, syslog, words")
        output  = flag.String("output", "json", "output format: json, jsonl, text")
        pretty  = flag.Bool("pretty", false, "pretty-print JSON output")
        ver     = flag.Bool("version", false, "print version and exit")
    )
    flag.Parse()

    if *ver {
        fmt.Println(version)
        os.Exit(0)
    }

    // Determine input source
    var input io.Reader
    if flag.NArg() > 0 {
        f, err := os.Open(flag.Arg(0))
        if err != nil {
            fmt.Fprintf(os.Stderr, "error: %v\n", err)
            os.Exit(1)
        }
        defer f.Close()
        input = f
    } else {
        input = os.Stdin
    }

    scanner := bufio.NewScanner(input)
    scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

    enc := json.NewEncoder(os.Stdout)
    if *pretty {
        enc.SetIndent("", "  ")
    }

    lineNum := 0
    for scanner.Scan() {
        lineNum++
        line := scanner.Text()
        if line == "" {
            continue
        }

        switch *format {
        case "words":
            tokens := lexer.TokenizeWords(line)
            enc.Encode(map[string]interface{}{
                "line":    lineNum,
                "tokens":  tokens,
                "summary": lexer.Describe(tokens),
            })
        case "kv":
            rec := lexer.TokenizeKV(line)
            enc.Encode(rec)
        case "syslog":
            rec := lexer.TokenizeSyslog(line)
            enc.Encode(rec)
        default: // auto
            detected := lexer.DetectFormat(line)
            switch detected {
            case lexer.FormatKV:
                enc.Encode(lexer.TokenizeKV(line))
            case lexer.FormatSyslog:
                enc.Encode(lexer.TokenizeSyslog(line))
            default:
                tokens := lexer.TokenizeWords(line)
                enc.Encode(map[string]interface{}{
                    "line":    lineNum,
                    "tokens":  tokens,
                    "summary": lexer.Describe(tokens),
                })
            }
        }
    }

    if err := scanner.Err(); err != nil {
        fmt.Fprintf(os.Stderr, "read error: %v\n", err)
        os.Exit(1)
    }
}
```

**testdata/sample.log:**

```
ts=2024-01-15T10:30:00Z level=ERROR msg="connection refused" host=db.prod port=5432
ts=2024-01-15T10:30:05Z level=INFO msg="retry successful" host=db.prod attempt=3
Jan 15 10:30:10 web-01 nginx[1234]: 10.0.1.100 GET /api/v1/health 200 0.002
Jan 15 10:30:15 web-01 sshd[5678]: Failed password for invalid user admin from 203.0.113.50
ERROR disk usage at 95% on /dev/sda1 mount=/data server=prod-db-01
```

**Usage:**

```bash
# Auto-detect format
./logtoken < testdata/sample.log

# Force key-value mode
./logtoken --format kv < testdata/sample.log

# Pretty JSON, word tokenization
./logtoken --format words --pretty < testdata/sample.log

# Pipeline: extract only ERROR records
./logtoken < testdata/sample.log | jq 'select(.level == "ERROR")'
```

---

### Use Case 2: Config File Linter (`go run` Script)

A quick-and-dirty script for validating and linting YAML and JSON configuration files. Demonstrates `go run` workflow, YAML/JSON parsing, and validation patterns.

**lint-config.go:**

```go
//go:build ignore

// Usage: go run lint-config.go [--strict] <file.yaml|file.json> ...
//
// Validates configuration files and reports issues.
// Reads from stdin if no files given.
package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "strings"
    "unicode/utf8"

    "gopkg.in/yaml.v3"
)

type Issue struct {
    File     string `json:"file"`
    Line     int    `json:"line,omitempty"`
    Severity string `json:"severity"` // "error", "warning", "info"
    Rule     string `json:"rule"`
    Message  string `json:"message"`
}

type LintResult struct {
    File      string  `json:"file"`
    Valid     bool    `json:"valid"`
    Format    string  `json:"format"` // "yaml", "json"
    Issues    []Issue `json:"issues"`
    KeyCount  int     `json:"key_count"`
    MaxDepth  int     `json:"max_depth"`
}

func main() {
    strict := flag.Bool("strict", false, "enable strict mode (warnings become errors)")
    outputJSON := flag.Bool("json", false, "output results as JSON")
    flag.Parse()

    files := flag.Args()
    if len(files) == 0 {
        files = []string{"-"} // stdin
    }

    exitCode := 0
    var results []LintResult

    for _, file := range files {
        result := lintFile(file, *strict)
        results = append(results, result)
        if !result.Valid {
            exitCode = 1
        }
    }

    if *outputJSON {
        enc := json.NewEncoder(os.Stdout)
        enc.SetIndent("", "  ")
        enc.Encode(results)
    } else {
        for _, r := range results {
            if r.Valid {
                fmt.Printf("✓ %s (%s, %d keys, depth %d)\n",
                    r.File, r.Format, r.KeyCount, r.MaxDepth)
            } else {
                fmt.Printf("✗ %s\n", r.File)
            }
            for _, issue := range r.Issues {
                prefix := "  "
                switch issue.Severity {
                case "error":
                    prefix = "  ✗"
                case "warning":
                    prefix = "  ⚠"
                case "info":
                    prefix = "  ℹ"
                }
                if issue.Line > 0 {
                    fmt.Printf("%s [%s] line %d: %s\n", prefix, issue.Rule, issue.Line, issue.Message)
                } else {
                    fmt.Printf("%s [%s] %s\n", prefix, issue.Rule, issue.Message)
                }
            }
        }
    }

    os.Exit(exitCode)
}

func lintFile(path string, strict bool) LintResult {
    result := LintResult{File: path, Valid: true}

    var data []byte
    var err error

    if path == "-" {
        result.File = "<stdin>"
        data, err = io.ReadAll(os.Stdin)
    } else {
        data, err = os.ReadFile(path)
    }
    if err != nil {
        result.Valid = false
        result.Issues = append(result.Issues, Issue{
            File: result.File, Severity: "error", Rule: "read",
            Message: fmt.Sprintf("cannot read file: %v", err),
        })
        return result
    }

    // Basic checks
    if !utf8.Valid(data) {
        result.Issues = append(result.Issues, Issue{
            File: result.File, Severity: "error", Rule: "encoding",
            Message: "file contains invalid UTF-8",
        })
        result.Valid = false
    }

    // Detect format
    ext := strings.ToLower(filepath.Ext(path))
    trimmed := strings.TrimSpace(string(data))

    switch {
    case ext == ".json" || (ext == "" && len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[')):
        result.Format = "json"
        lintJSON(data, &result, strict)
    case ext == ".yaml" || ext == ".yml" || ext == "":
        result.Format = "yaml"
        lintYAML(data, &result, strict)
    default:
        result.Format = "yaml" // default to YAML
        lintYAML(data, &result, strict)
    }

    return result
}

func lintJSON(data []byte, result *LintResult, strict bool) {
    // Validate JSON
    if !json.Valid(data) {
        result.Valid = false
        result.Issues = append(result.Issues, Issue{
            File: result.File, Severity: "error", Rule: "syntax",
            Message: "invalid JSON syntax",
        })
        return
    }

    // Decode and analyze
    var raw interface{}
    dec := json.NewDecoder(strings.NewReader(string(data)))
    dec.UseNumber()
    if err := dec.Decode(&raw); err != nil {
        result.Valid = false
        result.Issues = append(result.Issues, Issue{
            File: result.File, Severity: "error", Rule: "decode",
            Message: fmt.Sprintf("JSON decode error: %v", err),
        })
        return
    }

    // Analyze structure
    keys, depth := analyzeValue(raw)
    result.KeyCount = keys
    result.MaxDepth = depth

    // Lint rules
    if depth > 10 {
        sev := "warning"
        if strict { sev = "error"; result.Valid = false }
        result.Issues = append(result.Issues, Issue{
            File: result.File, Severity: sev, Rule: "depth",
            Message: fmt.Sprintf("nesting depth %d exceeds recommended maximum of 10", depth),
        })
    }

    // Check for trailing data
    var extra json.Token
    if err := dec.Decode(&extra); err == nil {
        result.Issues = append(result.Issues, Issue{
            File: result.File, Severity: "warning", Rule: "trailing",
            Message: "file contains data after the root JSON value",
        })
    }

    // Check for duplicate keys (basic heuristic via raw parsing)
    checkDuplicateKeys(data, result)
}

func lintYAML(data []byte, result *LintResult, strict bool) {
    // Parse as node tree for position info
    var root yaml.Node
    if err := yaml.Unmarshal(data, &root); err != nil {
        result.Valid = false
        result.Issues = append(result.Issues, Issue{
            File: result.File, Severity: "error", Rule: "syntax",
            Message: fmt.Sprintf("YAML parse error: %v", err),
        })
        return
    }

    // Walk and analyze
    keys, depth := analyzeYAMLNode(&root, 0)
    result.KeyCount = keys
    result.MaxDepth = depth

    // Check for tab characters
    for i, line := range strings.Split(string(data), "\n") {
        if strings.Contains(line, "\t") {
            result.Issues = append(result.Issues, Issue{
                File: result.File, Line: i + 1, Severity: "error", Rule: "tabs",
                Message: "YAML forbids tab characters for indentation",
            })
            result.Valid = false
            break
        }
    }

    // Check depth
    if depth > 10 {
        sev := "warning"
        if strict { sev = "error"; result.Valid = false }
        result.Issues = append(result.Issues, Issue{
            File: result.File, Severity: sev, Rule: "depth",
            Message: fmt.Sprintf("nesting depth %d exceeds recommended maximum of 10", depth),
        })
    }

    // Check for duplicate keys in mappings
    checkYAMLDuplicateKeys(&root, result)

    // Check for truthy strings that YAML might misinterpret
    checkYAMLTruthyStrings(&root, result, strict)
}

func analyzeValue(v interface{}) (keys int, depth int) {
    switch val := v.(type) {
    case map[string]interface{}:
        keys += len(val)
        maxChildDepth := 0
        for _, child := range val {
            ck, cd := analyzeValue(child)
            keys += ck
            if cd > maxChildDepth { maxChildDepth = cd }
        }
        return keys, maxChildDepth + 1
    case []interface{}:
        maxChildDepth := 0
        for _, child := range val {
            ck, cd := analyzeValue(child)
            keys += ck
            if cd > maxChildDepth { maxChildDepth = cd }
        }
        return keys, maxChildDepth + 1
    default:
        return 0, 0
    }
}

func analyzeYAMLNode(node *yaml.Node, depth int) (keys int, maxDepth int) {
    maxDepth = depth
    switch node.Kind {
    case yaml.DocumentNode:
        for _, child := range node.Content {
            ck, cd := analyzeYAMLNode(child, depth)
            keys += ck
            if cd > maxDepth { maxDepth = cd }
        }
    case yaml.MappingNode:
        keys += len(node.Content) / 2
        for i := 1; i < len(node.Content); i += 2 {
            ck, cd := analyzeYAMLNode(node.Content[i], depth+1)
            keys += ck
            if cd > maxDepth { maxDepth = cd }
        }
    case yaml.SequenceNode:
        for _, child := range node.Content {
            ck, cd := analyzeYAMLNode(child, depth+1)
            keys += ck
            if cd > maxDepth { maxDepth = cd }
        }
    }
    return
}

func checkDuplicateKeys(data []byte, result *LintResult) {
    // Simple heuristic: decode into a map and check if key count differs
    // from what a raw parse finds. Full duplicate detection requires a
    // custom JSON parser, but this catches common cases.
}

func checkYAMLDuplicateKeys(node *yaml.Node, result *LintResult) {
    if node.Kind == yaml.MappingNode {
        seen := make(map[string]int)
        for i := 0; i < len(node.Content)-1; i += 2 {
            key := node.Content[i].Value
            if prevLine, ok := seen[key]; ok {
                result.Issues = append(result.Issues, Issue{
                    File: result.File, Line: node.Content[i].Line,
                    Severity: "error", Rule: "duplicate-key",
                    Message: fmt.Sprintf("duplicate key %q (first seen at line %d)", key, prevLine),
                })
                result.Valid = false
            }
            seen[key] = node.Content[i].Line
        }
    }
    for _, child := range node.Content {
        checkYAMLDuplicateKeys(child, result)
    }
}

func checkYAMLTruthyStrings(node *yaml.Node, result *LintResult, strict bool) {
    // YAML 1.1 interprets "yes", "no", "on", "off" as booleans
    truthyValues := map[string]bool{
        "yes": true, "no": true, "on": true, "off": true,
        "Yes": true, "No": true, "On": true, "Off": true,
        "YES": true, "NO": true, "ON": true, "OFF": true,
    }

    var walk func(*yaml.Node)
    walk = func(n *yaml.Node) {
        if n.Kind == yaml.ScalarNode && n.Tag == "!!bool" {
            if _, ambiguous := truthyValues[n.Value]; ambiguous {
                sev := "warning"
                if strict { sev = "error" }
                result.Issues = append(result.Issues, Issue{
                    File: result.File, Line: n.Line,
                    Severity: sev, Rule: "truthy",
                    Message: fmt.Sprintf("%q is interpreted as boolean; quote it if you mean the string", n.Value),
                })
            }
        }
        for _, child := range n.Content {
            walk(child)
        }
    }
    walk(node)
}
```

**Usage:**

```bash
# Lint a YAML file
go run lint-config.go config.yaml

# Lint multiple files in strict mode
go run lint-config.go --strict deployment.yaml service.yaml

# JSON output for CI integration
go run lint-config.go --json --strict *.yaml | jq '.[] | select(.valid == false)'

# Pipe from stdin
cat config.yaml | go run lint-config.go -

# Use in a Makefile target
# lint-configs:
#     find . -name '*.yaml' -exec go run scripts/lint-config.go --strict {} +
```

---

### Use Case 3: CLI Text Analyzer with Viper + Cobra (Full CLI Application)

A production CLI tool built with Cobra (command framework) and Viper (configuration management). Analyzes text files for tokens, patterns, and statistics. Demonstrates the full Viper configuration stack: flags, environment variables, config files, and defaults.

**Directory structure:**

```
textanalyzer/
├── go.mod
├── go.sum
├── cmd/
│   ├── root.go
│   ├── tokenize.go
│   ├── stats.go
│   └── scan.go
├── internal/
│   ├── analyzer/
│   │   └── analyzer.go
│   └── output/
│       └── output.go
├── .textanalyzer.yaml   (example config file)
└── Dockerfile
```

**go.mod:**

```
module github.com/example/textanalyzer

go 1.22

require (
    github.com/spf13/cobra v1.8.0
    github.com/spf13/viper v1.18.2
    gopkg.in/yaml.v3 v3.0.1
)
```

**cmd/root.go:**

```go
package cmd

import (
    "fmt"
    "os"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var cfgFile string

var rootCmd = &cobra.Command{
    Use:   "textanalyzer",
    Short: "Text analysis and tokenization toolkit",
    Long: `textanalyzer is a CLI tool for text processing, tokenization,
and natural language analysis. It reads from files or stdin and
produces structured output in JSON, YAML, or plain text.

Configuration hierarchy (highest priority first):
  1. Command-line flags
  2. Environment variables (TEXTANALYZER_ prefix)
  3. Config file (.textanalyzer.yaml)
  4. Built-in defaults`,
}

func Execute() {
    if err := rootCmd.Execute(); err != nil {
        os.Exit(1)
    }
}

func init() {
    cobra.OnInitialize(initConfig)

    // Global flags
    rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
        "config file (default: .textanalyzer.yaml)")
    rootCmd.PersistentFlags().StringP("output", "o", "text",
        "output format: text, json, yaml")
    rootCmd.PersistentFlags().BoolP("verbose", "v", false,
        "enable verbose output")
    rootCmd.PersistentFlags().Bool("no-color", false,
        "disable colored output")

    // Bind flags to viper
    viper.BindPFlag("output.format", rootCmd.PersistentFlags().Lookup("output"))
    viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
    viper.BindPFlag("output.no_color", rootCmd.PersistentFlags().Lookup("no-color"))

    // Environment variable bindings
    viper.SetEnvPrefix("TEXTANALYZER")
    viper.AutomaticEnv()
}

func initConfig() {
    if cfgFile != "" {
        viper.SetConfigFile(cfgFile)
    } else {
        home, err := os.UserHomeDir()
        if err != nil {
            fmt.Fprintln(os.Stderr, err)
            os.Exit(1)
        }

        // Search in current dir, then home
        viper.AddConfigPath(".")
        viper.AddConfigPath(home)
        viper.SetConfigName(".textanalyzer")
        viper.SetConfigType("yaml")
    }

    // Defaults
    viper.SetDefault("output.format", "text")
    viper.SetDefault("output.no_color", false)
    viper.SetDefault("tokenizer.min_length", 1)
    viper.SetDefault("tokenizer.max_length", 100)
    viper.SetDefault("tokenizer.lowercase", true)
    viper.SetDefault("tokenizer.strip_punctuation", false)
    viper.SetDefault("tokenizer.stopwords", true)
    viper.SetDefault("scanner.buffer_size", 65536)
    viper.SetDefault("stats.top_n", 20)
    viper.SetDefault("verbose", false)

    if err := viper.ReadInConfig(); err != nil {
        if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
            fmt.Fprintf(os.Stderr, "config error: %v\n", err)
            os.Exit(1)
        }
    } else if viper.GetBool("verbose") {
        fmt.Fprintf(os.Stderr, "using config: %s\n", viper.ConfigFileUsed())
    }
}
```

**cmd/tokenize.go:**

```go
package cmd

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "strings"

    "github.com/example/textanalyzer/internal/analyzer"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
    "gopkg.in/yaml.v3"
)

var tokenizeCmd = &cobra.Command{
    Use:   "tokenize [file...]",
    Short: "Tokenize text into structured tokens",
    Long:  "Break input text into classified tokens with position and type information.",
    Args:  cobra.ArbitraryArgs,
    RunE:  runTokenize,
}

func init() {
    tokenizeCmd.Flags().BoolP("lowercase", "l", false, "lowercase all tokens")
    tokenizeCmd.Flags().Bool("no-stopwords", false, "remove stopwords")
    tokenizeCmd.Flags().Int("min-length", 0, "minimum token length (0 = no filter)")
    tokenizeCmd.Flags().String("delimiter", "", "custom delimiter (default: whitespace)")
    tokenizeCmd.Flags().Bool("ngrams", false, "include bigrams and trigrams")

    viper.BindPFlag("tokenizer.lowercase", tokenizeCmd.Flags().Lookup("lowercase"))
    viper.BindPFlag("tokenizer.stopwords_remove", tokenizeCmd.Flags().Lookup("no-stopwords"))
    viper.BindPFlag("tokenizer.min_length", tokenizeCmd.Flags().Lookup("min-length"))

    rootCmd.AddCommand(tokenizeCmd)
}

func runTokenize(cmd *cobra.Command, args []string) error {
    config := analyzer.TokenizerConfig{
        Lowercase:      viper.GetBool("tokenizer.lowercase"),
        RemoveStopwords: viper.GetBool("tokenizer.stopwords_remove"),
        MinLength:      viper.GetInt("tokenizer.min_length"),
    }

    if delim, _ := cmd.Flags().GetString("delimiter"); delim != "" {
        config.Delimiter = delim
    }
    includeNgrams, _ := cmd.Flags().GetBool("ngrams")

    readers, err := openInputs(args)
    if err != nil {
        return err
    }
    defer closeAll(readers)

    outputFormat := viper.GetString("output.format")

    for i, rc := range readers {
        name := "<stdin>"
        if i < len(args) {
            name = args[i]
        }

        scanner := bufio.NewScanner(rc)
        bufSize := viper.GetInt("scanner.buffer_size")
        scanner.Buffer(make([]byte, 0, bufSize), bufSize*16)

        lineNum := 0
        for scanner.Scan() {
            lineNum++
            line := scanner.Text()
            if strings.TrimSpace(line) == "" {
                continue
            }

            result := analyzer.Tokenize(line, config)
            result.File = name
            result.Line = lineNum

            if includeNgrams {
                result.Bigrams = analyzer.Ngrams(result.TokenStrings(), 2)
                result.Trigrams = analyzer.Ngrams(result.TokenStrings(), 3)
            }

            switch outputFormat {
            case "json":
                enc := json.NewEncoder(os.Stdout)
                enc.Encode(result)
            case "yaml":
                data, _ := yaml.Marshal(result)
                fmt.Printf("---\n%s", data)
            default:
                fmt.Printf("%s:%d: %s\n", name, lineNum,
                    strings.Join(result.TokenStrings(), " | "))
            }
        }

        if err := scanner.Err(); err != nil {
            return fmt.Errorf("read %s: %w", name, err)
        }
    }
    return nil
}

func openInputs(args []string) ([]io.ReadCloser, error) {
    if len(args) == 0 {
        return []io.ReadCloser{os.Stdin}, nil
    }
    var readers []io.ReadCloser
    for _, path := range args {
        if path == "-" {
            readers = append(readers, os.Stdin)
            continue
        }
        f, err := os.Open(path)
        if err != nil {
            closeAll(readers)
            return nil, fmt.Errorf("open %s: %w", path, err)
        }
        readers = append(readers, f)
    }
    return readers, nil
}

func closeAll(readers []io.ReadCloser) {
    for _, r := range readers {
        if r != os.Stdin {
            r.Close()
        }
    }
}
```

**cmd/stats.go:**

```go
package cmd

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "sort"
    "strings"

    "github.com/example/textanalyzer/internal/analyzer"
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var statsCmd = &cobra.Command{
    Use:   "stats [file...]",
    Short: "Compute text statistics",
    Long:  "Analyze text and produce frequency distributions, readability scores, and token statistics.",
    Args:  cobra.ArbitraryArgs,
    RunE:  runStats,
}

func init() {
    statsCmd.Flags().IntP("top", "n", 0, "show top N most frequent tokens")
    viper.BindPFlag("stats.top_n", statsCmd.Flags().Lookup("top"))

    rootCmd.AddCommand(statsCmd)
}

func runStats(cmd *cobra.Command, args []string) error {
    readers, err := openInputs(args)
    if err != nil {
        return err
    }
    defer closeAll(readers)

    config := analyzer.TokenizerConfig{
        Lowercase:       true,
        RemoveStopwords: true,
        MinLength:       2,
    }

    var allTokens []string
    totalLines := 0
    totalChars := 0

    for _, rc := range readers {
        scanner := bufio.NewScanner(rc)
        for scanner.Scan() {
            line := scanner.Text()
            totalLines++
            totalChars += len(line)
            result := analyzer.Tokenize(line, config)
            allTokens = append(allTokens, result.TokenStrings()...)
        }
        if err := scanner.Err(); err != nil {
            return err
        }
    }

    // Compute frequency distribution
    freq := make(map[string]int)
    for _, t := range allTokens {
        freq[t]++
    }

    type wordCount struct {
        Word  string `json:"word"`
        Count int    `json:"count"`
        Pct   float64 `json:"pct"`
    }

    var sorted []wordCount
    for w, c := range freq {
        sorted = append(sorted, wordCount{
            Word:  w,
            Count: c,
            Pct:   float64(c) / float64(len(allTokens)) * 100,
        })
    }
    sort.Slice(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })

    topN := viper.GetInt("stats.top_n")
    if topN > 0 && topN < len(sorted) {
        sorted = sorted[:topN]
    }

    // Vocabulary richness: type-token ratio
    ttr := float64(len(freq)) / float64(max(len(allTokens), 1))

    stats := map[string]interface{}{
        "total_lines":      totalLines,
        "total_characters":  totalChars,
        "total_tokens":      len(allTokens),
        "unique_tokens":     len(freq),
        "type_token_ratio":  ttr,
        "top_tokens":        sorted,
    }

    switch viper.GetString("output.format") {
    case "json":
        enc := json.NewEncoder(os.Stdout)
        enc.SetIndent("", "  ")
        enc.Encode(stats)
    default:
        fmt.Printf("Lines:           %d\n", totalLines)
        fmt.Printf("Characters:      %d\n", totalChars)
        fmt.Printf("Total tokens:    %d\n", len(allTokens))
        fmt.Printf("Unique tokens:   %d\n", len(freq))
        fmt.Printf("Type/Token ratio: %.4f\n", ttr)
        fmt.Printf("\nTop %d tokens:\n", len(sorted))
        for i, wc := range sorted {
            bar := strings.Repeat("█", int(wc.Pct*2))
            fmt.Printf("  %3d. %-20s %5d  %5.1f%%  %s\n",
                i+1, wc.Word, wc.Count, wc.Pct, bar)
        }
    }
    return nil
}

func max(a, b int) int {
    if a > b {
        return a
    }
    return b
}
```

**cmd/scan.go:**

```go
package cmd

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
    "strings"
    "text/scanner"

    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var scanCmd = &cobra.Command{
    Use:   "scan [file...]",
    Short: "Lexical scanning of source text",
    Long:  "Perform lexical analysis using Go's text/scanner, identifying identifiers, numbers, strings, and operators.",
    Args:  cobra.ArbitraryArgs,
    RunE:  runScan,
}

func init() {
    scanCmd.Flags().Bool("comments", false, "include comments in output")
    scanCmd.Flags().Bool("positions", true, "include position info")

    rootCmd.AddCommand(scanCmd)
}

type ScanToken struct {
    Type     string `json:"type"`
    Value    string `json:"value"`
    Line     int    `json:"line"`
    Column   int    `json:"col"`
    Filename string `json:"file,omitempty"`
}

func runScan(cmd *cobra.Command, args []string) error {
    readers, err := openInputs(args)
    if err != nil {
        return err
    }
    defer closeAll(readers)

    includeComments, _ := cmd.Flags().GetBool("comments")
    outputFormat := viper.GetString("output.format")

    for i, rc := range readers {
        name := "<stdin>"
        if i < len(args) {
            name = args[i]
        }

        // Read entire input for text/scanner (it needs a string or []byte)
        buf := new(strings.Builder)
        bscanner := bufio.NewScanner(rc)
        bscanner.Buffer(make([]byte, 0, 256*1024), 10*1024*1024)
        for bscanner.Scan() {
            buf.WriteString(bscanner.Text())
            buf.WriteByte('\n')
        }
        if err := bscanner.Err(); err != nil {
            return fmt.Errorf("read %s: %w", name, err)
        }

        var s scanner.Scanner
        s.Init(strings.NewReader(buf.String()))
        s.Filename = name

        mode := scanner.ScanIdents | scanner.ScanInts | scanner.ScanFloats |
            scanner.ScanStrings | scanner.ScanRawStrings | scanner.ScanChars
        if includeComments {
            mode |= scanner.ScanComments
        }
        s.Mode = mode

        var tokens []ScanToken
        for tok := s.Scan(); tok != scanner.EOF; tok = s.Scan() {
            st := ScanToken{
                Value:    s.TokenText(),
                Line:     s.Position.Line,
                Column:   s.Position.Column,
                Filename: name,
            }

            switch tok {
            case scanner.Ident:
                st.Type = "IDENT"
            case scanner.Int:
                st.Type = "INT"
            case scanner.Float:
                st.Type = "FLOAT"
            case scanner.String:
                st.Type = "STRING"
            case scanner.RawString:
                st.Type = "RAWSTRING"
            case scanner.Char:
                st.Type = "CHAR"
            case scanner.Comment:
                st.Type = "COMMENT"
            default:
                st.Type = "PUNCT"
            }

            tokens = append(tokens, st)

            if outputFormat == "text" {
                fmt.Printf("%-8s %-10s %s:%d:%d\n",
                    st.Type, st.Value, st.Filename, st.Line, st.Column)
            }
        }

        if outputFormat == "json" {
            enc := json.NewEncoder(os.Stdout)
            enc.SetIndent("", "  ")
            enc.Encode(map[string]interface{}{
                "file":   name,
                "tokens": tokens,
                "count":  len(tokens),
            })
        }
    }
    return nil
}
```

**internal/analyzer/analyzer.go:**

```go
package analyzer

import (
    "strings"
    "unicode"
)

type TokenizerConfig struct {
    Lowercase       bool
    RemoveStopwords bool
    MinLength       int
    MaxLength       int
    Delimiter       string
    StripPunct      bool
}

type Token struct {
    Text     string `json:"text" yaml:"text"`
    Type     string `json:"type" yaml:"type"`
    Position int    `json:"pos"  yaml:"pos"`
}

type TokenizeResult struct {
    File     string   `json:"file,omitempty"   yaml:"file,omitempty"`
    Line     int      `json:"line,omitempty"   yaml:"line,omitempty"`
    Tokens   []Token  `json:"tokens"           yaml:"tokens"`
    Bigrams  []string `json:"bigrams,omitempty"  yaml:"bigrams,omitempty"`
    Trigrams []string `json:"trigrams,omitempty" yaml:"trigrams,omitempty"`
}

func (r *TokenizeResult) TokenStrings() []string {
    out := make([]string, len(r.Tokens))
    for i, t := range r.Tokens {
        out[i] = t.Text
    }
    return out
}

var stopwords = map[string]bool{
    "a": true, "an": true, "the": true, "is": true, "are": true,
    "was": true, "were": true, "be": true, "been": true, "being": true,
    "have": true, "has": true, "had": true, "do": true, "does": true,
    "did": true, "will": true, "would": true, "could": true, "should": true,
    "may": true, "might": true, "shall": true, "can": true,
    "in": true, "on": true, "at": true, "to": true, "for": true,
    "of": true, "with": true, "by": true, "from": true, "as": true,
    "and": true, "but": true, "or": true, "nor": true, "not": true,
    "so": true, "yet": true, "it": true, "its": true, "this": true,
    "that": true, "these": true, "those": true, "i": true, "me": true,
    "my": true, "we": true, "our": true, "you": true, "your": true,
    "he": true, "she": true, "his": true, "her": true, "they": true,
}

func Tokenize(text string, cfg TokenizerConfig) TokenizeResult {
    var result TokenizeResult
    var current strings.Builder
    startPos := 0

    if cfg.Lowercase {
        text = strings.ToLower(text)
    }

    flush := func(pos int) {
        if current.Len() == 0 {
            return
        }
        word := current.String()
        current.Reset()

        // Apply filters
        if cfg.MinLength > 0 && len(word) < cfg.MinLength {
            return
        }
        if cfg.MaxLength > 0 && len(word) > cfg.MaxLength {
            return
        }
        if cfg.RemoveStopwords && stopwords[word] {
            return
        }

        tok := Token{
            Text:     word,
            Position: startPos,
        }

        // Classify
        switch {
        case isNumeric(word):
            tok.Type = "number"
        case isAllPunct(word):
            tok.Type = "punct"
        default:
            tok.Type = "word"
        }

        result.Tokens = append(result.Tokens, tok)
    }

    if cfg.Delimiter != "" {
        parts := strings.Split(text, cfg.Delimiter)
        pos := 0
        for _, part := range parts {
            trimmed := strings.TrimSpace(part)
            if trimmed != "" {
                startPos = pos
                current.WriteString(trimmed)
                flush(pos)
            }
            pos += len(part) + len(cfg.Delimiter)
        }
        return result
    }

    for i, r := range text {
        if unicode.IsSpace(r) {
            flush(i)
            startPos = i + 1
        } else if cfg.StripPunct && unicode.IsPunct(r) {
            flush(i)
            startPos = i + 1
        } else {
            if current.Len() == 0 {
                startPos = i
            }
            current.WriteRune(r)
        }
    }
    flush(len(text))

    return result
}

func Ngrams(tokens []string, n int) []string {
    if len(tokens) < n {
        return nil
    }
    out := make([]string, 0, len(tokens)-n+1)
    for i := 0; i <= len(tokens)-n; i++ {
        out = append(out, strings.Join(tokens[i:i+n], " "))
    }
    return out
}

func isNumeric(s string) bool {
    for _, r := range s {
        if !unicode.IsDigit(r) && r != '.' && r != '-' && r != '+' {
            return false
        }
    }
    return len(s) > 0
}

func isAllPunct(s string) bool {
    for _, r := range s {
        if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
            return false
        }
    }
    return len(s) > 0
}
```

**internal/output/output.go:**

```go
package output

import (
    "encoding/json"
    "fmt"
    "io"
    "os"

    "gopkg.in/yaml.v3"
)

type Formatter struct {
    Format string
    Writer io.Writer
    Pretty bool
}

func NewFormatter(format string) *Formatter {
    return &Formatter{
        Format: format,
        Writer: os.Stdout,
        Pretty: format == "json",
    }
}

func (f *Formatter) Emit(v interface{}) error {
    switch f.Format {
    case "json":
        enc := json.NewEncoder(f.Writer)
        if f.Pretty {
            enc.SetIndent("", "  ")
        }
        return enc.Encode(v)
    case "yaml":
        data, err := yaml.Marshal(v)
        if err != nil {
            return err
        }
        fmt.Fprintf(f.Writer, "---\n%s", data)
        return nil
    default:
        _, err := fmt.Fprintf(f.Writer, "%v\n", v)
        return err
    }
}
```

**.textanalyzer.yaml (example config file):**

```yaml
# textanalyzer configuration
# Place at .textanalyzer.yaml in your project root or home directory

output:
  format: text        # text, json, yaml
  no_color: false

tokenizer:
  lowercase: true
  min_length: 2
  max_length: 100
  strip_punctuation: false
  stopwords: true

scanner:
  buffer_size: 65536  # 64KB default line buffer

stats:
  top_n: 20           # show top N tokens in stats output
```

**main.go (entrypoint):**

```go
package main

import "github.com/example/textanalyzer/cmd"

func main() {
    cmd.Execute()
}
```

**Usage examples:**

```bash
# Tokenize a file
textanalyzer tokenize README.md

# Word frequency stats with JSON output
textanalyzer stats -o json --top 30 chapter1.txt chapter2.txt

# Lexical scan of a config file
textanalyzer scan --comments config.go

# Tokenize with all filters, custom delimiter
textanalyzer tokenize --lowercase --no-stopwords --min-length 3 --delimiter "," data.csv

# Override config via environment
TEXTANALYZER_OUTPUT_FORMAT=json textanalyzer stats book.txt

# Use config file from custom location
textanalyzer --config /etc/textanalyzer.yaml tokenize /var/log/syslog

# Pipeline with jq
textanalyzer tokenize -o json server.log | jq '[.tokens[] | select(.type == "word")] | length'
```

---

## Part 7 — Appendix

### A.1 Unicode Categories and Go Functions

| Unicode Category | `unicode.Is*` Function | Example Runes | Description |
|---|---|---|---|
| Letter (L) | `IsLetter(r)` | A, é, 漢, Д | Any letter from any alphabet |
| Uppercase (Lu) | `IsUpper(r)` | A, É, Д | Uppercase letters |
| Lowercase (Ll) | `IsLower(r)` | a, é, д | Lowercase letters |
| Digit (Nd) | `IsDigit(r)` | 0-9, ٣, ५ | Decimal digits (any script) |
| Number (N) | `IsNumber(r)` | 0-9, ², ¾, Ⅳ | Digits + letter numbers + other numbers |
| Space (Zs/Zl/Zp) | `IsSpace(r)` | ' ', \t, \n, \r | Whitespace characters |
| Punctuation (P) | `IsPunct(r)` | . , ; : ! ? | Punctuation marks |
| Symbol (S) | `IsSymbol(r)` | $ + = < > | Mathematical, currency, etc. |
| Control (Cc) | `IsControl(r)` | \x00, \x1b | Control characters (C0, C1) |
| Mark (M) | `IsMark(r)` | ◌̀, ◌̂ | Combining marks (accents) |
| Graphic | `IsGraphic(r)` | (most visible) | Letters + marks + numbers + punct + symbols + spaces |
| Print | `IsPrint(r)` | (most visible) | Like Graphic but includes space |

**Script-specific ranges (selected):**

| Script | `unicode.Is(unicode.X, r)` | Range Var |
|---|---|---|
| Latin | `unicode.Is(unicode.Latin, r)` | `unicode.Latin` |
| Han (CJK) | `unicode.Is(unicode.Han, r)` | `unicode.Han` |
| Cyrillic | `unicode.Is(unicode.Cyrillic, r)` | `unicode.Cyrillic` |
| Arabic | `unicode.Is(unicode.Arabic, r)` | `unicode.Arabic` |
| Devanagari | `unicode.Is(unicode.Devanagari, r)` | `unicode.Devanagari` |
| Hiragana | `unicode.Is(unicode.Hiragana, r)` | `unicode.Hiragana` |
| Katakana | `unicode.Is(unicode.Katakana, r)` | `unicode.Katakana` |
| Greek | `unicode.Is(unicode.Greek, r)` | `unicode.Greek` |
| Hangul | `unicode.Is(unicode.Hangul, r)` | `unicode.Hangul` |
| Thai | `unicode.Is(unicode.Thai, r)` | `unicode.Thai` |
| Emoji | `unicode.Is(unicode.So, r)` | (subset of Symbol) |

### A.2 Go String Conversion Reference

| From | To | Method | Copies? |
|---|---|---|---|
| `string` | `[]byte` | `[]byte(s)` | Yes |
| `string` | `[]rune` | `[]rune(s)` | Yes |
| `[]byte` | `string` | `string(bs)` | Yes |
| `[]rune` | `string` | `string(rs)` | Yes |
| `rune` | `string` | `string(r)` | Yes |
| `byte` | `string` | `string(b)` | Yes |
| `int` | `string` | `strconv.Itoa(n)` | — |
| `string` | `int` | `strconv.Atoi(s)` | — |
| `float64` | `string` | `strconv.FormatFloat(f,'f',-1,64)` | — |
| `string` | `float64` | `strconv.ParseFloat(s, 64)` | — |
| `string` | `bool` | `strconv.ParseBool(s)` | — |
| `bool` | `string` | `strconv.FormatBool(b)` | — |
| `string` | `io.Reader` | `strings.NewReader(s)` | No |
| `[]byte` | `io.Reader` | `bytes.NewReader(bs)` | No |

### A.3 Common Regex Patterns for DevOps

| Pattern | Regex | Description |
|---|---|---|
| IPv4 address | `\b(\d{1,3}\.){3}\d{1,3}\b` | Matches IPv4 addresses |
| IPv4:port | `(\d{1,3}\.){3}\d{1,3}:\d{1,5}` | IP with port number |
| ISO 8601 timestamp | `\d{4}-\d{2}-\d{2}T[\d:.]+Z?` | ISO datetime |
| Syslog timestamp | `\w{3}\s+\d+\s+[\d:]+` | BSD syslog date |
| HTTP method | `\b(GET\|POST\|PUT\|DELETE\|PATCH\|HEAD\|OPTIONS)\b` | HTTP verbs |
| HTTP status | `\b[1-5]\d{2}\b` | 3-digit HTTP codes |
| UUID | `[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}` | UUID v4 |
| Email | `[\w.+-]+@[\w.-]+\.\w{2,}` | Basic email match |
| URL | `https?://\S+` | HTTP(S) URLs |
| File path | `/[\w./-]+` | Unix file paths |
| Key=Value | `(\w+)=("[^"]*"\|\S+)` | Key-value pairs |
| Docker image | `[\w./-]+:[\w.-]+` | image:tag format |
| Git SHA | `\b[0-9a-f]{7,40}\b` | Short or full SHA |
| Semantic version | `v?\d+\.\d+\.\d+(-[\w.]+)?` | SemVer |
| CIDR | `(\d{1,3}\.){3}\d{1,3}/\d{1,2}` | IPv4 CIDR notation |
| Kubernetes pod | `[\w-]+-[a-z0-9]{5,10}-[a-z0-9]{5}` | K8s pod naming |

### A.4 `text/scanner` Token Constants

| Constant | Value | Meaning |
|---|---|---|
| `scanner.EOF` | -1 | End of input |
| `scanner.Ident` | -2 | Identifier |
| `scanner.Int` | -3 | Integer literal |
| `scanner.Float` | -4 | Float literal |
| `scanner.Char` | -5 | Character literal |
| `scanner.String` | -6 | Double-quoted string |
| `scanner.RawString` | -7 | Backtick-quoted raw string |
| `scanner.Comment` | -8 | Comment |
| `> 0` | rune | Any single rune (punctuation, operator, etc.) |

### A.5 `encoding/json` Token Types

| Go Type from `Token()` | JSON Element |
|---|---|
| `json.Delim` ('{') | Object start |
| `json.Delim` ('}') | Object end |
| `json.Delim` ('[') | Array start |
| `json.Delim` (']') | Array end |
| `string` | String value (or object key) |
| `float64` | Number (default) |
| `json.Number` | Number (with `UseNumber()`) |
| `bool` | Boolean |
| `nil` | Null |

### A.6 Performance Characteristics

| Operation | Time Complexity | Allocation | Notes |
|---|---|---|---|
| `strings.Contains` | O(n) | 0 | Rabin-Karp for long patterns |
| `strings.Index` | O(n) | 0 | — |
| `strings.Split` | O(n) | O(k) slices | k = number of splits |
| `strings.Fields` | O(n) | O(k) slices | Splits on whitespace runs |
| `strings.Builder.WriteString` | O(1) amortized | 0 (amortized) | Pre-allocate with `Grow()` |
| `string([]byte)` | O(n) | O(n) | Always copies |
| `[]byte(string)` | O(n) | O(n) | Always copies |
| `regexp.MatchString` | O(n) | varies | RE2: linear, guaranteed |
| `bufio.Scanner.Scan` | O(line) | 0 (reuses buffer) | `Bytes()` is volatile |
| `json.Decoder.Decode` | O(n) | O(n) for result struct | Streaming: no full load |
| `json.Decoder.Token` | O(token) | O(1) | Lexer-level streaming |
| `yaml.Unmarshal` | O(n) | O(n) | Full parse required |

### A.7 Standard Library Import Paths

```
strings             — string manipulation
bytes               — byte slice manipulation
strconv             — string ↔ numeric conversion
unicode             — rune classification
unicode/utf8        — UTF-8 encoding/decoding
bufio               — buffered I/O and scanning
text/scanner        — lexical scanning
go/scanner          — Go source lexical scanning
go/token            — Go token types
go/ast              — Go abstract syntax tree
go/parser           — Go source parser
regexp              — RE2 regular expressions
regexp/syntax       — regex parse trees
encoding/json       — JSON encode/decode
encoding/csv        — CSV reading/writing
encoding/binary     — binary encode/decode
encoding/hex        — hex encode/decode
encoding/base64     — base64 encode/decode
io                  — core I/O interfaces
os                  — file I/O, stdin/stdout
fmt                 — formatted I/O
sort                — sorting
math                — math functions
path/filepath       — file path manipulation
text/template       — text templating
html/template       — HTML-safe templating
```

**External packages (commonly used):**

```
gopkg.in/yaml.v3                — YAML 1.2 parsing
github.com/spf13/cobra          — CLI command framework
github.com/spf13/viper          — configuration management
github.com/spf13/pflag          — POSIX/GNU-style flags
golang.org/x/text/transform     — text transforms
golang.org/x/text/unicode/norm  — Unicode normalization (NFC, NFD, NFKC, NFKD)
golang.org/x/text/language      — language tags
golang.org/x/text/cases         — case mapping (proper Title, Upper, Lower)
golang.org/x/text/encoding      — character encoding (Shift-JIS, GBK, etc.)
github.com/BurntSushi/toml      — TOML parsing
github.com/pelletier/go-toml/v2 — TOML v2 parsing
```

---

*Built with the spirit of Unix, the type safety of Go, and the relentless pragmatism of Plan 9. Every tool in this manual reads from stdin, writes to stdout, and composes with pipes. As it should be.*

*— The Ghost of Bell Labs, 2024*
