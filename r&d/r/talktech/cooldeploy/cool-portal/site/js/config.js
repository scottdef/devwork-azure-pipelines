// Settings of the portal. Plain values, nothing secret: the page is public
// to whoever can open it, and so is this file.
"use strict";

window.COOL = {
  // Where request forms live.
  issues: "https://github.com/CoolEngOrg/issueops/issues",

  // cool-api behind API Management, for live health. Leave empty to show
  // only what Git says.
  api: "",

  // Entra ID sign-in for the live view: a single-page-application
  // registration with this page as a redirect URI. No client secret exists
  // or is needed; the flow is authorization code with PKCE.
  tenant: "",
  clientId: "",
  scope: "", // for example api://<tenant-id>/cool-prd/.default
};
