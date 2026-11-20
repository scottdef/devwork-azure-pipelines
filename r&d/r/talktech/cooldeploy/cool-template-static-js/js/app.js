// Plain JavaScript, no build step: load a list and filter it as you type.
"use strict";

const list = document.getElementById("items");
const filter = document.getElementById("filter");
let items = [];

function render() {
  const q = filter.value.trim().toLowerCase();
  const shown = items.filter((it) => it.name.toLowerCase().includes(q));
  list.replaceChildren(
    ...shown.map((it) => {
      const li = document.createElement("li");
      li.textContent = it.name; // textContent, never innerHTML: data is not markup
      return li;
    }),
  );
  if (shown.length === 0) {
    const li = document.createElement("li");
    li.textContent = q ? "Nothing matches." : "Nothing here yet.";
    list.replaceChildren(li);
  }
}

async function load() {
  try {
    const resp = await fetch("data/items.json");
    if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
    items = (await resp.json()).items;
    render();
  } catch (err) {
    const li = document.createElement("li");
    li.textContent = `Could not load the list: ${err.message}`;
    list.replaceChildren(li);
  }
}

filter.addEventListener("input", render);
load();
