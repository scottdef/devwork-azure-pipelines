// library.js: client-side catalog filter. Raw DOM, no framework: every row
// carries a lowercase data-search string; typing hides rows that do not
// contain every word typed. The page works unfiltered without JS.
(function () {
  "use strict";
  function init() {
    var q = document.querySelector("input[data-filter]");
    if (!q) return;
    var rows = document.querySelectorAll(q.getAttribute("data-filter"));
    var count = document.getElementById("q-count");
    function run() {
      var words = q.value.toLowerCase().split(/\s+/).filter(Boolean), shown = 0;
      for (var i = 0; i < rows.length; i++) {
        var hay = rows[i].getAttribute("data-search");
        if (hay === null) continue; // "empty library" row
        var ok = true;
        for (var w = 0; w < words.length && ok; w++) ok = hay.indexOf(words[w]) >= 0;
        rows[i].hidden = !ok;
        if (ok) shown++;
      }
      count.textContent = words.length ? shown + " shown" : "";
    }
    q.addEventListener("input", run);
    var h = decodeURIComponent(location.hash.slice(1));
    if (h.indexOf("q=") === 0) { q.value = h.slice(2); run(); }
  }
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init);
  else init();
})();
