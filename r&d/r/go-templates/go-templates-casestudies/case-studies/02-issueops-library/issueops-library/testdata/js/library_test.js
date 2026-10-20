// Run static/library.js against a stub DOM built from a catalog page's
// data-search attributes; print which rows each query leaves visible.
const fs = require('fs');
const src = fs.readFileSync(process.argv[2], 'utf8');
const page = fs.readFileSync(process.argv[3], 'utf8');
const rows = [...page.matchAll(/<tr class="(?:odd|even)" data-search="([^"]*)"/g)]
  .map(m => ({ hidden: false, getAttribute: k => (k === 'data-search' ? m[1] : null), id: m[1] }));
let handler;
const q = { value: '', getAttribute: () => '#catalog tbody tr', addEventListener: (t, f) => { handler = f; } };
const count = { textContent: '' };
global.location = { hash: '' };
global.document = {
  readyState: 'complete',
  querySelector: () => q,
  querySelectorAll: () => rows,
  getElementById: () => count,
};
eval(src);
for (const term of ['', 'caddy', 'payments 42', 'NOMAD', 'zzz']) {
  q.value = term;
  handler();
  const shown = rows.filter(r => !r.hidden).map(r => r.id);
  console.log(JSON.stringify(term) + ' -> ' + shown.length + ' [' + shown.join(' ') + '] "' + count.textContent + '"');
}
