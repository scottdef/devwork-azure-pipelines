// lab.js: load render.wasm and call the Go function it registers.
(function () {
  "use strict";
  var btn = document.getElementById("lab-go");
  var out = document.getElementById("lab-out");
  if (typeof Go === "undefined") { out.textContent = "wasm_exec.js missing: lab not built."; return; }
  var go = new Go();
  WebAssembly.instantiateStreaming(fetch("render.wasm"), go.importObject).then(function (r) {
    go.run(r.instance);
    btn.disabled = false;
    btn.textContent = "Render";
  }).catch(function (e) { out.textContent = "wasm failed to load: " + e; });
  document.getElementById("lab").addEventListener("submit", function (ev) {
    ev.preventDefault();
    var r = window.renderTemplate(document.getElementById("lab-tmpl").value,
      document.getElementById("lab-data").value);
    out.className = r.err ? "out bad" : "out";
    out.textContent = r.err || r.out;
  });
})();
