#!/usr/bin/env bash
# wasm_test.sh: build the lab wasm and call renderTemplate under node.
# Skips (exit 0) when node is absent. Used by scripts/preflight.sh.
set -euo pipefail
cd "$(dirname "$0")/.."
command -v node >/dev/null 2>&1 || { echo "   node not installed: skipping wasm run"; exit 0; }
d="$(mktemp -d)"
trap 'rm -rf "$d"' EXIT
lab/build.sh "$d" >/dev/null
cat >"$d/run.js" <<'JS'
require(process.argv[2]);
const go = new Go();
WebAssembly.instantiate(require('fs').readFileSync(process.argv[3]), go.importObject).then(r => {
  go.run(r.instance);
  const ok = globalThis.renderTemplate('{{.name}}:{{.port | default "8080"}}', '{"name":"svc"}');
  const bad = globalThis.renderTemplate('{{.x', '{}');
  if (ok.out !== 'svc:8080' || ok.err !== '' || !bad.err.includes('unclosed action')) {
    console.error('wasm: unexpected', JSON.stringify(ok), JSON.stringify(bad)); process.exit(1);
  }
  console.log('   wasm renderTemplate ok under node ' + process.version);
  process.exit(0);
});
JS
node "$d/run.js" "$d/lab/wasm_exec.js" "$d/lab/render.wasm"
