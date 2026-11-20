// Sign in to Entra ID from a static page: authorization code with PKCE.
//
// No library and no client secret. The page sends the browser to Entra ID
// with a one-time challenge, Entra ID sends it back with a code, and the
// page trades the code and the challenge's secret half for an access token.
// The token is kept in sessionStorage, which a closed tab forgets.
//
// Used only for the live view. Everything else on the portal needs no token.
"use strict";

window.coolAuth = (() => {
  const cfg = () => window.COOL || {};
  const authority = () => `https://login.microsoftonline.com/${cfg().tenant}/oauth2/v2.0`;
  const redirect = () => location.origin + location.pathname;
  const store = {
    get: (k) => { try { return sessionStorage.getItem(k); } catch { return null; } },
    set: (k, v) => { try { sessionStorage.setItem(k, v); } catch { /* private mode: stay signed out */ } },
    del: (k) => { try { sessionStorage.removeItem(k); } catch { /* nothing to remove */ } },
  };

  const base64url = (bytes) =>
    btoa(String.fromCharCode(...new Uint8Array(bytes))).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  const random = () => base64url(crypto.getRandomValues(new Uint8Array(32)));

  const configured = () => Boolean(cfg().tenant && cfg().clientId && cfg().scope);

  // token returns a valid access token, or null when signed out.
  function token() {
    const t = JSON.parse(store.get("cool.token") || "null");
    return t && t.expires > Date.now() + 60000 ? t.value : null;
  }

  // signIn leaves the page; finish() completes the round trip on return.
  async function signIn() {
    const verifier = random();
    const state = random();
    const challenge = base64url(await crypto.subtle.digest("SHA-256", new TextEncoder().encode(verifier)));
    store.set("cool.pkce", JSON.stringify({ verifier, state, back: location.search }));
    const q = new URLSearchParams({
      client_id: cfg().clientId,
      response_type: "code",
      redirect_uri: redirect(),
      scope: cfg().scope,
      state,
      code_challenge: challenge,
      code_challenge_method: "S256",
    });
    location.assign(`${authority()}/authorize?${q}`);
  }

  // finish trades the code in the URL for a token. It returns true when it
  // did, so the caller can go on to use the token.
  async function finish() {
    const params = new URLSearchParams(location.search);
    const code = params.get("code");
    const pkce = JSON.parse(store.get("cool.pkce") || "null");
    if (!code || !pkce) return false;
    store.del("cool.pkce");
    if (params.get("state") !== pkce.state) throw new Error("sign-in state does not match; try again");
    const resp = await fetch(`${authority()}/token`, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        client_id: cfg().clientId,
        grant_type: "authorization_code",
        code,
        redirect_uri: redirect(),
        code_verifier: pkce.verifier,
        scope: cfg().scope,
      }),
    });
    if (!resp.ok) throw new Error(`sign-in failed: HTTP ${resp.status}`);
    const body = await resp.json();
    store.set("cool.token", JSON.stringify({ value: body.access_token, expires: Date.now() + body.expires_in * 1000 }));
    // Put back the address the person was at, without the code in it.
    history.replaceState(null, "", location.pathname + (pkce.back || ""));
    return true;
  }

  return { configured, token, signIn, finish };
})();
