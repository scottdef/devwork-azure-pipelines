// intake.js: turn a form into a prefilled GitHub issue URL. No network
// calls except reading flags.json from this same static site.
// GitHub prefills issue-form inputs and textareas from ?<field-id>=value;
// dropdowns and checkboxes are not reliably prefilled, so they are not sent.
(function () {
  "use strict";
  var b = document.body;
  var cfg = {base: b.getAttribute("data-base"), max: +b.getAttribute("data-max"), flags: b.getAttribute("data-flags")};
  var enabled = true;

  function out(form, msg, bad) {
    var pre = form.querySelector(".out");
    pre.textContent = msg;
    pre.className = bad ? "out bad" : "out";
  }

  function val(form, id) {
    var el = form.elements[id];
    return el && el.value !== undefined ? el.value.trim() : "";
  }

  function check(form) {
    var errs = [];
    var fields = form.querySelectorAll(".field");
    for (var i = 0; i < fields.length; i++) {
      var f = fields[i], id = f.getAttribute("data-id");
      var el = form.elements[id];
      if (!el || el.length !== undefined && !el.tagName) continue; // checkbox groups
      var v = (el.value || "").trim();
      if (el.required && v === "") errs.push(id + ": required");
      else if (v !== "" && el.pattern && !new RegExp("^(?:" + el.pattern + ")$").test(v))
        errs.push(id + ": does not match " + el.pattern);
      if (el.maxLength > 0 && v.length > el.maxLength)
        errs.push(id + ": longer than " + el.maxLength);
      if (v !== "" && el.getAttribute("data-json")) {
        try {
          var j = JSON.parse(v);
          if (j === null || typeof j !== "object" || Array.isArray(j))
            errs.push(id + ": must be a JSON object");
        } catch (e) {
          errs.push(id + ": JSON does not parse: " + e.message);
        }
      }
    }
    return errs;
  }

  function build(form) {
    var q = ["template=" + encodeURIComponent(form.getAttribute("data-template"))];
    var parts = [];
    var tf = (form.getAttribute("data-title-fields") || "").split(" ");
    for (var i = 0; i < tf.length; i++) if (tf[i] && val(form, tf[i])) parts.push(val(form, tf[i]));
    q.push("title=" + encodeURIComponent(form.getAttribute("data-title") + parts.join(" ")));
    var fields = form.querySelectorAll(".field[data-prefill]");
    for (var k = 0; k < fields.length; k++) {
      var id = fields[k].getAttribute("data-id");
      var v = val(form, id);
      if (v === "") continue;
      // Compact JSON so the URL stays short; the parser re-reads it anyway.
      if (form.elements[id].getAttribute("data-json")) v = JSON.stringify(JSON.parse(v));
      q.push(encodeURIComponent(id) + "=" + encodeURIComponent(v));
    }
    return cfg.base + "?" + q.join("&");
  }

  function submit(ev) {
    ev.preventDefault();
    var form = ev.target;
    if (!enabled) { out(form, "Submissions are paused.", true); return; }
    var errs = check(form);
    if (errs.length) { out(form, errs.join("\n"), true); return; }
    var url = build(form);
    if (url.length > cfg.max) {
      out(form, "The prefilled link would be " + url.length + " characters (limit " + cfg.max +
        ").\nOpen the plain form and attach the payload as a file or a gist link instead.", true);
      return;
    }
    out(form, "Opening GitHub...\n" + url.slice(0, 200) + (url.length > 200 ? "..." : ""));
    window.location.href = url;
  }

  var forms = document.querySelectorAll("form.intake");
  for (var i = 0; i < forms.length; i++) forms[i].addEventListener("submit", submit);

  // Beta gate: flags.json is published with the site; a missing or broken
  // file leaves the form enabled rather than locking everyone out.
  var x = new XMLHttpRequest();
  x.open("GET", cfg.flags);
  x.onload = function () {
    try {
      var f = JSON.parse(x.responseText);
      if (f.submit === false) {
        enabled = false;
        document.getElementById("flag-off").hidden = false;
        var b = document.querySelectorAll("form.intake button");
        for (var i = 0; i < b.length; i++) b[i].disabled = true;
      }
    } catch (e) {}
  };
  x.send();
})();
