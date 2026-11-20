// The app page. Plain JavaScript, no build step.
//
// It shows one app twice over. First what Git says: a fragment the Pages
// build rendered from cool-gitops, which needs no sign-in and is always
// there. Then, on request, what the clusters say: the same fragment served
// live by cool-api, with Argo CD's health in it, for people who sign in.
"use strict";

(async () => {
  const cfg = window.COOL || {};
  const params = new URLSearchParams(location.search);
  const detail = document.getElementById("detail");
  const source = document.getElementById("source");
  const live = document.getElementById("live");
  const actions = document.getElementById("actions");

  // ?app=<team>-<app> from the list, or ?team=&app= from a hand-written link.
  let team = params.get("team") || "";
  let app = params.get("app") || "";
  if (!team && app.includes("-")) {
    [team] = app.split("-", 1);
    app = app.slice(team.length + 1);
  }
  const valid = /^[a-z][a-z0-9]{1,7}$/.test(team) && /^[a-z][a-z0-9-]{0,18}[a-z0-9]$/.test(app);
  if (!valid) {
    detail.textContent = "No such app. Go back to the list and pick one.";
    return;
  }
  document.title = `${team} / ${app} · CoolDeploy`;

  // Fragments are HTML that the platform rendered with every value escaped.
  // They come from this site or from cool-api, never from the address bar.
  const show = (html, from) => {
    detail.innerHTML = html;
    source.textContent = from;
  };

  const link = (text, template, quiet) => {
    const a = document.createElement("a");
    a.className = quiet ? "button quiet" : "button";
    a.href = `${cfg.issues}/new?template=${template}.yml&app=${encodeURIComponent(`${team}/${app}`)}`;
    a.textContent = text;
    return a;
  };
  actions.replaceChildren(
    link("Promote", "cool-deploy-promote"),
    link("Rolling release", "cool-rolling-release", true),
    link("Roll back", "cool-deploy-rollback", true),
    link("Settings", "cool-env-var", true),
  );

  try {
    const resp = await fetch(`fragments/app/${team}-${app}.html`);
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    show(await resp.text(), "From Git: what is meant to be running.");
  } catch (err) {
    detail.textContent = `This app is not on the platform, or the portal has not been rebuilt since it was created. (${err.message})`;
    return;
  }

  // Live status. ?api=... is for the local demo, where cool-api needs no token.
  const api = (params.get("api") || cfg.api || "").replace(/\/$/, "");
  const local = /^http:\/\/127\.0\.0\.1:\d+$/.test(api);
  if (!api || (!local && !window.coolAuth.configured())) return;

  const loadLive = async () => {
    const headers = {};
    if (!local) headers.Authorization = `Bearer ${window.coolAuth.token()}`;
    const resp = await fetch(`${api}/v1/apps/${team}/${app}/status`, { headers });
    if (!resp.ok) throw new Error(`cool-api answered HTTP ${resp.status}`);
    show(await resp.text(), `Live from the clusters, at ${new Date().toLocaleTimeString()}.`);
  };

  try {
    if (local || (await window.coolAuth.finish()) || window.coolAuth.token()) {
      await loadLive();
      live.textContent = "Refresh live health";
    }
  } catch (err) {
    source.textContent = `From Git. Live status is unavailable: ${err.message}`;
  }
  live.hidden = false;
  live.addEventListener("click", async () => {
    if (!local && !window.coolAuth.token()) return window.coolAuth.signIn();
    try {
      await loadLive();
      live.textContent = "Refresh live health";
    } catch (err) {
      source.textContent = `Live status is unavailable: ${err.message}`;
    }
  });
})();
