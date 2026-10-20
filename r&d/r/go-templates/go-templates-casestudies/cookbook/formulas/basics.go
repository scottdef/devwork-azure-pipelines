package formulas

import "time"

// basics is F01..F13: the small tools every template needs on day one.
func basics() []Formula {
	return []Formula{
		{
			ID:      "F01",
			Name:    "dict: several values through one template call",
			Problem: "`{{template \"x\" a b}}` takes exactly one argument, so a partial that needs a kind, a text and an optional size has nothing to receive them in.",
			Template: `{{- define "badge" -}}
<{{ .Kind }}:{{ .Text }}{{ if .Size }}:{{ .Size }}{{ end }}>
{{- end -}}
{{ template "badge" (dict "Kind" "info" "Text" .Msg) }}
{{ template "badge" (dict "Kind" "warn" "Text" .Msg "Size" "lg") }}
`,
			Data: M{"Msg": "disk 91% full"},
			Want: `<info:disk 91% full>
<warn:disk 91% full:lg>
`,
			Notes: "`dict` packs alternating keys and values into a `map[string]any`, which is the one argument `template` accepts. " +
				"A key you leave out reads as the zero value, so `{{ if .Size }}` is the optional-argument test. " +
				"An odd number of arguments or a non-string key is an error at execution time (see F53), not a silent drop. " +
				"`$` inside the partial is the dict, not the caller's data: pass everything the partial needs.",
		},
		{
			ID:      "F02",
			Name:    "list: a slice literal",
			Problem: "Templates have no `[...]` syntax, so a fixed set of values has to be declared in the data or hard-coded into repeated text.",
			Template: `{{ $envs := list "dev" "stage" "prod" -}}
count={{ len $envs }} second={{ index $envs 1 }} tail={{ slice $envs 1 }}
mixed={{ list 1 "a" true }}
{{ range $envs }}- {{ . }}
{{ end -}}
`,
			Want: `count=3 second=stage tail=[stage prod]
mixed=[1 a true]
- dev
- stage
- prod
`,
			Notes: "`list` returns a `[]any`, so every builtin that takes a slice works: `len`, `index`, `slice`, `range`, `join`. " +
				"Elements are `any`: comparing one with `eq` needs matching types (`eq (index $l 0) \"dev\"` is fine, `eq (index $l 0) 1` is an error).",
		},
		{
			ID:      "F03",
			Name:    "default: a value when the field is empty",
			Problem: "An unset field prints as nothing (or `<no value>`), and the config renders with holes in it.",
			Template: `name={{ default "anonymous" .Name }}
port={{ default 8080 .Port }}
host={{ default "localhost" .Host }}
debug={{ default "off" .Debug }}
`,
			Data: M{"Name": "", "Port": 0, "Host": "example.org"},
			Want: `name=anonymous
port=8080
host=example.org
debug=off
`,
			Notes: "The value is the **last** argument so it pipes: `{{ .Name | default \"anonymous\" }}`. " +
				"*Empty* means the zero value: `\"\"`, `0`, `false`, nil, an empty slice or map. The trap is `Port: 0`: a port the author set deliberately to 0 is replaced by 8080, because `default` cannot tell 'set to zero' from 'unset'. " +
				"When zero is meaningful, carry a pointer in the data (`*int`): nil is unset, a pointer to 0 is not. A missing map key reaches `default` as nil, which is why `.Debug` works.",
		},
		{
			ID:      "F04",
			Name:    "coalesce: first non-empty of several",
			Problem: "A display name falls back from nickname to name to email; nested `if/else` chains for that are noisy.",
			Template: `who={{ coalesce .Nick .Name .Email "unknown" }}
retries={{ coalesce .Retries .Fallback }}
`,
			Data: M{"Nick": "", "Name": "", "Email": "rob@example.org", "Retries": 0, "Fallback": 3},
			Want: `who=rob@example.org
retries=3
`,
			Notes: "`coalesce` is `default` for a chain. It uses the same emptiness, so `Retries: 0` is skipped. If every argument is empty it returns nil, which prints as `<no value>`: end the chain with a literal.",
		},
		{
			ID:      "F05",
			Name:    "ternary: if/else as an expression",
			Problem: "A one-word choice in the middle of a line needs a five-token `if/else/end` that breaks up the text.",
			Template: `{{ if .Prod }}https{{ else }}http{{ end }}://example.org
{{ ternary "https" "http" .Prod }}://example.org
{{ ternary "https" "http" (eq .Env "prod") }}://example.org
`,
			Data: M{"Prod": true, "Env": "stage"},
			Want: `https://example.org
https://example.org
http://example.org
`,
			Notes: "The condition is the last argument. `ternary` is a **function**: both branches are evaluated before it is called. " +
				"`{{ ternary .Cfg.Port 80 .HasCfg }}` with a nil `.Cfg` fails with a nil-pointer error even when `.HasCfg` is false, because `.Cfg.Port` was evaluated anyway. " +
				"Use `if` whenever a branch can fail or has side effects; `and`/`or` short-circuit since Go 1.18.",
		},
		{
			ID:      "F06",
			Name:    "Join struct fields with a comma rule",
			Problem: "`join` wants a slice of strings, but the names live in a field of each struct, and a trailing comma after the last one is a bug.",
			Template: `{{ range $i, $u := .Users }}{{ if $i }}, {{ end }}{{ $u.Name }}{{ end }}
{{ range $i, $u := .Users }}{{ if $i }}{{ if eq $i (sub (len $.Users) 1) }} and {{ else }}, {{ end }}{{ end }}{{ $u.Name }}{{ end }}
`,
			Data: M{"Users": L{M{"Name": "ada"}, M{"Name": "grace"}, M{"Name": "ken"}}},
			Want: `ada, grace, ken
ada, grace and ken
`,
			Notes: "`{{ if $i }}, {{ end }}` writes the separator **before** every item except the first (`$i` is 0, which is false). " +
				"That is simpler than detecting the last item. The second line shows the 'a, b and c' variant, which does need the length; note `$.Users`, because inside `range` the dot has moved. " +
				"When the data is already `[]string`, `join` is shorter: `{{ join \", \" .Names }}`.",
		},
		{
			ID:      "F07",
			Name:    "Iterate a map in sorted key order",
			Problem: "Go randomises map iteration, so you expect generated files to differ on every run and every diff to be noise.",
			Template: `{{ range $k, $v := .Env }}{{ $k }}={{ $v }}
{{ end -}}
keys: {{ join "," (sortedKeys .Env) }}
`,
			Data: M{"Env": M{"ZONE": "a", "HOME": "/h", "APP": "x"}},
			Want: `APP=x
HOME=/h
ZONE=a
keys: APP,HOME,ZONE
`,
			Notes: "Good news: `range` over a map in a template **already visits keys in sorted order** (for string, integer and float keys), unlike a Go `for` loop, so the first two lines are deterministic. " +
				"You need `sortedKeys` only when you want the slice itself: to join it, to take the first key, or to index the map with `index .Env (index $keys 0)`. " +
				"It never helps with struct fields, which keep declaration order.",
		},
		{
			ID:      "F08",
			Name:    "Nested range with $ to reach the root",
			Problem: "Inside a nested `range` the dot is the inner element, and the organisation name from the top is out of reach.",
			Template: `{{ range .Teams }}{{ $team := . }}{{ range .Members }}{{ $.Org }}/{{ $team.Name }}: {{ . }}
{{ end }}{{ end -}}
`,
			Data: M{"Org": "CoolGitOrg", "Teams": L{
				M{"Name": "platform", "Members": L{"ada", "ken"}},
				M{"Name": "data", "Members": L{"grace"}},
			}},
			Want: `CoolGitOrg/platform: ada
CoolGitOrg/platform: ken
CoolGitOrg/data: grace
`,
			Notes: "`$` is the data passed to `Execute`: it always reaches the root. To keep an outer *loop* element, bind it with `$team := .` before descending. " +
				"The trap: inside a `{{ define }}` block `$` is the argument of that `template` call, not the root of the page (see F20).",
		},
		{
			ID:      "F09",
			Name:    "1-based index arithmetic",
			Problem: "`range` indexes from 0 and humans count from 1; numbered lists end up starting at 0.",
			Template: `{{ range $i, $x := .Items }}{{ add $i 1 }}/{{ len $.Items }} {{ $x }}
{{ end -}}
`,
			Data: M{"Items": []string{"lint", "test", "build"}},
			Want: `1/3 lint
2/3 test
3/3 build
`,
			Notes: "Templates have no `+` operator; arithmetic is a function call. `add`, `sub`, `mul`, `div` and `mod` come from the shared FuncMap and accept any integer type, plus integral floats because JSON numbers arrive as `float64`. For plain loops `{{ range seq 3 }}` yields 1, 2, 3 directly.",
		},
		{
			ID:      "F10",
			Name:    "Zebra stripes with mod",
			Problem: "Alternating row classes need the parity of the loop index, and there is no `%` in templates.",
			Template: `<table>
{{- range $i, $r := .Rows }}
  <tr class="{{ if eq (mod $i 2) 0 }}even{{ else }}odd{{ end }}"><td>{{ $r }}</td></tr>
{{- end }}
</table>
`,
			Engine: "html",
			Data:   M{"Rows": []string{"ada", "grace", "ken", "dennis"}},
			Want: `<table>
  <tr class="even"><td>ada</td></tr>
  <tr class="odd"><td>grace</td></tr>
  <tr class="even"><td>ken</td></tr>
  <tr class="odd"><td>dennis</td></tr>
</table>
`,
			Notes: "`mod` takes the dividend first: `mod $i 2`. The pipe form `{{ $i | mod 2 }}` means `mod 2 $i`, which is 2 mod i, and is wrong in a way that looks plausible for i = 0..2. " +
				"`ternary \"even\" \"odd\" (eq (mod $i 2) 0)` is the one-liner. Better still: do it in CSS with `tr:nth-child(even)` and leave the template out of it.",
		},
		{
			ID:      "F11",
			Name:    "Pluralisation",
			Problem: "'1 files' and '0 file' look careless in a report.",
			Template: `{{ range .Counts }}{{ . }} {{ plural . "file" "files" }}
{{ end -}}
`,
			Data: M{"Counts": []int{0, 1, 2, 11}},
			Want: `0 files
1 file
2 files
11 files
`,
			Notes: "`plural n one other` returns `one` only when `n == 1`; zero is plural in English (\"0 files\"). " +
				"This is English-only. Languages like Polish or Arabic have several plural categories chosen by CLDR rules, and no two-way function can express them: use a real message catalogue for user-facing text.",
		},
		{
			ID:      "F12",
			Name:    "time.Time: Format and friends",
			Problem: "Dates arrive as `time.Time` and need a specific layout; people try `strftime` codes and get nonsense.",
			Template: `iso:     {{ .When.Format "2006-01-02T15:04:05Z07:00" }}
short:   {{ .When.Format "Mon Jan _2 3:04PM" }}
helper:  {{ dateFmt "02 Jan 2006" .When }}
weekday: {{ .When.Weekday }}
next:    {{ (.When.AddDate 0 0 7).Format "2006-01-02" }}
unix:    {{ .When.Unix }}
`,
			Data: M{"When": time.Date(2024, 3, 5, 14, 30, 0, 0, time.UTC)},
			Want: `iso:     2024-03-05T14:30:00Z
short:   Tue Mar  5 2:30PM
helper:  05 Mar 2024
weekday: Tuesday
next:    2024-03-12
unix:    1709649000
`,
			Notes: "Go layouts are written as the reference time `Mon Jan 2 15:04:05 MST 2006` (1 2 3 4 5 6 7 in order); `%Y-%m-%d` means nothing. " +
				"Methods are called straight from the template, and arguments follow the method name: `.When.AddDate 0 0 7`. Parenthesise to chain: `(.When.AddDate 0 0 7).Format \"...\"`. " +
				"`_2` pads the day with a space. Data decoded from JSON has no `time.Time` (it is a string); `dateFmt` also accepts RFC 3339 strings and Unix seconds for that case.",
		},
		{
			ID:      "F13",
			Name:    "printf: number formatting",
			Problem: "Money, hex, padding and percentages come out as raw `%v` unless you format them.",
			Template: `pi:      {{ printf "%08.3f" .Pi }}
columns: [{{ printf "%5d|%-5d" 42 42 }}]
bases:   {{ printf "%x %o %b" 255 255 5 }}
quoted:  {{ printf "%q" .Name }}
percent: {{ printf "%.1f%%" .Ratio }}
width:   [{{ printf "%-*s" 8 .Name }}]
`,
			Data: M{"Pi": 3.14159265, "Name": "ken", "Ratio": 87.46},
			Want: `pi:      0003.142
columns: [   42|42   ]
bases:   ff 377 101
quoted:  "ken"
percent: 87.5%
width:   [ken     ]
`,
			Notes: "`printf` is `fmt.Sprintf`, verbs and all, including `*` for a computed width. A literal percent is `%%`. " +
				"The verb must match the type: `%d` with a `float64` prints `%!d(float64=...)` instead of failing, which is how JSON-decoded numbers bite. Thousands separators are not in `fmt`; format those in Go.",
		},
	}
}
