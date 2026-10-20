// tmplforge GUI: the only hand-written script. htmx does the requests;
// this removes variable rows and tells htmx when the fixture changed.
// Served from 'self' so it passes the CSP; no inline handlers anywhere.
"use strict";
document.addEventListener("click", function (e) {
  var b = e.target.closest("button.rm");
  if (!b) return;
  var tr = b.closest("tr");
  if (tr) tr.remove();
  refresh();
});
document.addEventListener("htmx:afterSwap", function (e) {
  // A new skeleton fixture was swapped in: refresh the preview.
  if (e.detail.target && e.detail.target.id === "fixture-wrap") {
    refresh();
  }
});
function refresh() {
  var p = document.getElementById("preview");
  if (p && window.htmx) htmx.trigger(p, "refresh");
}
