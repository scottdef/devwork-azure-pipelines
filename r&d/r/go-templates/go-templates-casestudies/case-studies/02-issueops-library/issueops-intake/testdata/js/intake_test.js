// Minimal DOM stub to exercise intake.js's check/build/guard paths.
const fs = require('fs');
const src = fs.readFileSync(process.argv[2], "utf8");
function el(attrs, value) {
  return { value, required: !!attrs.required, pattern: attrs.pattern || '', maxLength: attrs.maxlength || -1,
    getAttribute: k => attrs[k] === undefined ? null : attrs[k], tagName: 'INPUT' };
}
function field(id, type, prefill) { return { getAttribute: k => ({'data-id': id, 'data-type': type, 'data-prefill': prefill ? '1' : null})[k] }; }
function mkForm(payload, sourceId) {
  const els = {
    'source-id': el({required: 1, pattern: '[a-z0-9][a-z0-9-]*[a-z0-9]', maxlength: 48}, sourceId),
    'template-id': el({required: 1}, 'caddyfile'),
    'payload': el({required: 1, 'data-json': '1', maxlength: 32768}, payload),
    'notes': el({}, ''),
  };
  const fields = [field('source-id','input',1), field('template-id','dropdown',0), field('payload','textarea',1), field('notes','textarea',1)];
  const pre = { textContent: '', className: '' };
  return { elements: els, pre,
    getAttribute: k => ({'data-template': 'request.yml', 'data-title': '[request] ', 'data-title-fields': 'source-id template-id'})[k],
    querySelector: s => pre,
    querySelectorAll: s => s === '.field' ? fields : fields.filter(f => f.getAttribute('data-prefill')),
    addEventListener: (t, f) => { this.handler = f; } };
}
let forms = [], loc = { href: '' };
global.window = { location: loc };
global.document = {
  body: { getAttribute: k => ({'data-base': 'https://github.com/CoolGitOrg/issueops-intake/issues/new', 'data-max': '7000', 'data-flags': './flags.json'})[k] },
  querySelectorAll: s => s === 'form.intake' ? forms : [],
  getElementById: () => ({ hidden: true }),
};
global.XMLHttpRequest = function () { this.open = () => {}; this.send = () => {}; };
function run(payload, sourceId) {
  const f = mkForm(payload, sourceId); let h;
  f.addEventListener = (t, fn) => { h = fn; };
  forms = [f]; loc.href = '';
  eval(src);
  h({ preventDefault() {}, target: f });
  return { url: loc.href, msg: f.pre.textContent, bad: f.pre.className };
}
let r = run('{ "name": "payments-api", "port": 8443 }', 'payments-api');
console.log('OK   ', r.url);
r = run('{"name":', 'payments-api');            console.log('JSON ', r.bad, '|', r.msg.split('\n')[0]);
r = run('[1,2]', 'payments-api');                 console.log('ARR  ', r.bad, '|', r.msg);
r = run('{}', 'Bad_ID');                          console.log('PAT  ', r.bad, '|', r.msg);
r = run(JSON.stringify({blob: 'x'.repeat(8000)}), 'payments-api'); console.log('LONG ', r.bad, '|', r.msg.split('\n')[0], '| navigated:', r.url !== '');
