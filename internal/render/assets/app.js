(function () {
  "use strict";

  const D = JSON.parse(document.getElementById("review-data").textContent);

  // ── Indexes ────────────────────────────────────────────────
  const nodeById = new Map(D.nodes.map((n, i) => [n.id, Object.assign(n, { index: i })]));
  const edgeById = new Map(D.edges.map((e) => [e.id, e]));
  const repoById = new Map(D.repos.map((r) => [r.id, r]));
  const outEdges = new Map();
  const inEdges = new Map();
  for (const e of D.edges) {
    if (!outEdges.has(e.from)) outEdges.set(e.from, []);
    if (!inEdges.has(e.to)) inEdges.set(e.to, []);
    outEdges.get(e.from).push(e);
    inEdges.get(e.to).push(e);
  }
  const testsOf = (id) => (inEdges.get(id) || []).filter((e) => e.kind === "tests" && nodeById.get(e.from)?.kind === "test");
  const testedBy = (id) => (outEdges.get(id) || []).filter((e) => e.kind === "tests");
  const allFindings = D.errors.map((f) => Object.assign(f, { level: "error" })).concat(D.warnings.map((f) => Object.assign(f, { level: "warning" })));

  const STATUS_LABEL = { verified: "Verified", contradicted: "Contradicted", unresolved: "Unresolved", agent: "Agent statement" };
  const CHANGE_LABEL = { added: "Added", modified: "Modified", deleted: "Deleted", unchanged: "Unchanged", renamed: "Renamed" };

  // ── Icons ──────────────────────────────────────────────────
  const P = {
    check: '<path d="M20 6 9 17l-5-5"/>',
    x: '<path d="M18 6 6 18M6 6l12 12"/>',
    xcircle: '<circle cx="12" cy="12" r="9.5"/><path d="m15 9-6 6M9 9l6 6"/>',
    checkcircle: '<circle cx="12" cy="12" r="9.5"/><path d="m8.5 12.2 2.4 2.4 4.8-5"/>',
    help: '<circle cx="12" cy="12" r="9.5"/><path d="M9.1 9a3 3 0 0 1 5.8 1c0 2-3 2.6-3 4"/><path d="M12 17.2h.01"/>',
    chat: '<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>',
    alert: '<path d="m21.7 18-8-14a2 2 0 0 0-3.5 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.7-3"/><path d="M12 9v4M12 17h.01"/>',
    info: '<circle cx="12" cy="12" r="9.5"/><path d="M12 16v-4M12 8h.01"/>',
    commit: '<circle cx="12" cy="12" r="3.2"/><path d="M3 12h5.8M15.2 12H21"/>',
    repo: '<path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1 0-5H20"/>',
    file: '<path d="M14.5 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7.5z"/><path d="M14 2v6h6"/>',
    test: '<path d="M9 3h6M10 3v6.5L4.6 18.4A1.8 1.8 0 0 0 6.2 21h11.6a1.8 1.8 0 0 0 1.6-2.6L14 9.5V3"/><path d="M7 15h10"/>',
    component: '<path d="M21 16V8a2 2 0 0 0-1-1.7l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.7l7 4a2 2 0 0 0 2 0l7-4a2 2 0 0 0 1-1.7z"/><path d="M3.3 7 12 12l8.7-5M12 22V12"/>',
    code: '<path d="m16 18 6-6-6-6M8 6l-6 6 6 6"/>',
    endpoint: '<circle cx="12" cy="12" r="9.5"/><path d="M2.5 12h19M12 2.5a14.5 14.5 0 0 1 0 19 14.5 14.5 0 0 1 0-19"/>',
    entry: '<path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4M10 17l5-5-5-5M15 12H3"/>',
    external: '<path d="M17.5 19H9a7 7 0 1 1 6.7-9h1.8a4.5 4.5 0 1 1 0 9"/>',
    other: '<circle cx="12" cy="12" r="9.5"/><circle cx="12" cy="12" r="3"/>',
    config: '<path d="M4 21v-7M4 10V3M12 21v-9M12 8V3M20 21v-5M20 12V3M1 14h6M9 8h6M17 16h6"/>',
    data: '<ellipse cx="12" cy="5" rx="9" ry="3"/><path d="M3 5v14c0 1.7 4 3 9 3s9-1.3 9-3V5M3 12c0 1.7 4 3 9 3s9-1.3 9-3"/>',
    doc: '<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6M16 13H8M16 17H8M10 9H8"/>',
    graph: '<circle cx="5.5" cy="6" r="2.5"/><circle cx="18.5" cy="6" r="2.5"/><circle cx="12" cy="18" r="2.5"/><path d="m7 8.2 3.8 7.6M17 8.2l-3.8 7.6M8 6h8"/>',
    flows: '<rect x="2.5" y="4" width="6" height="6" rx="1.5"/><rect x="15.5" y="14" width="6" height="6" rx="1.5"/><path d="M8.5 7h4a2 2 0 0 1 2 2v6a2 2 0 0 0 2 2"/>',
    diff: '<path d="M12 3v10M7 8h10M7 20h10"/>',
    claims: '<path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/><path d="m9 12 2 2 4-4"/>',
    sun: '<circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/>',
    moon: '<path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8"/>',
    monitor: '<rect x="2" y="3" width="20" height="14" rx="2"/><path d="M8 21h8M12 17v4"/>',
    fit: '<path d="M8 3H5a2 2 0 0 0-2 2v3M21 8V5a2 2 0 0 0-2-2h-3M16 21h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3"/>',
    plus: '<path d="M12 5v14M5 12h14"/>',
    minus: '<path d="M5 12h14"/>',
    chevron: '<path d="m9 18 6-6-6-6"/>',
    close: '<path d="M18 6 6 18M6 6l12 12"/>',
    copy: '<rect x="9" y="9" width="13" height="13" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>',
    arrow: '<path d="M5 12h14M13 6l6 6-6 6"/>',
    layers: '<path d="m12 2 10 5-10 5L2 7z"/><path d="m2 17 10 5 10-5M2 12l10 5 10-5"/>',
    logo: '<path d="M6 7h8M6 12h12M6 17h6"/><circle cx="18" cy="7" r="1.6" fill="currentColor"/>',
    note: '<path d="M12 20h9M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4z"/>',
    branch: '<circle cx="6" cy="6" r="2.5"/><circle cx="6" cy="18" r="2.5"/><circle cx="18" cy="8" r="2.5"/><path d="M6 8.5v7M18 10.5c0 4-6 3-11 5.5"/>',
    eye: '<path d="M2 12s3.6-7 10-7 10 7 10 7-3.6 7-10 7S2 12 2 12"/><circle cx="12" cy="12" r="3"/>',
    target: '<circle cx="12" cy="12" r="9.5"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1"/>',
    tree: '<path d="M3 3h6v6H3zM15 15h6v6h-6zM6 9v6a3 3 0 0 0 3 3h6"/>',
  };
  const icon = (name, cls = "") => `<svg class="i ${cls}" viewBox="0 0 24 24" aria-hidden="true">${P[name] || P.other}</svg>`;
  const KIND_ICON = { entry: "entry", code: "code", component: "component", endpoint: "endpoint", test: "test", external: "external",
    config: "config", migration: "data", data: "data", schema: "data", store: "data", doc: "doc", docs: "doc", function: "code",
    handler: "code", module: "code", type: "code", hook: "code", route: "entry", page: "entry", ui: "component", screen: "component" };
  const kindIcon = (kind) => KIND_ICON[kind] || "other";
  const STATUS_ICON = { verified: "checkcircle", contradicted: "xcircle", unresolved: "help", agent: "chat" };

  // ── DOM helpers ────────────────────────────────────────────
  function h(tag, attrs, ...kids) {
    const el = document.createElement(tag);
    if (attrs) {
      for (const [k, v] of Object.entries(attrs)) {
        if (v == null || v === false) continue;
        if (k === "class") el.className = v;
        else if (k === "html") el.innerHTML = v;
        else if (k === "style" && typeof v === "object") Object.assign(el.style, v);
        else if (k.startsWith("on")) el.addEventListener(k.slice(2), v);
        else el.setAttribute(k, v === true ? "" : v);
      }
    }
    for (const kid of kids.flat(Infinity)) {
      if (kid == null || kid === false) continue;
      el.appendChild(kid instanceof Node ? kid : document.createTextNode(String(kid)));
    }
    return el;
  }
  const raw = (html) => { const t = document.createElement("template"); t.innerHTML = html; return t.content; };
  const esc = (s) => String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[c]);
  const plural = (n, one, many) => `${n} ${n === 1 ? one : many || one + "s"}`;
  const short = (sha) => (sha === "WORKTREE" ? "WORKTREE" : (sha || "").slice(0, 7));

  function linesText(ref) {
    if (!ref) return "";
    if (ref.lines) return ref.lines[0] === ref.lines[1] ? `:${ref.lines[0]}` : `:${ref.lines[0]}–${ref.lines[1]}`;
    if (ref.line) return `:${ref.line}`;
    return "";
  }
  function refText(ref) {
    if (!ref) return "";
    if (ref.type === "commit") return `${ref.repo} @ ${short(ref.sha)}`;
    return `${ref.file}${linesText(ref)}`;
  }
  function basename(p) { const i = p.lastIndexOf("/"); return i < 0 ? p : p.slice(i + 1); }
  function dirname(p) { const i = p.lastIndexOf("/"); return i < 0 ? "" : p.slice(0, i + 1); }

  function statusBadge(status) {
    return h("span", { class: `badge ${status}`, html: icon(STATUS_ICON[status], "sm") + esc(STATUS_LABEL[status] || status) });
  }
  function changeBadge(change) {
    return h("span", { class: `badge ${change}` }, CHANGE_LABEL[change] || change);
  }
  function agentTag(text = "Agent says") {
    return h("div", { class: "agent-tag", html: icon("chat") + esc(text) });
  }
  function observedClass(v) {
    const s = String(v).toLowerCase();
    if (/(^|\b)(pass|passed|ok|green|success)/.test(s)) return "passed";
    if (/(fail|failed|error|red)/.test(s)) return "failed";
    return "";
  }

  // ── State & routing ────────────────────────────────────────
  function loadPref(k, def) { try { return localStorage.getItem(k) || def; } catch { return def; } }
  function savePref(k, v) { try { localStorage.setItem(k, v); } catch {} }
  const state = {
    view: "graph",
    sel: null, // {type: node|edge|hunk|findings|file, id}
    showUnchanged: true,
    direction: "RIGHT",
    diffFilter: "all",
    commitFilter: "",
    claimFilter: "all",
    openFiles: new Set(),
    theme: loadPref("cv-theme", "auto"),
    legend: loadPref("cv-legend", D.nodes.length > 20 ? "0" : "1") === "1",
  };
  const VIEWS = [
    { id: "graph", label: "Graph", icon: "graph", count: () => D.nodes.length },
    { id: "flows", label: "Flows", icon: "flows", count: () => D.flows.length },
    { id: "diff", label: "Diff map", icon: "diff", count: () => D.files.length },
    { id: "claims", label: "Claims", icon: "claims", count: () => D.claims.length },
  ];

  function readHash() {
    const p = new URLSearchParams(location.hash.slice(1));
    const v = p.get("view");
    if (VIEWS.some((x) => x.id === v)) state.view = v;
    const sel = p.get("sel");
    if (sel) {
      const i = sel.indexOf(":");
      state.sel = { type: sel.slice(0, i), id: sel.slice(i + 1) };
    }
  }
  function writeHash() {
    const p = new URLSearchParams();
    p.set("view", state.view);
    if (state.sel && state.sel.type !== "findings") p.set("sel", `${state.sel.type}:${state.sel.id}`);
    history.replaceState(null, "", "#" + p.toString());
  }

  function applyTheme() {
    const root = document.documentElement;
    if (state.theme === "auto") root.removeAttribute("data-theme");
    else root.setAttribute("data-theme", state.theme);
    savePref("cv-theme", state.theme);
  }

  // ── Shell ──────────────────────────────────────────────────
  const app = document.getElementById("app");
  let viewEl, inspectorEl, tabbarEl, toolbarEl;

  function render() {
    applyTheme();
    app.innerHTML = "";
    app.appendChild(topbar());
    const banner = errorBanner();
    if (banner) app.appendChild(banner);
    else app.style.gridTemplateRows = "auto 1fr";
    const body = h("div", { class: "body" });
    body.appendChild(sidebar());
    const main = h("div", { class: "main" });
    tabbarEl = h("div", { class: "tabbar" });
    viewEl = h("div", { class: "view" });
    main.append(tabbarEl, viewEl);
    body.appendChild(main);
    inspectorEl = h("aside", { class: "inspector" });
    body.appendChild(inspectorEl);
    app.appendChild(body);
    renderTabs();
    renderView();
    renderInspector();
  }

  function topbar() {
    const c = D.counts;
    const errCount = D.errors.length;
    const warnCount = D.warnings.length;
    const pill = (cls, ic, n, label, onclick, title) =>
      h("button", { class: `pill ${cls}${n > 0 && (cls === "bad" || cls === "warn") ? " has" : ""}`, onclick, title, html: `${icon(ic, "sm")}<b>${n}</b><span>${esc(label)}</span>` });
    const themeIcon = { auto: "monitor", light: "sun", dark: "moon" }[state.theme];
    const repos = D.repos.map((r) => r.id).join(" · ");
    return h("header", { class: "topbar" },
      h("div", { class: "brand" }, h("span", { class: "brand-mark", html: icon("logo") }), "change-view"),
      h("div", { class: "crumbs" },
        h("span", { class: "sep" }, "/"),
        h("span", { class: "path", title: D.reviewPath }, repos),
        h("span", { class: "sep" }, "/"),
        h("span", { class: "path", title: D.reviewPath }, D.reviewName)),
      h("div", { class: "spacer" }),
      h("div", { class: "pills" },
        pill("ok", "checkcircle", c.verified, "verified", () => openFindings("all"), "References the tool resolved and checked"),
        pill("bad", "xcircle", c.contradicted, "contradicted", () => openFindings("contradicted"), "References whose check failed"),
        pill("warn", "help", c.unresolved, "unresolved", () => openFindings("unresolved"), "References that could not be found"),
        pill("agent", "chat", c.agentStatements, "agent statements", null, "Content the tool cannot check: shown as the agent's word"),
        h("span", { class: "tb-sep" }),
        pill(errCount ? "bad" : "ok", errCount ? "xcircle" : "checkcircle", errCount, plural(errCount, "error").replace(/^\d+ /, ""), () => openFindings("errors"), "Errors in the review file"),
        pill(warnCount ? "warn" : "ok", "alert", warnCount, plural(warnCount, "warning").replace(/^\d+ /, ""), () => openFindings("warnings"), "Diff-level findings and warnings")),
      h("button", { class: "icon-btn", title: `Theme: ${state.theme}`, html: icon(themeIcon), onclick: () => {
        state.theme = { auto: "light", light: "dark", dark: "auto" }[state.theme];
        render();
      } }));
  }

  function errorBanner() {
    if (D.exitCode === 0 || D.errors.length === 0) return null;
    return h("div", { class: "banner", html: icon("alert") },
      h("div", null, h("b", null, `This review file has ${plural(D.errors.length, "error")}.`),
        " The views show the account as written; elements marked contradicted or unresolved are not facts. Ask the agent to fix the file and validate again."),
      h("button", { class: "link", onclick: () => openFindings("errors") }, "Show errors →"));
  }

  // ── Sidebar ────────────────────────────────────────────────
  function sidebar() {
    const sb = h("aside", { class: "sidebar" });

    sb.appendChild(h("section", { class: "sb-section" },
      h("h3", { class: "sb-title", html: icon("chat", "sm") + "Summary" }),
      summaryBlock()));

    const repoSec = h("section", { class: "sb-section" }, h("h3", { class: "sb-title", html: icon("repo", "sm") + `Repositories<span class="count">${D.repos.length}</span>` }));
    for (const r of D.repos) repoSec.appendChild(repoCard(r));
    sb.appendChild(repoSec);

    sb.appendChild(findingsSection());

    if (D.notes.length) {
      sb.appendChild(h("section", { class: "sb-section" },
        h("h3", { class: "sb-title", html: icon("note", "sm") + `Agent notes<span class="count">${D.notes.length}</span>` }),
        h("ul", { class: "notes" }, D.notes.map((n) => h("li", null, n)))));
    }
    sb.appendChild(h("section", { class: "sb-section muted", style: { fontSize: "11.5px" } },
      `Generated ${new Date(D.generated).toLocaleString()} · change-view ${D.version}`));
    return sb;
  }

  function summaryBlock() {
    const text = h("p", { class: "summary-text clamp" }, D.summary);
    const box = h("div", { class: "agent-says" }, agentTag(), text);
    if (D.summary.length > 280) {
      const more = h("button", { class: "link more", onclick: () => {
        const open = text.classList.toggle("clamp");
        more.textContent = open ? "Show more" : "Show less";
      } }, "Show more");
      box.appendChild(more);
    } else text.classList.remove("clamp");
    return box;
  }

  function repoCard(r) {
    const card = h("div", { class: "repo-card" });
    card.appendChild(h("div", { class: "repo-head" },
      h("span", { html: icon(r.role === "context" ? "eye" : "repo", "sm"), style: { color: "var(--text-3)" } }),
      h("span", { class: "repo-name" }, r.id),
      h("span", { class: r.role === "context" ? "badge context" : "badge role" }, r.role === "context" ? "context" : "changed"),
      r.role !== "context" ? h("span", { class: "repo-stats" },
        h("span", { class: "muted" }, plural(r.files, "file")),
        h("span", { class: "plus" }, `+${r.added}`), h("span", { class: "minus" }, `−${r.removed}`)) : null));
    card.appendChild(h("div", { class: "repo-path", title: r.abs }, r.path));
    const list = h("div", { class: "commits" });
    for (const c of r.commits) {
      const multi = r.role !== "context" && r.commits.length > 1;
      list.appendChild(h("div", { class: "commit" + (c.worktree ? " wt" : "") + (multi ? " clickable" : ""), onclick: multi ? () => {
        state.commitFilter = `${r.id}@${c.sha}`;
        state.diffFilter = "all";
        D.files.filter((f) => f.repo === r.id).forEach((f) => state.openFiles.add(f.id));
        if (state.view !== "diff") setView("diff"); else renderView();
      } : null, title: c.worktree ? "Uncommitted work on disk" : `${c.sha}\n${c.subject}\n${c.author || ""} ${c.date ? "· " + new Date(c.date).toLocaleString() : ""}` },
        h("span", { class: "dot" }), h("span", { class: "sha" }, c.worktree ? "worktree" : c.short), h("span", { class: "subj" }, c.subject || "")));
    }
    if (r.base) {
      list.appendChild(h("div", { class: "commit base", title: `Initial state: ${r.base.sha}\n${r.base.subject || ""}` },
        h("span", { class: "dot" }), h("span", { class: "sha" }, r.base.short), h("span", { class: "subj" }, `base · ${r.base.subject || ""}`)));
    }
    card.appendChild(list);
    if (r.uncommitted && r.uncommitted.length) {
      card.appendChild(h("div", { class: "callout warn", style: { margin: "0 8px 8px", padding: "7px 9px", fontSize: "11.5px" }, html: icon("alert", "sm") },
        h("div", null, `${plural(r.uncommitted.length, "uncommitted path")} not part of the review: `, h("span", { class: "mono" }, r.uncommitted.slice(0, 4).join(", ") + (r.uncommitted.length > 4 ? " …" : "")))));
    }
    return card;
  }

  const GROUPS = [
    { id: "contradicted", label: "Contradicted references", level: "err", icon: "xcircle", match: (f) => f.level === "error" && f.status === "contradicted" },
    { id: "unresolved", label: "Unresolved references", level: "err", icon: "help", match: (f) => f.level === "error" && f.status === "unresolved" },
    { id: "chain", label: "Other errors", level: "err", icon: "alert", match: (f) => f.level === "error" && !f.status },
    { id: "unmentioned_hunk", label: "Unmentioned changes", level: "warn", icon: "diff", match: (f) => f.code === "unmentioned_hunk" },
    { id: "unreferenced_test_hunk", label: "Unreferenced test code", level: "warn", icon: "test", match: (f) => f.code === "unreferenced_test_hunk" },
    { id: "untested_change", label: "Untested changed nodes", level: "warn", icon: "target", match: (f) => f.code === "untested_change" },
    { id: "unlisted_repo", label: "Unlisted repos", level: "warn", icon: "repo", match: (f) => f.code === "unlisted_repo" },
    { id: "uncommitted", label: "Work outside the review", level: "warn", icon: "branch", match: (f) => f.code === "uncommitted_changes" || f.code === "head_ahead" },
    { id: "other", label: "Other warnings", level: "warn", icon: "alert", match: (f) => f.level === "warning" && !["unmentioned_hunk", "unreferenced_test_hunk", "untested_change", "unlisted_repo", "uncommitted_changes", "head_ahead"].includes(f.code) },
  ];

  function changedTests() {
    return D.nodes.filter((n) => n.kind === "test" && n.change !== "unchanged" && n.result.status === "verified");
  }

  function findingsSection() {
    const sec = h("section", { class: "sb-section" }, h("h3", { class: "sb-title", html: icon("alert", "sm") + `Findings<span class="count">${allFindings.length}</span>` }));
    let any = false;
    for (const g of GROUPS) {
      const n = allFindings.filter(g.match).length;
      if (!n) continue;
      any = true;
      sec.appendChild(h("button", { class: `finding-group ${g.level}`, onclick: () => openFindings(g.id), html: icon(g.icon) + `<span>${esc(g.label)}</span><span class="n">${n}</span>` }));
    }
    if (!any) sec.appendChild(h("div", { class: "all-clear", html: icon("checkcircle") + "The account matches the diff" }));
    const ct = changedTests();
    if (ct.length) {
      sec.appendChild(h("button", { class: "finding-group info", onclick: () => openFindings("tests"), html: icon("test") + `<span>Changed tests</span><span class="n">${ct.length}</span>` }));
    }
    return sec;
  }

  // ── Tabs & toolbar ─────────────────────────────────────────
  function renderTabs() {
    tabbarEl.innerHTML = "";
    VIEWS.forEach((v, i) => {
      tabbarEl.appendChild(h("button", { class: "tab" + (state.view === v.id ? " on" : ""), title: `${v.label} (${i + 1})`, onclick: () => setView(v.id), html: `${icon(v.icon)}${esc(v.label)}<span class="n">${v.count()}</span>` }));
    });
    toolbarEl = h("div", { class: "toolbar" });
    tabbarEl.appendChild(toolbarEl);
  }

  function setView(id) {
    if (state.view === id) return;
    state.view = id;
    renderTabs();
    renderView();
    writeHash();
  }

  function renderView() {
    destroyGraph();
    viewEl.innerHTML = "";
    viewEl.scrollTop = 0;
    toolbarEl.innerHTML = "";
    ({ graph: graphView, flows: flowsView, diff: diffView, claims: claimsView })[state.view]();
  }

  function select(sel) {
    state.sel = sel;
    renderInspector();
    highlightSelection();
    writeHash();
  }

  // ── Graph view ─────────────────────────────────────────────
  let cy = null;
  let overlay = null;
  let tooltipEl = null;

  function css(name) { return getComputedStyle(document.documentElement).getPropertyValue(name).trim(); }

  const measureCtx = document.createElement("canvas").getContext("2d");
  function textWidth(t, font) { measureCtx.font = font; return measureCtx.measureText(t).width; }

  function nodeSub(n) {
    if (n.ref && n.ref.file) return `${n.ref.repo === n.ref.repo ? "" : ""}${basename(n.ref.file)}${linesText(n.ref)}`;
    if (n.ref && n.ref.type === "commit") return `commit ${short(n.ref.sha)}`;
    return n.kind;
  }

  function nodeSize(n) {
    const lw = textWidth(n.label, "600 12.5px " + css("--font"));
    const sw = textWidth(nodeSub(n), "10.5px " + css("--mono"));
    const w = Math.max(150, Math.min(290, Math.max(lw, sw) + 74));
    const testMeta = n.kind === "test" && n.meta && n.meta.some((m) => m.key === "observed");
    return { w: Math.round(w), h: testMeta ? 66 : 52 };
  }

  function graphStyle() {
    const C = (n) => css(n);
    const fill = { added: C("--added-fill"), modified: C("--modified-fill"), deleted: C("--deleted-fill"), unchanged: C("--unchanged-fill") };
    return [
      { selector: "node", style: { shape: "round-rectangle", "corner-radius": 8, width: "data(w)", height: "data(h)", "background-color": (e) => fill[e.data("change")] || fill.unchanged,
        "border-width": 1.5, "border-color": C("--text-3"), "border-style": "solid", label: "", "transition-property": "opacity", "transition-duration": "120ms" } },
      { selector: "node.test", style: { "corner-radius": 26 } },
      { selector: "node.verified", style: { "border-width": 1.5, "border-color": C("--text-2") } },
      { selector: "node.contradicted", style: { "border-width": 2.5, "border-color": C("--bad") } },
      { selector: "node.unresolved", style: { "border-width": 2, "border-color": C("--warn"), "border-style": "dashed" } },
      { selector: "node.agent", style: { "border-width": 2, "border-color": C("--agent"), "border-style": "dotted" } },
      { selector: "node.repo", style: { shape: "round-rectangle", "corner-radius": 14, "background-color": C("--panel"), "background-opacity": 0.65, "border-width": 1, "border-color": C("--repo-border"),
        "border-style": "solid", padding: 30, "compound-sizing-wrt-labels": "exclude" } },
      { selector: "node.repo.context", style: { "border-style": "dashed", "background-opacity": 0.35 } },
      { selector: "node:selected", style: { "underlay-color": C("--accent"), "underlay-padding": 5, "underlay-opacity": 0.22, "underlay-shape": "round-rectangle", "underlay-corner-radius": 11 } },
      { selector: "node.repo:selected", style: { "underlay-opacity": 0 } },
      { selector: "edge", style: { width: 1.6, "line-color": C("--text-3"), "target-arrow-color": C("--text-3"), "target-arrow-shape": "triangle", "arrow-scale": 0.9,
        "curve-style": "bezier", "control-point-step-size": 40, label: "data(kind)", "font-size": 10, "font-family": C("--font"), color: C("--text-2"),
        "text-background-color": C("--bg"), "text-background-opacity": 1, "text-background-padding": 3, "text-background-shape": "round-rectangle",
        "text-border-width": 1, "text-border-color": C("--border"), "text-border-opacity": 1, "line-style": "dotted", "min-zoomed-font-size": 7,
        "line-opacity": D.edges.length > 40 ? 0.55 : 1, "transition-property": "opacity", "transition-duration": "120ms" } },
      { selector: "edge.verified", style: { "line-style": "solid", "line-color": C("--text-2"), "target-arrow-color": C("--text-2") } },
      { selector: "edge.contradicted", style: { "line-style": "solid", width: 2.2, "line-color": C("--bad"), "target-arrow-color": C("--bad"), color: C("--bad") } },
      { selector: "edge.unresolved", style: { "line-style": "dashed", "line-color": C("--warn"), "target-arrow-color": C("--warn"), color: C("--warn") } },
      { selector: "edge.tests", style: { "target-arrow-shape": "triangle-backcurve" } },
      { selector: "edge.focus", style: { "line-opacity": 1, "z-index": 10, "min-zoomed-font-size": 0 } },
      { selector: "edge:selected", style: { width: 3, "line-color": C("--accent"), "target-arrow-color": C("--accent"), color: C("--accent-text") } },
      { selector: ".dim, .miss", style: { opacity: 0.22 } },
      { selector: "node.hit", style: { "underlay-color": C("--warn"), "underlay-padding": 5, "underlay-opacity": 0.28, "underlay-shape": "round-rectangle" } },
    ];
  }

  function graphElements() {
    const visible = D.nodes.filter((n) => state.showUnchanged || n.change !== "unchanged");
    const ids = new Set(visible.map((n) => n.id));
    const els = [];
    const reposUsed = new Set();
    // One repo: the header already names it, so skip the box.
    const boxed = new Set(D.nodes.filter((n) => n.ref && repoById.get(n.ref.repo)).map((n) => n.ref.repo)).size > 1;
    for (const n of visible) {
      const repo = boxed && n.ref && repoById.get(n.ref.repo) ? n.ref.repo : null;
      if (repo) reposUsed.add(repo);
      const sz = nodeSize(n);
      els.push({ group: "nodes", data: { id: n.id, change: n.change, w: sz.w, h: sz.h, parent: repo ? "repo:" + repo : undefined },
        classes: [n.result.status, n.kind === "test" ? "test" : "", "change-" + n.change].join(" ") });
    }
    for (const r of D.repos) {
      if (!reposUsed.has(r.id)) continue;
      els.push({ group: "nodes", data: { id: "repo:" + r.id, repo: r.id }, classes: "repo" + (r.role === "context" ? " context" : "") });
    }
    for (const e of D.edges) {
      if (!ids.has(e.from) || !ids.has(e.to)) continue;
      els.push({ group: "edges", data: { id: e.id, source: e.from, target: e.to, kind: e.kind }, classes: [e.result.status, e.kind === "tests" ? "tests" : ""].join(" ") });
    }
    return els;
  }

  function graphView() {
    const seg = h("div", { class: "seg" },
      h("button", { class: state.direction === "RIGHT" ? "on" : "", onclick: () => { state.direction = "RIGHT"; renderView(); }, title: "Left to right" }, "→"),
      h("button", { class: state.direction === "DOWN" ? "on" : "", onclick: () => { state.direction = "DOWN"; renderView(); }, title: "Top to bottom" }, "↓"));
    const unchangedCount = D.nodes.filter((n) => n.change === "unchanged").length;
    const search = h("input", { class: "search", type: "search", placeholder: "Find node…  /", value: state.query || "",
      oninput: (ev) => { state.query = ev.target.value; applySearch(); },
      onkeydown: (ev) => {
        if (ev.key === "Enter" && cy) {
          const hit = cy.nodes(".hit").first();
          if (hit.length) { select({ type: "node", id: hit.id() }); cy.animate({ center: { eles: hit }, zoom: Math.max(cy.zoom(), 0.9) }, { duration: 220 }); }
        }
        if (ev.key === "Escape") { ev.target.value = ""; state.query = ""; applySearch(); ev.target.blur(); }
      } });
    toolbarEl.append(
      search,
      h("span", { class: "tb-sep" }),
      h("label", { class: "toggle", title: "Show nodes the change does not modify" },
        h("input", { type: "checkbox", checked: state.showUnchanged, onchange: (ev) => { state.showUnchanged = ev.target.checked; renderView(); } }),
        h("span", { class: "sw" }), `Unchanged (${unchangedCount})`),
      h("span", { class: "tb-sep" }), seg);

    if (typeof cytoscape === "undefined") {
      viewEl.appendChild(h("div", { class: "empty" }, "Graph renderer failed to load."));
      return;
    }
    const wrap = h("div", { class: "graph-wrap" });
    const cyEl = h("div", { class: "cy" });
    overlay = h("div", { class: "cy-overlay" });
    const layer = h("div", { class: "cy-layer" });
    overlay.appendChild(layer);
    wrap.append(cyEl, overlay);
    wrap.appendChild(h("div", { class: "graph-controls" },
      h("button", { class: "icon-btn", title: "Zoom in", html: icon("plus"), onclick: () => zoomBy(1.25) }),
      h("button", { class: "icon-btn", title: "Zoom out", html: icon("minus"), onclick: () => zoomBy(0.8) }),
      h("button", { class: "icon-btn", title: "Fit to screen", html: icon("fit"), onclick: () => cy && cy.animate({ fit: { padding: 48 } }, { duration: 220 }) })));
    wrap.appendChild(legend());
    viewEl.appendChild(wrap);

    if (typeof cytoscapeElk !== "undefined" && !cytoscape.__elk) { cytoscape.use(cytoscapeElk); cytoscape.__elk = true; }

    cy = cytoscape({
      container: cyEl, elements: graphElements(), style: graphStyle(), wheelSensitivity: 0.25, minZoom: 0.15, maxZoom: 2.5,
      boxSelectionEnabled: false, autoungrabify: false,
    });

    // HTML overlay for rich node content.
    const nodeEls = new Map();
    cy.nodes().forEach((ele) => {
      if (ele.hasClass("repo")) {
        const r = repoById.get(ele.data("repo"));
        const el = h("div", { class: "gl", html: icon(r.role === "context" ? "eye" : "repo", "sm") + `<span>${esc(r.id)}</span><span class="role">${r.role === "context" ? "context · read only" : esc(r.worktree ? "worktree" : short(r.commits[0]?.sha))}</span>` });
        layer.appendChild(el);
        nodeEls.set(ele.id(), el);
        return;
      }
      const n = nodeById.get(ele.id());
      const observed = n.meta && n.meta.find((m) => m.key === "observed");
      const el = h("div", { class: `gn ${n.change} ${n.result.status}`, style: { width: ele.data("w") + "px", height: ele.data("h") + "px" } },
        h("div", { class: "ic", html: icon(kindIcon(n.kind)) }),
        h("div", { class: "tx" },
          h("div", { class: "lb" }, n.label),
          h("div", { class: "sb" }, `${n.kind} · ${nodeSub(n)}`),
          observed ? h("span", { class: "says " + observedClass(observed.value) }, `agent says: ${observed.value}`) : null),
        n.result.status === "contradicted" || n.result.status === "unresolved" ? h("div", { class: `st ${n.result.status}`, html: icon(STATUS_ICON[n.result.status]) }) : null);
      layer.appendChild(el);
      nodeEls.set(ele.id(), el);
    });

    let raf = 0;
    const sync = () => {
      raf = 0;
      const pan = cy.pan(), zoom = cy.zoom();
      layer.style.transform = `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`;
      cy.nodes().forEach((ele) => {
        const el = nodeEls.get(ele.id());
        if (!el) return;
        if (ele.hasClass("repo")) {
          const bb = ele.boundingBox({ includeLabels: false, includeOverlays: false, includeUnderlays: false });
          el.style.left = bb.x1 + 14 + "px";
          el.style.top = bb.y1 + 7 + "px";
        } else {
          const p = ele.position();
          el.style.left = p.x - ele.data("w") / 2 + "px";
          el.style.top = p.y - ele.data("h") / 2 + "px";
        }
        el.classList.toggle("dim", ele.hasClass("dim") || ele.hasClass("miss"));
      });
    };
    const schedule = () => { if (!raf) raf = requestAnimationFrame(sync); };
    cy.on("render viewport position", schedule);
    overlay.__sync = schedule;
    const ro = new ResizeObserver(() => {
      if (!cy) return;
      cy.resize();
      schedule();
      // Keep the selection in view when the inspector opens.
      const sel = cy.$(":selected");
      if (sel.length) {
        const bb = sel.renderedBoundingBox(), w = cy.width(), hh = cy.height();
        if (bb.x2 > w - 20 || bb.x1 < 20 || bb.y1 < 20 || bb.y2 > hh - 20) cy.animate({ center: { eles: sel } }, { duration: 200 });
      }
    });
    ro.observe(cyEl);
    cy.on("destroy", () => ro.disconnect());

    cy.on("tap", "node", (ev) => {
      const id = ev.target.id();
      if (id.startsWith("repo:")) return;
      select({ type: "node", id });
    });
    cy.on("tap", "edge", (ev) => select({ type: "edge", id: ev.target.id() }));
    cy.on("tap", (ev) => { if (ev.target === cy) select(null); });

    cy.on("mouseover", "node, edge", (ev) => {
      const t = ev.target;
      if (t.isNode() && t.hasClass("repo")) return;
      showTooltip(ev.originalEvent, t.isNode() ? nodeTooltip(nodeById.get(t.id())) : edgeTooltip(edgeById.get(t.id())));
      cyEl.style.cursor = "pointer";
    });
    cy.on("mousemove", "node, edge", (ev) => moveTooltip(ev.originalEvent));
    cy.on("mouseout", "node, edge", () => { hideTooltip(); cyEl.style.cursor = ""; });

    // Spacing must be set on every compound too: ELK reads it per graph level.
    const spacing = {
      "elk.direction": state.direction,
      "elk.layered.spacing.nodeNodeBetweenLayers": 96,
      "elk.spacing.nodeNode": 30,
      "elk.layered.spacing.edgeNodeBetweenLayers": 28,
      "elk.spacing.edgeNode": 24,
      "elk.spacing.componentComponent": 56,
      "elk.padding": "[top=50,left=30,bottom=30,right=30]",
      "elk.layered.nodePlacement.strategy": "NETWORK_SIMPLEX",
      "elk.layered.crossingMinimization.strategy": "LAYER_SWEEP",
    };
    const layout = cy.layout({
      name: typeof cytoscapeElk !== "undefined" ? "elk" : "breadthfirst",
      fit: true, padding: 48, animate: false, nodeDimensionsIncludeLabels: false,
      nodeLayoutOptions: (node) => (node.isParent() ? spacing : {}),
      elk: Object.assign({ algorithm: "layered", "elk.hierarchyHandling": "INCLUDE_CHILDREN",
        "elk.layered.compaction.postCompaction.strategy": "EDGE_LENGTH" }, spacing),
    });
    layout.on("layoutstop", () => {
      if (cy.zoom() > 1.15) { cy.zoom(1.15); cy.center(); }
      // Large graphs: start readable at the first flow's entry, not as a speck.
      if (cy.zoom() < 0.5) {
        const first = D.flows.length && cy.getElementById(D.flows[0].path[0]);
        const anchor = first && first.length ? first : cy.nodes().not(".repo").first();
        cy.zoom(0.62);
        cy.center(anchor);
      }
      highlightSelection();
      applySearch();
      schedule();
    });
    layout.run();
  }

  function applySearch() {
    if (!cy) return;
    const q = (state.query || "").trim().toLowerCase();
    cy.batch(() => {
      cy.nodes().removeClass("hit miss");
      if (!q) return;
      cy.nodes().not(".repo").forEach((ele) => {
        const n = nodeById.get(ele.id());
        const text = `${n.label} ${n.id} ${n.kind} ${n.ref && n.ref.file ? n.ref.file : ""}`.toLowerCase();
        ele.addClass(text.includes(q) ? "hit" : "miss");
      });
    });
    overlay && overlay.__sync && overlay.__sync();
  }

  function zoomBy(f) {
    if (!cy) return;
    const z = cy.zoom() * f;
    cy.animate({ zoom: { level: z, renderedPosition: { x: cy.width() / 2, y: cy.height() / 2 } } }, { duration: 160 });
  }

  function destroyGraph() {
    if (cy) { cy.destroy(); cy = null; }
    hideTooltip();
  }

  function highlightSelection() {
    if (cy) {
      cy.batch(() => {
        cy.elements().unselect().removeClass("dim focus");
        const sel = state.sel;
        if (sel && (sel.type === "node" || sel.type === "edge")) {
          const ele = cy.getElementById(sel.id);
          if (ele && ele.length) {
            ele.select();
            const keep = sel.type === "node" ? ele.closedNeighborhood() : ele.union(ele.connectedNodes());
            keep.edges().addClass("focus");
            const parents = keep.nodes().parents();
            cy.elements().not(keep).not(parents).addClass("dim");
          }
        }
      });
      overlay && overlay.__sync && overlay.__sync();
    }
    document.querySelectorAll(".ncard").forEach((el) => el.classList.toggle("sel", !!(state.sel && state.sel.type === "node" && el.dataset.node === state.sel.id)));
  }

  function legend() {
    const sw = (style) => h("span", { class: "sw", style });
    const ln = (style) => h("span", { class: "ln", style });
    if (!state.legend) {
      return h("button", { class: "legend-btn", onclick: () => { state.legend = true; savePref("cv-legend", "1"); renderView(); }, html: icon("info", "sm") + "Legend" });
    }
    return h("div", { class: "legend" },
      h("button", { class: "icon-btn legend-x", title: "Hide legend", html: icon("close", "sm"), onclick: () => { state.legend = false; savePref("cv-legend", "0"); renderView(); } }),
      h("h6", null, "Reference status"),
      h("div", { class: "lg" }, sw({ border: "1.5px solid var(--text-2)" }), "Verified"),
      h("div", { class: "lg" }, sw({ border: "2.5px solid var(--bad)" }), "Contradicted"),
      h("div", { class: "lg" }, sw({ border: "2px dashed var(--warn)" }), "Unresolved"),
      h("div", { class: "lg" }, sw({ border: "2px dotted var(--agent)" }), h("i", null, "Agent statement")),
      h("h6", null, "Change"),
      h("div", { class: "lg" }, sw({ background: "var(--added-fill)", border: "1px solid var(--added)" }), "Added"),
      h("div", { class: "lg" }, sw({ background: "var(--modified-fill)", border: "1px solid var(--modified)" }), "Modified"),
      h("div", { class: "lg" }, sw({ background: "var(--deleted-fill)", border: "1px solid var(--deleted)" }), "Deleted"),
      h("div", { class: "lg" }, sw({ background: "var(--unchanged-fill)", border: "1px solid var(--text-3)" }), "Unchanged"),
      h("h6", null, "Edges"),
      h("div", { class: "lg" }, ln({}), "Verified site"),
      h("div", { class: "lg" }, ln({ borderTopStyle: "dotted", borderTopColor: "var(--text-3)" }), "No reference"));
  }

  // ── Tooltip ────────────────────────────────────────────────
  function showTooltip(ev, html) {
    if (!tooltipEl) { tooltipEl = h("div", { class: "tooltip" }); document.body.appendChild(tooltipEl); }
    tooltipEl.innerHTML = html;
    tooltipEl.classList.add("on");
    moveTooltip(ev);
  }
  function moveTooltip(ev) {
    if (!tooltipEl || !ev) return;
    const pad = 14, r = tooltipEl.getBoundingClientRect();
    let x = ev.clientX + pad, y = ev.clientY + pad;
    if (x + r.width > innerWidth - 8) x = ev.clientX - r.width - pad;
    if (y + r.height > innerHeight - 8) y = ev.clientY - r.height - pad;
    tooltipEl.style.left = x + "px";
    tooltipEl.style.top = y + "px";
  }
  function hideTooltip() { tooltipEl && tooltipEl.classList.remove("on"); }

  function reasonHTML(res) {
    if (res.status === "contradicted") return `<div class="tt-r tt-bad">✕ ${esc(res.reason || "contradicted")}</div>`;
    if (res.status === "unresolved") return `<div class="tt-r tt-warn">? ${esc(res.reason || "unresolved")}</div>`;
    if (res.status === "agent") return `<div class="tt-r"><i>No reference: the agent's word</i></div>`;
    return "";
  }
  function nodeTooltip(n) {
    return `<div class="tt-t">${esc(n.label)}</div><div>${esc(CHANGE_LABEL[n.change])} ${esc(n.kind)}${n.ref ? " · " + esc(n.ref.repo + " " + refText(n.ref)) : ""}</div>${reasonHTML(n.result)}`;
  }
  function edgeTooltip(e) {
    const a = nodeById.get(e.from), b = nodeById.get(e.to);
    return `<div class="tt-t">${esc(a.label)} → ${esc(e.kind)} → ${esc(b.label)}</div>${e.ref ? `<div>${esc(e.ref.repo + " " + refText(e.ref))}</div>` : ""}${reasonHTML(e.result)}`;
  }

  // ── Node cards (flows) ─────────────────────────────────────
  function nodeCard(n, small) {
    const observed = n.meta && n.meta.find((m) => m.key === "observed");
    const el = h("button", { class: `ncard ${n.change} ${n.result.status}${n.kind === "test" ? " test" : ""}`, "data-node": n.id, onclick: () => select({ type: "node", id: n.id }),
      onmouseenter: (ev) => showTooltip(ev, nodeTooltip(n)), onmousemove: moveTooltip, onmouseleave: hideTooltip },
      h("div", { class: "ic", html: icon(kindIcon(n.kind)) }),
      h("div", { class: "tx" },
        h("div", { class: "lb" }, n.label),
        h("div", { class: "sb" }, `${n.kind} · ${nodeSub(n)}`),
        observed ? h("span", { class: "says " + observedClass(observed.value) }, `agent says: ${observed.value}`) : null),
      n.result.status === "contradicted" || n.result.status === "unresolved" ? h("div", { class: `st ${n.result.status}`, html: icon(STATUS_ICON[n.result.status], "sm") }) : null);
    if (small) el.style.width = "200px";
    return el;
  }

  // ── Flows view ─────────────────────────────────────────────
  function flowsView() {
    if (!D.flows.length) {
      viewEl.appendChild(h("div", { class: "empty" }, h("div", null, raw(icon("flows")), h("div", null, "The agent described no flows."))));
      return;
    }
    const wrap = h("div", { class: "flows" });
    D.flows.forEach((f, fi) => {
      const tests = (f.tests || []).map((id) => nodeById.get(id)).filter(Boolean);
      const attached = new Map();
      const loose = [];
      for (const t of tests) {
        const target = f.path.find((pid) => (outEdges.get(t.id) || []).some((e) => e.kind === "tests" && e.to === pid));
        if (target) {
          if (!attached.has(target)) attached.set(target, []);
          attached.get(target).push(t);
        } else loose.push(t);
      }
      const statuses = f.path.map((id) => nodeById.get(id)?.result.status);
      const bad = statuses.filter((s) => s === "contradicted" || s === "unresolved").length;
      const card = h("section", { class: "flow" });
      card.appendChild(h("div", { class: "flow-head" },
        h("span", { html: icon("flows"), style: { color: "var(--accent)" } }),
        h("h3", null, f.title),
        h("span", { class: "badge mono" }, f.id),
        h("div", { class: "meta" },
          h("span", { class: "badge" }, plural(f.path.length, "step")),
          h("span", { class: "badge", html: icon("test", "sm") + esc(plural(tests.length, "test")) }),
          bad ? h("span", { class: "badge contradicted" }, plural(bad, "issue")) : null)));
      const lane = h("div", { class: "lane" });
      f.path.forEach((id, i) => {
        const n = nodeById.get(id);
        if (!n) return;
        if (i > 0) lane.appendChild(connector(f.path[i - 1], id));
        const step = h("div", { class: "step" }, h("div", { class: "step-n" }, `${String(i + 1).padStart(2, "0")}`), nodeCard(n));
        const att = attached.get(id) || [];
        if (att.length) {
          const box = h("div", { class: "attached" });
          for (const t of att) box.append(h("div", { class: "stem" }), nodeCard(t, true));
          step.appendChild(box);
        }
        lane.appendChild(step);
      });
      card.appendChild(lane);
      if (loose.length || !tests.length) {
        card.appendChild(h("div", { class: "flow-foot" },
          tests.length ? [h("span", null, "Tests on this flow without a `tests` edge to a step:"), loose.map((t) => nodeChip(t))]
            : h("span", { class: "muted", html: icon("info", "sm") + " No tests listed for this flow." })));
      }
      wrap.appendChild(card);
    });
    viewEl.appendChild(wrap);
    highlightSelection();
  }

  function connector(a, b) {
    const fwd = (outEdges.get(a) || []).find((e) => e.to === b);
    const rev = !fwd && (outEdges.get(b) || []).find((e) => e.to === a);
    const e = fwd || rev;
    const cls = e ? e.result.status : "none";
    const c = h("div", { class: `connector ${cls}${rev ? " rev" : ""}` }, h("div", { class: "line" }));
    if (e) {
      c.appendChild(h("span", { class: "ek", onclick: () => select({ type: "edge", id: e.id }), onmouseenter: (ev) => showTooltip(ev, edgeTooltip(e)), onmouseleave: hideTooltip }, e.kind));
    }
    return c;
  }

  function nodeChip(n) {
    const color = { verified: "var(--ok)", contradicted: "var(--bad)", unresolved: "var(--warn)", agent: "var(--agent)" }[n.result.status];
    return h("button", { class: "chip" + (n.kind === "test" ? " test" : ""), onclick: () => select({ type: "node", id: n.id }), title: `${n.kind} · ${STATUS_LABEL[n.result.status]}` },
      h("span", { class: "dot", style: { background: color } }), n.label);
  }

  // ── Diff map ───────────────────────────────────────────────
  function commitOptions() {
    const opts = [];
    for (const r of D.repos) {
      if (r.role === "context" || r.commits.length < 2) continue;
      for (const c of r.commits) opts.push({ value: `${r.id}@${c.sha}`, label: `${r.id} · ${c.worktree ? "worktree" : c.short} · ${c.subject || ""}`.slice(0, 80) });
    }
    return opts;
  }

  function hunkInCommit(f, hk) {
    if (!state.commitFilter) return true;
    const [repo, sha] = state.commitFilter.split("@");
    return f.repo === repo && (hk.commits || []).includes(sha);
  }

  function diffView() {
    const copts = commitOptions();
    if (copts.length) {
      toolbarEl.appendChild(h("select", { class: "select", title: "Show the hunks each commit wrote", onchange: (ev) => { state.commitFilter = ev.target.value; renderView(); } },
        h("option", { value: "" }, "All commits"),
        copts.map((o) => h("option", { value: o.value, selected: state.commitFilter === o.value }, o.label))));
      toolbarEl.appendChild(h("span", { class: "tb-sep" }));
    }
    toolbarEl.appendChild(h("div", { class: "seg" },
      ["all", "uncovered", "covered"].map((f) => h("button", { class: state.diffFilter === f ? "on" : "", onclick: () => { state.diffFilter = f; renderView(); } },
        { all: "All files", uncovered: "Needs attention", covered: "Covered" }[f]))));
    toolbarEl.appendChild(h("span", { class: "tb-sep" }));
    toolbarEl.appendChild(h("button", { class: "icon-btn", title: "Expand all", html: icon("layers"), onclick: () => {
      const all = D.files.every((f) => state.openFiles.has(f.id));
      D.files.forEach((f) => (all ? state.openFiles.delete(f.id) : state.openFiles.add(f.id)));
      renderView();
    } }));

    const wrap = h("div", { class: "diffmap" });
    const hunks = D.files.flatMap((f) => f.hunks.length ? f.hunks : [{ coverage: f.coverage }]);
    const cov = { covered: 0, partial: 0, uncovered: 0 };
    hunks.forEach((x) => cov[x.coverage]++);
    const total = hunks.length || 1;
    const added = D.files.reduce((a, f) => a + f.added, 0), removed = D.files.reduce((a, f) => a + f.removed, 0);
    wrap.appendChild(h("div", { class: "dm-summary" },
      h("div", { class: "dm-stat" }, h("span", { class: "v" }, D.files.length), h("span", { class: "k" }, "files")),
      h("div", { class: "dm-stat" }, h("span", { class: "v" }, hunks.length), h("span", { class: "k" }, "hunks")),
      h("div", { class: "dm-stat" }, h("span", { class: "v plus" }, "+" + added), h("span", { class: "k" }, "added")),
      h("div", { class: "dm-stat" }, h("span", { class: "v minus" }, "−" + removed), h("span", { class: "k" }, "removed")),
      h("div", { class: "covbar" },
        h("div", { class: "bar" }, ["covered", "partial", "uncovered"].map((k) => h("span", { class: "c-" + k, style: { width: (cov[k] / total) * 100 + "%" } }))),
        h("div", { class: "lbl" },
          h("span", null, h("i", { class: "c-covered" }), `${cov.covered} covered by nodes`),
          h("span", null, h("i", { class: "c-partial" }), `${cov.partial} partly covered`),
          h("span", null, h("i", { class: "c-uncovered" }), `${cov.uncovered} unmentioned`)))));

    if (!D.files.length) {
      wrap.appendChild(h("div", { class: "empty" }, "No changed files in the listed repos."));
      viewEl.appendChild(wrap);
      return;
    }

    for (const r of D.repos) {
      let files = D.files.filter((f) => f.repo === r.id);
      if (!files.length) continue;
      if (state.commitFilter && !state.commitFilter.startsWith(r.id + "@")) continue;
      if (state.diffFilter === "uncovered") files = files.filter((f) => f.coverage !== "covered");
      if (state.diffFilter === "covered") files = files.filter((f) => f.coverage === "covered");
      if (state.commitFilter) files = files.filter((f) => f.hunks.some((hk) => hunkInCommit(f, hk)));
      const sec = h("section", { class: "dm-repo" });
      sec.appendChild(h("div", { class: "dm-repo-head", html: icon("repo", "sm") },
        h("span", { class: "name" }, r.id),
        h("span", { class: "muted mono" }, r.path),
        h("span", { class: "spacer" }),
        h("span", { class: "badge mono" }, r.worktree ? "worktree" : short(r.commits[0]?.sha)),
        h("span", { class: "muted", style: { fontSize: "12px" } }, plural(D.files.filter((f) => f.repo === r.id).length, "file"))));
      if (!files.length) sec.appendChild(h("div", { class: "code-note" }, "No files match this filter."));
      for (const f of files) sec.append(...fileRow(f));
      wrap.appendChild(sec);
    }
    viewEl.appendChild(wrap);
  }

  function fileRow(f) {
    const open = state.openFiles.has(f.id);
    const statusLetter = { added: "A", modified: "M", deleted: "D", renamed: "R" }[f.status];
    const lines = Math.max(f.lines, 1);
    const track = h("div", { class: "track", title: "Hunks along the file: green covered, amber partly covered, red unmentioned" });
    for (const hk of f.hunks) {
      const start = f.status === "deleted" || hk.newCount === 0 ? (f.status === "deleted" ? hk.oldStart : hk.newStart) : hk.newStart;
      const len = f.status === "deleted" ? hk.oldCount : Math.max(hk.newCount, 1);
      track.appendChild(h("span", { class: "seg-h c-" + hk.coverage, style: { left: ((start - 1) / lines) * 100 + "%", width: (len / lines) * 100 + "%" } }));
    }
    if (!f.hunks.length) track.appendChild(h("span", { class: "seg-h c-" + f.coverage, style: { left: 0, width: "100%", opacity: 0.5 } }));
    const covIcon = { covered: ["checkcircle", "var(--ok)"], partial: ["alert", "var(--warn)"], uncovered: ["xcircle", "var(--bad)"] }[f.coverage];
    const row = h("div", { class: "frow" + (open ? " open" : ""), onclick: () => { open ? state.openFiles.delete(f.id) : state.openFiles.add(f.id); renderView(); } },
      h("span", { class: "chev", html: icon("chevron", "sm") }),
      h("span", { class: "fstat " + f.status, title: CHANGE_LABEL[f.status] }, statusLetter),
      h("div", { style: { minWidth: 0 } },
        h("div", { class: "fpath", title: f.path }, h("bdi", null, h("span", { class: "dir" }, dirname(f.path)), h("span", { class: "base" }, basename(f.path)))),
        h("div", { class: "fsub" }, [f.oldPath ? `renamed from ${f.oldPath}` : null, f.untracked ? "untracked" : null, f.binary ? "binary" : null, f.language || "plain text",
          f.hasTests ? "test file" : null].filter(Boolean).join(" · "))),
      h("div", { style: { display: "flex", alignItems: "center", gap: "8px" } }, h("span", { html: icon(covIcon[0], "sm"), style: { color: covIcon[1] } }), h("div", { class: "track", style: { flex: 1 } }, ...track.childNodes)),
      h("div", { class: "fnums" }, h("span", { class: "plus" }, `+${f.added}`), " ", h("span", { class: "minus" }, `−${f.removed}`)));
    if (!open) return [row];
    const body = h("div", { class: "hunks" });
    if (f.fileNodes && f.fileNodes.length) {
      body.appendChild(h("div", { class: "hunk-head", style: { border: 0, background: "none", padding: 0 } }, h("span", { class: "muted" }, "Whole file covered by"), f.fileNodes.map((id) => nodeChip(nodeById.get(id)))));
    }
    if (!f.hunks.length) body.appendChild(h("div", { class: "code-note" }, f.binary ? "Binary file: no line diff." : "Renamed without content change."));
    for (const hk of f.hunks) if (hunkInCommit(f, hk)) body.appendChild(hunkBox(f, hk));
    return [row, body];
  }

  function hunkRange(f, hk) {
    if (hk.newCount === 0) return `−${hk.oldStart}${hk.oldCount > 1 ? "–" + (hk.oldStart + hk.oldCount - 1) : ""} (removed after line ${hk.newStart})`;
    return `+${hk.newStart}${hk.newCount > 1 ? "–" + (hk.newStart + hk.newCount - 1) : ""}${hk.oldCount ? `  −${hk.oldStart}${hk.oldCount > 1 ? "–" + (hk.oldStart + hk.oldCount - 1) : ""}` : ""}`;
  }

  function hunkBox(f, hk) {
    const label = { covered: "Covered", partial: "Partly covered", uncovered: "Not mentioned by any node" }[hk.coverage];
    const box = h("div", { class: "hunk " + hk.coverage, id: "hunk-" + hk.id });
    box.appendChild(h("div", { class: "hunk-head" },
      h("span", { class: "rng" }, hunkRange(f, hk)),
      h("span", { class: "badge " + ({ covered: "verified", partial: "unresolved", uncovered: "contradicted" }[hk.coverage]) }, label),
      hk.uncovered && hk.coverage === "partial" ? h("span", { class: "muted", style: { fontSize: "11.5px" } }, `uncovered ${hk.oldSide ? "initial " : ""}lines ${hk.uncovered.map((r) => (r[0] === r[1] ? r[0] : r[0] + "–" + r[1])).join(", ")}`) : null,
      h("span", { class: "spacer" }),
      (hk.commits || []).map((c) => h("span", { class: "badge mono", title: "Written by " + (c === "WORKTREE" ? "uncommitted work" : c) }, c === "WORKTREE" ? "worktree" : short(c))),
      hk.nodes.map((id) => nodeChip(nodeById.get(id))),
      h("button", { class: "icon-btn", title: "Open in inspector", html: icon("arrow", "sm"), onclick: (ev) => { ev.stopPropagation(); select({ type: "hunk", id: hk.id }); } })));
    box.appendChild(codeTable(hk.rows, null));
    return box;
  }

  // ── Code excerpt ───────────────────────────────────────────
  function codeTable(rows, site) {
    const tbody = h("tbody");
    for (const r of rows) {
      if (r.t === "gap") { tbody.appendChild(h("tr", { class: "gap" }, h("td", { colspan: 4 }, "⋯"))); continue; }
      let codeHTML = r.h || "";
      const isSite = site && r.n === site.line;
      if (isSite && site.contains) codeHTML = markContains(codeHTML, site.contains);
      const sign = r.t === "add" ? "+" : r.t === "del" ? "−" : "";
      tbody.appendChild(h("tr", { class: `${r.t}${r.f ? " f" : ""}${isSite ? " site" : ""}` },
        h("td", { class: "ln" }, r.o || ""), h("td", { class: "ln" }, r.n || ""), h("td", { class: "sg" }, sign), h("td", { class: "cd", html: codeHTML || " " })));
    }
    return h("div", { class: "code-scroll" }, h("table", { class: "code" }, tbody));
  }

  // Wraps the first plain-text occurrence of needle in <mark>, across spans.
  function markContains(html, needle) {
    const tmp = document.createElement("div");
    tmp.innerHTML = html;
    const text = tmp.textContent;
    const idx = text.indexOf(needle);
    if (idx < 0) return html;
    let pos = 0;
    const walker = document.createTreeWalker(tmp, NodeFilter.SHOW_TEXT);
    const parts = [];
    let node;
    while ((node = walker.nextNode())) {
      const len = node.nodeValue.length;
      const s = Math.max(idx, pos), e = Math.min(idx + needle.length, pos + len);
      if (s < e) parts.push([node, s - pos, e - pos]);
      pos += len;
    }
    for (const [tn, s, e] of parts.reverse()) {
      const after = tn.splitText(s);
      after.splitText(e - s);
      const m = document.createElement("mark");
      after.parentNode.replaceChild(m, after);
      m.appendChild(after);
    }
    return tmp.innerHTML;
  }

  function excerptBox(exId, ref, opts = {}) {
    const x = exId && D.excerpts[exId];
    if (!x) return null;
    const stateLabel = x.state === "WORKTREE" ? "worktree" : short(x.state);
    const kindLabel = { final: "final state", initial: "initial state", at: "at commit" }[x.stateKind] || "";
    const box = h("div", { class: "code-box" });
    box.appendChild(h("div", { class: "code-head", html: icon("file", "sm") },
      h("span", { class: "fp", title: `${x.repo}: ${x.file}` }, `${x.file}${linesText(ref)}`),
      x.oldPath ? h("span", { class: "badge renamed", title: "Renamed from " + x.oldPath }, "renamed") : null,
      h("span", { class: "badge mono", title: kindLabel }, `${stateLabel}`),
      h("span", { class: "badge" }, x.language || "plain text"),
      h("button", { class: "icon-btn copy", title: "Copy path", html: icon("copy", "sm"), onclick: () => navigator.clipboard && navigator.clipboard.writeText(`${x.file}${ref && ref.lines ? ":" + ref.lines[0] : ref && ref.line ? ":" + ref.line : ""}`) })));
    if (x.note) box.appendChild(h("div", { class: "code-note" }, x.note));
    else {
      const site = ref && ref.type === "site" ? { line: opts.actualLine || ref.line, contains: ref.contains } : null;
      box.appendChild(codeTable(x.rows || [], site));
    }
    if (x.truncated) box.appendChild(h("div", { class: "trunc" }, "Excerpt truncated."));
    return box;
  }

  // ── Claims ─────────────────────────────────────────────────
  function claimsView() {
    if (!D.claims.length) {
      viewEl.appendChild(h("div", { class: "empty" }, h("div", null, raw(icon("claims")), h("div", null, "The agent made no claims."))));
      return;
    }
    const counts = {};
    D.claims.forEach((c) => (counts[c.status] = (counts[c.status] || 0) + 1));
    toolbarEl.appendChild(h("div", { class: "seg" },
      ["all", "verified", "contradicted", "unresolved", "agent"].filter((s) => s === "all" || counts[s]).map((s) =>
        h("button", { class: state.claimFilter === s ? "on" : "", onclick: () => { state.claimFilter = s; renderView(); } },
          s === "all" ? `All ${D.claims.length}` : `${STATUS_LABEL[s]} ${counts[s]}`))));
    const wrap = h("div", { class: "claims" });
    D.claims.forEach((c, ci) => {
      if (state.claimFilter !== "all" && c.status !== state.claimFilter) return;
      const card = h("article", { class: "claim " + c.status });
      card.appendChild(h("div", { class: "claim-head" },
        h("div", { class: "claim-icon " + c.status, html: icon(STATUS_ICON[c.status]) }),
        h("div", { style: { flex: 1, minWidth: 0 } },
          h("div", { class: "claim-text" + (c.status === "agent" ? " agent" : "") }, c.text),
          h("div", { class: "claim-about" },
            statusBadge(c.status),
            c.evidence.length ? h("span", { class: "muted", style: { fontSize: "12px" } }, plural(c.evidence.length, "piece") + " of evidence") : h("span", { class: "muted", style: { fontSize: "12px" } }, "no evidence: the agent's word"),
            c.about.length ? h("span", { class: "muted", style: { fontSize: "12px", marginLeft: "6px" } }, "about") : null,
            c.about.map((id) => nodeById.get(id) && nodeChip(nodeById.get(id)))))));
      if (c.evidence.length) {
        const evs = h("div", { class: "evidence" });
        c.evidence.forEach((ev, ei) => {
          const key = `${ci}:${ei}`;
          const open = state.openFiles.has("ev:" + key);
          evs.appendChild(h("div", { class: "ev-row" + (open ? " open" : ""), onclick: () => { open ? state.openFiles.delete("ev:" + key) : state.openFiles.add("ev:" + key); renderView(); } },
            h("span", { class: "chev", html: icon("chevron", "sm") }),
            h("span", { class: "sdot " + ev.result.status, title: STATUS_LABEL[ev.result.status] }),
            h("span", { class: "badge mono" }, ev.ref.type),
            h("span", { class: "ev-loc" }, h("b", { style: { color: "var(--accent-text)" } }, ev.ref.repo), " ", refText(ev.ref)),
            ev.ref.contains ? h("span", { class: "ev-contains", title: ev.ref.contains }, ev.ref.contains) : null,
            h("span", { class: "spacer" }),
            statusBadge(ev.result.status)));
          if (open) evs.appendChild(h("div", { class: "ev-body" }, resolvedFact(ev.ref, ev.result, ev.excerpt)));
        });
        card.appendChild(evs);
      }
      wrap.appendChild(card);
    });
    viewEl.appendChild(wrap);
  }

  function resolvedFact(ref, res, exId) {
    const parts = [];
    if (res.status === "contradicted" || res.status === "unresolved") parts.push(reasonCallout(res));
    if (res.codes && res.codes.includes("site_offset")) parts.push(h("div", { class: "callout warn", style: { marginBottom: "8px" }, html: icon("alert", "sm") }, h("div", null, `Found on line ${res.actualLine}, not ${ref.line}.`)));
    if (ref.type === "commit") {
      const repo = repoById.get(ref.repo);
      const c = repo && repo.commits.find((c) => c.sha === res.state || c.sha.startsWith(ref.sha));
      parts.push(h("div", { class: "repo-card" }, h("div", { class: "commits" }, c ? h("div", { class: "commit" }, h("span", { class: "dot" }), h("span", { class: "sha" }, c.short), h("span", { class: "subj" }, c.subject)) : h("div", { class: "code-note" }, "Commit not in the list."))));
    } else {
      const box = excerptBox(exId, ref, { actualLine: res.actualLine });
      if (box) parts.push(box);
      else if (res.status === "verified") parts.push(h("div", { class: "code-note" }, "No excerpt available."));
    }
    return parts;
  }

  function reasonCallout(res) {
    const cls = res.status === "contradicted" ? "bad" : "warn";
    return h("div", { class: `callout ${cls}`, style: { marginBottom: "8px" }, html: icon(res.status === "contradicted" ? "xcircle" : "help") },
      h("div", null,
        h("div", null, h("b", null, STATUS_LABEL[res.status] + ": "), res.reason || ""),
        res.hint ? h("div", { class: "hint" }, "Hint for the agent: ", res.hint) : null,
        res.codes && res.codes.length ? h("div", { class: "code" }, res.codes.join(" · ")) : null));
  }

  // ── Inspector ──────────────────────────────────────────────
  function renderInspector() {
    inspectorEl.innerHTML = "";
    const sel = state.sel;
    if (!sel) { inspectorEl.classList.remove("on"); return; }
    let content = null;
    if (sel.type === "node" && nodeById.has(sel.id)) content = nodeInspector(nodeById.get(sel.id));
    else if (sel.type === "edge" && edgeById.has(sel.id)) content = edgeInspector(edgeById.get(sel.id));
    else if (sel.type === "hunk") content = hunkInspector(sel.id);
    else if (sel.type === "findings") content = findingsInspector(sel.id);
    if (!content) { inspectorEl.classList.remove("on"); return; }
    inspectorEl.classList.add("on");
    inspectorEl.append(...content);
    setTimeout(() => overlay && overlay.__sync && overlay.__sync(), 200);
  }

  function insHead(iconName, iconCls, title, sub, badges) {
    return h("div", { class: "ins-head" },
      h("div", { class: "ins-icon " + (iconCls || ""), html: icon(iconName, "lg") }),
      h("div", { class: "ins-title" }, h("h2", null, title), sub ? h("div", { class: "sub" }, sub) : null, badges ? h("div", { class: "ins-badges" }, badges) : null),
      h("button", { class: "icon-btn", title: "Close (Esc)", html: icon("close"), onclick: () => select(null) }));
  }
  const sec = (title, iconName, ...kids) => h("section", { class: "ins-sec" }, h("h4", { html: (iconName ? icon(iconName, "sm") : "") + esc(title) }), ...kids);

  function locLine(ref, res) {
    if (!ref) return null;
    const repo = repoById.get(ref.repo);
    return h("div", { class: "loc" },
      h("span", { class: "repo" }, ref.repo), h("span", { class: "sep" }, "›"),
      ref.type === "commit" ? h("span", null, "commit " + short(ref.sha)) : h("span", null, ref.file + linesText(ref)),
      h("span", { class: "badge mono", title: "Reference type" }, ref.type),
      res && res.state ? h("span", { class: "badge mono", title: { final: "Read in the final state", initial: "Read in the initial state (deleted node)", at: "Read at an intermediate commit" }[res.stateKind] }, (res.stateKind === "initial" ? "initial " : res.stateKind === "at" ? "at " : "") + short(res.state)) : null,
      repo && repo.role === "context" ? h("span", { class: "badge context" }, "context repo") : null);
  }

  function elementFindings(path) {
    return allFindings.filter((f) => f.path && (f.path === path || f.path.startsWith(path + "/")));
  }

  function nodeInspector(n) {
    const out = [insHead(kindIcon(n.kind), n.change, n.label, `${n.id} · ${n.kind}`, [changeBadge(n.change), statusBadge(n.result.status)])];
    const body = h("div", { class: "ins-body" });
    out.push(body);

    if (n.result.status === "contradicted" || n.result.status === "unresolved") body.appendChild(h("div", { class: "ins-sec" }, reasonCallout(n.result)));
    if (n.result.status === "agent") body.appendChild(h("div", { class: "ins-sec" }, h("div", { class: "callout info", html: icon("chat") }, h("div", null, "This node has no reference. Its existence and change state are the agent's word."))));

    if (n.ref) body.appendChild(sec("Location", "file", locLine(n.ref, n.result)));

    if (n.why || (n.meta && n.meta.length)) {
      const blocks = [];
      if (n.why) blocks.push(h("div", { class: "agent-says" }, agentTag("Why · agent says"), h("p", null, n.why)));
      if (n.meta && n.meta.length) {
        blocks.push(h("dl", { class: "kvs", style: { marginTop: n.why ? "10px" : 0 } }, n.meta.map((m) => [h("dt", null, m.key), h("dd", { class: "says" },
          m.key === "observed" ? h("span", { class: "says-badge badge agent " + observedClass(m.value) }, "agent says: " + m.value) : h("span", { class: m.key === "command" ? "mono" : "" }, m.value))])));
      }
      body.appendChild(sec("Agent's account", "chat", ...blocks));
    }

    const finds = elementFindings(`/nodes/${n.index}`).filter((f) => !(f.status && f.path.endsWith("/ref") && n.result.codes && n.result.reason === f.message));
    const extra = allFindings.filter((f) => f.code === "untested_change" && f.path === `/nodes/${n.index}`);
    const nodeFinds = [...new Set(finds.concat(extra))].filter((f) => f.level === "warning" || !f.status);
    if (nodeFinds.length) body.appendChild(sec("Findings", "alert", h("div", { class: "flist" }, nodeFinds.map((f) => findingItem(f, false)))));

    if (n.kind !== "test") {
      const tests = testsOf(n.id);
      body.appendChild(sec(`Tests (${tests.length})`, "test",
        tests.length ? h("div", { class: "rel" }, tests.map((e) => {
          const t = nodeById.get(e.from);
          const obs = t.meta && t.meta.find((m) => m.key === "observed");
          return h("div", { class: "rel-row", onclick: () => select({ type: "node", id: t.id }) },
            h("span", { class: "sdot " + t.result.status, title: "Test node: " + STATUS_LABEL[t.result.status] }),
            h("span", null, t.label),
            h("span", { class: "badge " + t.change }, CHANGE_LABEL[t.change]),
            obs ? h("span", { class: "badge agent " + observedClass(obs.value) }, "agent says: " + obs.value) : null,
            h("span", { class: "where" }, t.ref ? basename(t.ref.file || "") + linesText(t.ref) : ""));
        })) : h("div", { class: "muted", style: { fontSize: "12.5px" } }, n.change === "unchanged" ? "No tests attached." : "No test node has a `tests` edge to this node.")));
    } else {
      const targets = testedBy(n.id);
      body.appendChild(sec(`Tests (${targets.length})`, "target",
        targets.length ? h("div", { class: "rel" }, targets.map((e) => relRow(e, "out"))) : h("div", { class: "muted" }, "No `tests` edges from this test.")));
    }

    const ins = (inEdges.get(n.id) || []).filter((e) => !(n.kind !== "test" && e.kind === "tests"));
    const outs = (outEdges.get(n.id) || []).filter((e) => !(n.kind === "test" && e.kind === "tests"));
    if (ins.length || outs.length) {
      body.appendChild(sec("Relations", "graph", h("div", { class: "rel" }, ins.map((e) => relRow(e, "in")), outs.map((e) => relRow(e, "out")))));
    }

    if (n.excerpt) body.appendChild(sec(n.change === "deleted" ? "Code (initial state)" : "Code", "code", excerptBox(n.excerpt, n.ref, { actualLine: n.result.actualLine })));
    return out;
  }

  function relRow(e, dir) {
    const other = nodeById.get(dir === "in" ? e.from : e.to);
    return h("div", { class: "rel-row", onclick: () => select({ type: "edge", id: e.id }), title: STATUS_LABEL[e.result.status] },
      h("span", { class: "sdot " + e.result.status }),
      h("span", { class: "dir", title: dir === "in" ? "incoming" : "outgoing" }, dir === "in" ? "←" : "→"),
      h("span", { class: "k" }, e.kind),
      h("span", { class: "other", title: other.label }, other.label),
      h("span", { class: "where" }, e.ref ? basename(e.ref.file || "") + linesText(e.ref) : "no ref"));
  }

  function edgeInspector(e) {
    const a = nodeById.get(e.from), b = nodeById.get(e.to);
    const out = [insHead("arrow", "", `${a.label} → ${b.label}`, `edge · ${e.kind}`, [h("span", { class: "badge mono" }, e.kind), statusBadge(e.result.status)])];
    const body = h("div", { class: "ins-body" });
    out.push(body);
    if (e.result.status === "contradicted" || e.result.status === "unresolved") body.appendChild(h("div", { class: "ins-sec" }, reasonCallout(e.result)));
    if (!e.ref) body.appendChild(h("div", { class: "ins-sec" }, h("div", { class: "callout info", html: icon("chat") }, h("div", null, "No site reference: this relation is the agent's word, drawn dotted."))));
    body.appendChild(sec("Between", "graph", h("div", { style: { display: "flex", gap: "8px", alignItems: "center", flexWrap: "wrap" } }, nodeChip(a), h("span", { class: "muted" }, e.kind + " →"), nodeChip(b))));
    if (e.ref) {
      body.appendChild(sec("Site", "target", locLine(e.ref, e.result),
        e.ref.contains ? h("div", { style: { marginTop: "8px" } }, h("span", { class: "muted", style: { fontSize: "12px" } }, "contains "), h("code", { class: "ev-contains" }, e.ref.contains)) : null));
      const warn = allFindings.filter((f) => f.path === `/edges/${e.id.slice(1)}/ref` && f.level === "warning");
      if (warn.length) body.appendChild(sec("Findings", "alert", h("div", { class: "flist" }, warn.map((f) => findingItem(f, false)))));
      if (e.excerpt) body.appendChild(sec("Code", "code", excerptBox(e.excerpt, e.ref, { actualLine: e.result.actualLine })));
    }
    return out;
  }

  function hunkInspector(id) {
    for (const f of D.files) {
      const hk = f.hunks.find((x) => x.id === id);
      if (!hk) continue;
      const out = [insHead("diff", f.status, basename(f.path), `${f.repo} › ${f.path}`, [changeBadge(f.status), h("span", { class: "badge " + ({ covered: "verified", partial: "unresolved", uncovered: "contradicted" }[hk.coverage]) }, { covered: "Covered", partial: "Partly covered", uncovered: "Unmentioned" }[hk.coverage])])];
      const body = h("div", { class: "ins-body" });
      out.push(body);
      if (hk.coverage !== "covered") {
        body.appendChild(h("div", { class: "ins-sec" }, h("div", { class: "callout " + (hk.coverage === "uncovered" ? "bad" : "warn"), html: icon("alert") },
          h("div", null, hk.coverage === "uncovered" ? "No node's range or file ref covers this change. The agent did not describe it." : `Lines ${hk.uncovered.map((r) => (r[0] === r[1] ? r[0] : r[0] + "–" + r[1])).join(", ")} are outside every node.`))));
      }
      body.appendChild(sec("Hunk", "diff", h("div", { class: "loc" }, h("span", { class: "repo" }, f.repo), h("span", { class: "sep" }, "›"), h("span", null, f.path), h("span", { class: "badge mono" }, hunkRange(f, hk)))));
      if (hk.nodes.length) body.appendChild(sec("Covered by", "layers", h("div", { style: { display: "flex", gap: "6px", flexWrap: "wrap" } }, hk.nodes.map((nid) => nodeChip(nodeById.get(nid))))));
      body.appendChild(sec("Change", "code", h("div", { class: "code-box" }, codeTable(hk.rows, null))));
      return out;
    }
    return null;
  }

  function openFindings(group) {
    select({ type: "findings", id: group });
  }

  function findingsInspector(group) {
    let list, title;
    if (group === "errors") { list = allFindings.filter((f) => f.level === "error"); title = "Errors"; }
    else if (group === "warnings") { list = allFindings.filter((f) => f.level === "warning"); title = "Warnings"; }
    else if (group === "all") { list = allFindings; title = "All findings"; }
    else if (group === "tests") return changedTestsInspector();
    else {
      const g = GROUPS.find((x) => x.id === group);
      if (!g) return null;
      list = allFindings.filter(g.match);
      title = g.label;
    }
    const out = [insHead("alert", "", title, `${plural(list.length, "finding")} from validate`, null)];
    const body = h("div", { class: "ins-body" });
    out.push(body);
    if (!list.length) body.appendChild(h("div", { class: "ins-sec" }, h("div", { class: "all-clear", html: icon("checkcircle") + "Nothing here." })));
    else body.appendChild(h("div", { class: "ins-sec" }, h("div", { class: "flist" }, list.map((f) => findingItem(f, true)))));
    body.appendChild(h("div", { class: "ins-sec muted", style: { fontSize: "12px" } }, "Findings check the account against the repos, never whether the code is correct. Hints are written for the agent."));
    return out;
  }

  function changedTestsInspector() {
    const tests = changedTests();
    const out = [insHead("test", "", "Changed tests", "Test nodes that changed in the diff, next to the code they claim to test", null)];
    const body = h("div", { class: "ins-body" });
    out.push(body);
    body.appendChild(h("div", { class: "ins-sec" }, h("div", { class: "rel" }, tests.map((t) => {
      const targets = testedBy(t.id).map((e) => nodeById.get(e.to));
      const obs = t.meta && t.meta.find((m) => m.key === "observed");
      return h("div", { class: "rel-row", style: { flexWrap: "wrap" }, onclick: () => select({ type: "node", id: t.id }) },
        h("span", { class: "sdot " + t.result.status }), h("b", null, t.label), h("span", { class: "badge " + t.change }, CHANGE_LABEL[t.change]),
        obs ? h("span", { class: "badge agent " + observedClass(obs.value) }, "agent says: " + obs.value) : null,
        h("span", { class: "where" }, targets.length ? "tests " + targets.map((x) => x.label).join(", ") : "tests nothing"));
    }))));
    return out;
  }

  function findingTarget(f) {
    const m = f.path && f.path.match(/^\/(nodes|edges|claims)\/(\d+)/);
    if (m) {
      if (m[1] === "nodes") return { label: "Show node", go: () => { const n = D.nodes[+m[2]]; if (state.view === "diff" || state.view === "claims") setView("graph"); select({ type: "node", id: n.id }); } };
      if (m[1] === "edges") return { label: "Show edge", go: () => { if (state.view !== "graph" && state.view !== "flows") setView("graph"); select({ type: "edge", id: "e" + m[2] }); } };
      if (m[1] === "claims") return { label: "Show claim", go: () => { state.claimFilter = "all"; state.openFiles.add(`ev:${m[2]}:0`); setView("claims"); renderView(); } };
    }
    if (f.repo && f.file) {
      const file = D.files.find((x) => x.repo === f.repo && x.path === f.file);
      if (file) {
        let hk = null;
        if (f.lines) hk = file.hunks.find((x) => {
          const s = x.newCount ? x.newStart : x.oldStart, e = x.newCount ? x.newStart + x.newCount - 1 : x.oldStart + x.oldCount - 1;
          const a = x.oldSide ? x.oldStart : s, b = x.oldSide ? x.oldStart + x.oldCount - 1 : e;
          return f.lines[0] <= b && f.lines[1] >= a;
        });
        return { label: "Show in diff map", go: () => {
          state.openFiles.add(file.id);
          state.diffFilter = "all";
          if (state.view !== "diff") setView("diff"); else renderView();
          setTimeout(() => {
            const el = hk && document.getElementById("hunk-" + hk.id);
            if (el) { el.scrollIntoView({ behavior: "smooth", block: "center" }); el.animate([{ outline: "2px solid var(--accent)" }, { outline: "2px solid transparent" }], { duration: 1600 }); }
          }, 30);
        } };
      }
    }
    return null;
  }

  function findingItem(f, withGo) {
    const where = [f.path, f.repo && f.file ? `${f.repo} › ${f.file}${f.lines ? ":" + (f.lines[0] === f.lines[1] ? f.lines[0] : f.lines[0] + "–" + f.lines[1]) : ""}` : f.repo].filter(Boolean).join("  ");
    const target = withGo && findingTarget(f);
    return h("div", { class: "fitem " + (f.level === "error" ? "err" : "warn") },
      h("div", { class: "top" }, h("span", { class: "code-n" }, f.code), f.status ? statusBadge(f.status) : null, h("span", { class: "where" }, where)),
      h("div", { class: "msg" }, f.message),
      f.hint ? h("div", { class: "hint" }, "Hint: ", f.hint) : null,
      target ? h("div", { class: "go" }, h("button", { class: "link", onclick: target.go }, target.label + " →")) : null);
  }

  // ── Keyboard ───────────────────────────────────────────────
  document.addEventListener("keydown", (ev) => {
    if (ev.target.closest && ev.target.closest("input, textarea, select")) return;
    if (ev.key === "Escape") select(null);
    if (ev.key === "/" && state.view === "graph") { const s = document.querySelector(".search"); if (s) { ev.preventDefault(); s.focus(); } }
    const i = ["1", "2", "3", "4"].indexOf(ev.key);
    if (i >= 0 && !ev.metaKey && !ev.ctrlKey) setView(VIEWS[i].id);
  });

  const mq = matchMedia("(prefers-color-scheme: dark)");
  mq.addEventListener && mq.addEventListener("change", () => { if (state.theme === "auto" && state.view === "graph") renderView(); });

  readHash();
  render();
})();
