/* kpm analyze-local charts — fetch API + canvas series */
(function () {
  "use strict";

  var TOKEN_KEY = "kpm_api_token";
  var THEME_KEY = "kpm_theme";
  var state = {
    window: "24h",
    metric: "cpu_util_mcores",
    since: "",
    until: "",
    token: sessionStorage.getItem(TOKEN_KEY) || "",
    filters: { namespace: "", owner: "", grain: "", cpu: "", mem: "" },
  };
  var filterTimer = null;

  var el = {
    auth: document.getElementById("auth"),
    app: document.getElementById("app"),
    grid: document.getElementById("grid"),
    status: document.getElementById("status"),
    modeLabel: document.getElementById("mode-label"),
    clusterTarget: document.getElementById("cluster-target"),
    recSummary: document.getElementById("recommend-summary"),
    nodesPanel: document.getElementById("nodes-panel"),
    nodesMeta: document.getElementById("nodes-meta"),
    nodesBody: document.getElementById("nodes-body"),
    tokenInput: document.getElementById("token"),
    authErr: document.getElementById("auth-err"),
    drawer: document.getElementById("drawer"),
    backdrop: document.getElementById("drawer-backdrop"),
    drawerTitle: document.getElementById("drawer-title"),
    drawerBody: document.getElementById("drawer-body"),
  };

  var recByGrain = {};
  var recByOwner = {};

  function apiURL(path, params) {
    var u = new URL(path, window.location.origin);
    if (params) {
      Object.keys(params).forEach(function (k) {
        if (params[k] != null && params[k] !== "") u.searchParams.set(k, params[k]);
      });
    }
    return u.toString();
  }

  function headers() {
    var h = { Accept: "application/json" };
    if (state.token) h.Authorization = "Bearer " + state.token;
    return h;
  }

  function fetchJSON(path, params) {
    return fetch(apiURL(path, params), { headers: headers() }).then(function (r) {
      if (r.status === 401) {
        showAuth("Bearer token required");
        var err = new Error("unauthorized");
        err.code = 401;
        throw err;
      }
      var ct = (r.headers.get("content-type") || "").toLowerCase();
      var parse = ct.indexOf("application/json") >= 0
        ? r.json()
        : r.text().then(function (t) {
            return { error: (t || r.statusText || "error").trim() };
          });
      return parse.then(function (body) {
        if (!r.ok) {
          var e = new Error((body && body.error) || r.statusText || "request failed");
          e.code = r.status;
          throw e;
        }
        return body;
      });
    });
  }

  function showAuth(msg) {
    el.auth.classList.remove("hidden");
    el.app.classList.add("hidden");
    if (msg) el.authErr.textContent = msg;
  }

  function showApp() {
    el.auth.classList.add("hidden");
    el.app.classList.remove("hidden");
  }

  function windowParams() {
    if (state.since && state.until) {
      return { since: state.since, until: state.until };
    }
    return { window: state.window };
  }

  function workloadQueryParams() {
    var p = windowParams();
    if (state.filters.namespace) p.namespace = state.filters.namespace;
    if (state.filters.owner) p.workload = state.filters.owner;
    return p;
  }

  function filtersActive() {
    var f = state.filters;
    return !!(f.namespace || f.owner || f.grain || f.cpu || f.mem);
  }

  function filterItems(items) {
    var grain = (state.filters.grain || "").toLowerCase();
    var cpu = state.filters.cpu;
    var mem = state.filters.mem;
    if (!grain && !cpu && !mem) return items || [];
    return (items || []).filter(function (item) {
      if (grain && String(item.grain_key || "").toLowerCase().indexOf(grain) < 0) {
        return false;
      }
      if (cpu || mem) {
        var rec = lookupRec(item);
        if (cpu) {
          var cv = rec ? rec.cpu_verdict || "n/a" : "none";
          if (cv !== cpu) return false;
        }
        if (mem) {
          var mv = rec ? rec.mem_verdict || "n/a" : "none";
          if (mv !== mem) return false;
        }
      }
      return true;
    });
  }

  var METRICS = {
    cpu_util_mcores: {
      label: "CPU util",
      unit: "mcores",
      compare: "green=util · gray=request · orange=limit",
      fmt: function (v) {
        return Math.round(v) + "m";
      },
    },
    mem_util_bytes: {
      label: "Mem util",
      unit: "bytes",
      compare: "green=util · gray=request · orange=limit",
      fmt: function (v) {
        return formatBytes(v);
      },
    },
    cpu_util_ratio: {
      label: "CPU vs request",
      unit: "% of request",
      compare: "100%=request · orange=limit",
      fmt: function (v) {
        return (v * 100).toFixed(0) + "%";
      },
    },
    mem_util_ratio: {
      label: "Mem vs request",
      unit: "% of request",
      compare: "100%=request · orange=limit",
      fmt: function (v) {
        return (v * 100).toFixed(0) + "%";
      },
    },
  };

  function metricInfo(id) {
    return METRICS[id] || { label: id || "metric", unit: "", compare: "", fmt: String };
  }

  function formatBytes(n) {
    n = Number(n) || 0;
    if (n < 1024) return Math.round(n) + "B";
    if (n < 1024 * 1024) return (n / 1024).toFixed(0) + "Ki";
    return (n / (1024 * 1024)).toFixed(1) + "Mi";
  }

  function chartColors() {
    var s = getComputedStyle(document.documentElement);
    return {
      util: s.getPropertyValue("--chart-util").trim() || "#1f6b4a",
      req: s.getPropertyValue("--chart-req").trim() || "#6b746b",
      lim: s.getPropertyValue("--chart-lim").trim() || "#c45c26",
      grid: s.getPropertyValue("--chart-grid").trim() || "#e2e6e2",
      muted: s.getPropertyValue("--muted").trim() || "#5c645c",
    };
  }

  function drawGuide(ctx, left, right, y, color, label) {
    if (y < 0) return;
    ctx.strokeStyle = color;
    ctx.lineWidth = 1.5;
    ctx.setLineDash([4, 3]);
    ctx.beginPath();
    ctx.moveTo(left, y);
    ctx.lineTo(right, y);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.fillStyle = color;
    ctx.textAlign = "left";
    ctx.font = "10px ui-monospace, Menlo, monospace";
    ctx.fillText(label, left + 4, y - 3);
  }

  function drawSeries(canvas, points, metricId, refs) {
    var meta = metricInfo(metricId);
    var colors = chartColors();
    refs = refs || {};
    var ctx = canvas.getContext("2d");
    var dpr = window.devicePixelRatio || 1;
    var w = canvas.clientWidth;
    var h = canvas.clientHeight;
    canvas.width = Math.floor(w * dpr);
    canvas.height = Math.floor(h * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    var card = canvas.closest(".card");
    var stats = card && card.querySelector(".chart-stats");
    if (!points || !points.length) {
      ctx.fillStyle = colors.muted;
      ctx.font = "12px ui-monospace, Menlo, monospace";
      ctx.fillText("No samples in window", 8, h / 2);
      if (stats) stats.textContent = meta.label + " · no data";
      return;
    }
    var vals = points.map(function (p) {
      return p.v;
    });
    var min = Math.min.apply(null, vals);
    var max = Math.max.apply(null, vals);
    var last = vals[vals.length - 1];
    var req = refs.request != null && isFinite(refs.request) ? Number(refs.request) : null;
    var lim = refs.limit != null && isFinite(refs.limit) ? Number(refs.limit) : null;
    var axisMin = min;
    var axisMax = max;
    if (req != null) axisMax = Math.max(axisMax, req);
    if (lim != null) axisMax = Math.max(axisMax, lim);
    if (axisMin === axisMax) {
      axisMin -= 1;
      axisMax += 1;
    }
    // Ratio charts: pin floor at 0; include request (1.0) when present.
    if (metricId.indexOf("_ratio") !== -1) {
      axisMin = 0;
      axisMax = Math.max(axisMax * 1.05, req != null ? req : 1);
    } else {
      axisMin = Math.min(axisMin, 0);
      axisMax = axisMax * 1.05;
    }
    var left = 44;
    var padR = 8;
    var padT = 12;
    var padB = 16;
    var plotH = h - padT - padB;
    function yAt(v) {
      return padT + (1 - (v - axisMin) / (axisMax - axisMin)) * plotH;
    }
    ctx.strokeStyle = colors.grid;
    ctx.lineWidth = 1;
    [0, 0.5, 1].forEach(function (f) {
      var y = padT + (1 - f) * plotH;
      ctx.beginPath();
      ctx.moveTo(left, y);
      ctx.lineTo(w - padR, y);
      ctx.stroke();
    });
    ctx.fillStyle = colors.muted;
    ctx.font = "10px ui-monospace, Menlo, monospace";
    ctx.textAlign = "right";
    ctx.fillText(meta.fmt(axisMax), left - 4, padT + 8);
    ctx.fillText(meta.fmt(axisMin), left - 4, h - padB + 2);
    if (req != null) drawGuide(ctx, left, w - padR, yAt(req), colors.req, "req");
    if (lim != null) drawGuide(ctx, left, w - padR, yAt(lim), colors.lim, "lim");
    ctx.strokeStyle = colors.util;
    ctx.lineWidth = 2;
    ctx.beginPath();
    points.forEach(function (p, i) {
      var x = left + (i / Math.max(points.length - 1, 1)) * (w - left - padR);
      var y = yAt(p.v);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
    var lx = left + ((points.length - 1) / Math.max(points.length - 1, 1)) * (w - left - padR);
    ctx.fillStyle = colors.util;
    ctx.beginPath();
    ctx.arc(lx, yAt(last), 3, 0, Math.PI * 2);
    ctx.fill();
    canvas.setAttribute(
      "aria-label",
      meta.label +
        " last " +
        meta.fmt(last) +
        ", min " +
        meta.fmt(min) +
        ", max " +
        meta.fmt(max) +
        (req != null ? ", request " + meta.fmt(req) : "") +
        (lim != null ? ", limit " + meta.fmt(lim) : "")
    );
    if (stats) {
      var refsTxt = "";
      if (req != null) refsTxt += " · req " + meta.fmt(req);
      if (lim != null) refsTxt += " · lim " + meta.fmt(lim);
      var hint = meta.compare;
      if (req == null && lim == null) {
        hint = "no request/limit on pod (unset in cluster)";
      } else if (lim == null) {
        hint = "green=util · gray=request · no limit set";
      } else if (req == null) {
        hint = "green=util · orange=limit · no request set";
      }
      stats.innerHTML =
        '<span class="chart-metric">' +
        escapeHTML(meta.label) +
        "</span>" +
        '<span class="chart-last">last <strong>' +
        escapeHTML(meta.fmt(last)) +
        "</strong></span>" +
        '<span class="chart-minmax">min ' +
        escapeHTML(meta.fmt(min)) +
        " · max " +
        escapeHTML(meta.fmt(max)) +
        escapeHTML(refsTxt) +
        "</span>" +
        '<span class="chart-hint">' +
        escapeHTML(hint) +
        "</span>";
    }
  }

  function lookupRec(item) {
    return (
      recByGrain[item.grain_key] ||
      recByOwner[item.namespace + "/" + (item.owner_name || "")] ||
      null
    );
  }

  function fmtDeltaM(d) {
    if (!isFinite(d)) return "";
    var n = Math.round(d);
    return (n > 0 ? "+" : "") + n + "m";
  }

  function fmtDeltaMem(d) {
    if (!isFinite(d)) return "";
    var sign = d > 0 ? "+" : d < 0 ? "−" : "";
    return sign + formatBytes(Math.abs(d));
  }

  function badgeHTML(kind, verdict, deltaTxt) {
    var v = verdict || "none";
    var cls = "badge " + (v === "n/a" ? "na" : v);
    var label = kind + " " + (v === "none" ? "no recommend" : v);
    if (deltaTxt && v !== "none" && v !== "n/a") label += " " + deltaTxt;
    return '<span class="' + cls + '">' + escapeHTML(label) + "</span>";
  }

  function cardVerdictHTML(item) {
    var rec = lookupRec(item);
    if (!rec) {
      return (
        '<div class="verdict-row">' + badgeHTML("rightsizing", "none", "") + "</div>"
      );
    }
    return (
      '<div class="verdict-row">' +
      badgeHTML("CPU", rec.cpu_verdict, fmtDeltaM(rec.cpu_delta_mcores)) +
      badgeHTML("Mem", rec.mem_verdict, fmtDeltaMem(rec.mem_delta_bytes)) +
      "</div>"
    );
  }

  function updateRecSummary(items) {
    if (!el.recSummary) return;
    if (!items || !items.length) {
      el.recSummary.textContent = "";
      return;
    }
    var counts = { oversized: 0, undersized: 0, ok: 0, na: 0, none: 0 };
    items.forEach(function (item) {
      var rec = lookupRec(item);
      if (!rec) {
        counts.none++;
        return;
      }
      var worst = "ok";
      [rec.cpu_verdict, rec.mem_verdict].forEach(function (v) {
        if (v === "undersized") worst = "undersized";
        else if (v === "oversized" && worst === "ok") worst = "oversized";
        else if (v === "n/a" && worst === "ok") worst = "na";
      });
      if (worst === "na") counts.na++;
      else counts[worst]++;
    });
    el.recSummary.textContent =
      "rightsizing: " +
      counts.oversized +
      " oversized · " +
      counts.undersized +
      " undersized · " +
      counts.ok +
      " ok · " +
      counts.na +
      " no-req · " +
      counts.none +
      " no recommend";
  }

  function indexRecommendations(rows) {
    recByGrain = {};
    recByOwner = {};
    (rows || []).forEach(function (r) {
      if (r.grain_key) recByGrain[r.grain_key] = r;
      if (r.namespace && r.owner_name) {
        recByOwner[r.namespace + "/" + r.owner_name] = r;
      }
    });
  }

  function cardHTML(item) {
    var title = item.owner_name || item.grain_key;
    var id = encodeURIComponent(item.grain_key);
    var meta = metricInfo(state.metric);
    return (
      '<article class="card" data-grain="' +
      id +
      '" data-ns="' +
      escapeAttr(item.namespace) +
      '" data-wl="' +
      escapeAttr(item.owner_name || "") +
      '">' +
      "<h2>" +
      escapeHTML(title) +
      "</h2>" +
      '<div class="meta">' +
      escapeHTML(item.grain_key) +
      " · " +
      item.samples +
      " samples</div>" +
      cardVerdictHTML(item) +
      '<div class="chart-stats">' +
      '<span class="chart-metric">' +
      escapeHTML(meta.label) +
      "</span>" +
      '<span class="chart-hint">' +
      escapeHTML(meta.compare) +
      "</span></div>" +
      '<canvas aria-label="Series chart"></canvas>' +
      '<div class="actions"><button type="button" class="btn details">Details</button></div>' +
      "</article>"
    );
  }

  function escapeHTML(s) {
    return String(s)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function escapeAttr(s) {
    return escapeHTML(s).replace(/'/g, "&#39;");
  }

  function formatVersion(v) {
    v = String(v || "").trim();
    if (!v) return "?";
    if (v.charAt(0) === "v" || v === "dev") return v;
    return "v" + v;
  }

  function formatClusterTarget(st) {
    var name = (st.cluster_name || "").trim() || "unnamed-cluster";
    var env = (st.cluster_environment || "").trim();
    var ctx = (st.kube_context || "").trim() || "current-context";
    var parts = [name];
    if (env) parts.push(env);
    return parts.join(" · ") + " · ctx " + ctx;
  }

  function updateModeLabel(st) {
    if (!el.modeLabel) return;
    var live = !!st.kube_live;
    el.modeLabel.textContent = live ? "live" : "analyze-local";
    el.modeLabel.classList.toggle("live", live);
    el.modeLabel.title = live
      ? "Kube Metrics API reachable — live drawer enrichment on"
      : "SQLite store only — no live kube (charts/recommend still work)";
  }

  function setLoading() {
    el.grid.innerHTML =
      '<div class="skeleton"></div><div class="skeleton"></div><div class="skeleton"></div>';
  }

  function loadOverview() {
    setLoading();
    var wp = workloadQueryParams();
    return fetchJSON("/api/v1/status")
      .then(function (st) {
        updateModeLabel(st);
        if (el.clusterTarget) {
          el.clusterTarget.textContent = formatClusterTarget(st);
          el.clusterTarget.title = formatClusterTarget(st);
        }
        el.status.textContent =
          "store " +
          (st.store_path || "?") +
          " · samples " +
          (st.sample_count || 0) +
          " · " +
          (st.kube_live ? "kube live" : "store only") +
          " · " +
          formatVersion(st.version);
        return Promise.all([
          fetchJSON("/api/v1/workloads", wp),
          fetchJSON("/api/v1/recommendations", { grain: "workload" }).catch(function () {
            return { rows: [] };
          }),
          fetchJSON("/api/v1/nodes").catch(function () {
            return { items: [] };
          }),
        ]);
      })
      .then(function (triple) {
        var wl = triple[0];
        indexRecommendations(triple[1].rows || []);
        renderNodesPanel(triple[2]);
        if (!wl.items || !wl.items.length) {
          if (el.recSummary) el.recSummary.textContent = "";
          el.grid.innerHTML =
            '<div class="empty">' +
            (filtersActive()
              ? "No workloads match these filters."
              : "No workloads with samples in this window.") +
            "</div>";
          return;
        }
        var items = filterItems(wl.items);
        if (!items.length) {
          if (el.recSummary) el.recSummary.textContent = "";
          el.grid.innerHTML = '<div class="empty">No workloads match these filters.</div>';
          return;
        }
        updateRecSummary(items);
        el.grid.innerHTML = items.map(cardHTML).join("");
        return loadAllCardSeries(el.grid.querySelectorAll(".card"));
      })
      .catch(function (err) {
        if (err.code === 401) return;
        if (el.recSummary) el.recSummary.textContent = "";
        el.grid.innerHTML = '<div class="error">' + escapeHTML(err.message || String(err)) + "</div>";
      });
  }

  function renderNodesPanel(nodes) {
    if (!el.nodesPanel || !el.nodesBody) return;
    el.nodesPanel.hidden = false;
    var items = (nodes && nodes.items) || [];
    if (!items.length) {
      if (el.nodesMeta) el.nodesMeta.textContent = "";
      el.nodesBody.innerHTML =
        '<div class="empty">No node snapshots. Run <code>kpm capacity --record-nodes</code>.</div>';
      return;
    }
    var cpuAlloc = 0;
    var cpuReq = 0;
    var cpuUtil = 0;
    items.forEach(function (n) {
      cpuAlloc += n.cpu_alloc_mcores || 0;
      cpuReq += n.cpu_req_mcores || 0;
      cpuUtil += n.cpu_util_mcores || 0;
    });
    if (el.nodesMeta) {
      el.nodesMeta.textContent =
        items.length +
        " nodes · recorded " +
        (nodes.recorded_at || "?") +
        " · CPU alloc " +
        Math.round(cpuAlloc) +
        "m · req " +
        Math.round(cpuReq) +
        "m · util " +
        Math.round(cpuUtil) +
        "m";
    }
    var rows = items
      .map(function (n) {
        return (
          "<tr><td>" +
          escapeHTML(n.node) +
          "</td><td>" +
          Math.round(n.cpu_alloc_mcores || 0) +
          "m</td><td>" +
          Math.round(n.cpu_req_mcores || 0) +
          "m</td><td>" +
          Math.round(n.cpu_util_mcores || 0) +
          "m</td><td>" +
          formatBytes(n.mem_alloc_bytes || 0) +
          "</td><td>" +
          formatBytes(n.mem_req_bytes || 0) +
          "</td><td>" +
          (n.pods || 0) +
          "</td></tr>"
        );
      })
      .join("");
    el.nodesBody.innerHTML =
      '<table class="nodes-table"><thead><tr>' +
      "<th>Node</th><th>CPU alloc</th><th>CPU req</th><th>CPU util</th>" +
      "<th>Mem alloc</th><th>Mem req</th><th>Pods</th>" +
      "</tr></thead><tbody>" +
      rows +
      "</tbody></table>";
  }

  function loadAllCardSeries(cards) {
    if (!cards || !cards.length) return Promise.resolve();
    var keys = [];
    var byKey = {};
    Array.prototype.forEach.call(cards, function (card) {
      var ns = card.getAttribute("data-ns") || "";
      var wl = card.getAttribute("data-wl") || "";
      if (!ns || !wl) return;
      var id = ns + "/" + wl;
      if (!byKey[id]) {
        byKey[id] = [];
        keys.push(id);
      }
      byKey[id].push(card);
    });
    if (!keys.length) return Promise.resolve();
    // Matches maxSeriesBatchWorkloads. One request for every card used to scan
    // the store on the sampler's only connection.
    var batchSize = 20;
    var chunks = [];
    for (var i = 0; i < keys.length; i += batchSize) {
      chunks.push(keys.slice(i, i + batchSize));
    }
    var chain = Promise.resolve();
    chunks.forEach(function (chunk) {
      chain = chain.then(function () {
        var params = Object.assign(
          { metric: state.metric, max_points: "120", workloads: chunk.join(",") },
          windowParams()
        );
        return fetchJSON("/api/v1/series/batch", params).then(function (batch) {
          (batch.items || []).forEach(function (series) {
            var id = (series.namespace || "") + "/" + (series.workload || "");
            (byKey[id] || []).forEach(function (card) {
              drawSeries(card.querySelector("canvas"), series.points || [], series.metric || state.metric, {
                request: series.request,
                limit: series.limit,
              });
            });
          });
        });
      });
    });
    return chain.catch(function (err) {
        if (err.code === 429) {
          // Fall back to sequential single-series if the batch is limited.
          var chain = Promise.resolve();
          Array.prototype.forEach.call(cards, function (card) {
            chain = chain.then(function () {
              return loadCardSeries(card).catch(function () {});
            });
          });
          return chain;
        }
        throw err;
      });
  }

  function loadCardSeries(card) {
    var params = Object.assign(
      {
        namespace: card.getAttribute("data-ns"),
        workload: card.getAttribute("data-wl"),
        metric: state.metric,
        max_points: "120",
      },
      windowParams()
    );
    return fetchJSON("/api/v1/series", params).then(function (series) {
      drawSeries(card.querySelector("canvas"), series.points || [], series.metric || state.metric, {
        request: series.request,
        limit: series.limit,
      });
    });
  }

  function openDrawer(card) {
    var ns = card.getAttribute("data-ns");
    var wl = card.getAttribute("data-wl");
    var grain = decodeURIComponent(card.getAttribute("data-grain"));
    el.drawerTitle.textContent = wl || grain;
    el.drawerBody.innerHTML = '<div class="loading">Loading…</div>';
    el.drawer.classList.add("open");
    el.backdrop.classList.add("open");
    el.drawer.setAttribute("aria-hidden", "false");

    Promise.all([
      fetchJSON("/api/v1/recommendations", { grain: "workload" }).catch(function () {
        return { rows: [] };
      }),
      fetchJSON("/api/v1/live").catch(function (e) {
        return { available: false, error: e.message, rows: [] };
      }),
    ]).then(function (pair) {
      var recs = (pair[0].rows || []).filter(function (r) {
        return r.grain_key === grain || (r.namespace === ns && r.owner_name === wl);
      });
      var live = pair[1];
      var liveRows = (live.rows || []).filter(function (r) {
        return r.namespace === ns && (r.owner_name === wl || !wl);
      });
      var html = "";
      html += "<section><h3>Recommendations</h3>";
      if (!recs.length) html += "<p class=\"meta\">None stored for this grain.</p>";
      else {
        html += "<dl>";
        recs.forEach(function (r) {
          html +=
            "<dt>CPU</dt><dd>" +
            escapeHTML(r.cpu_verdict || "?") +
            " · sug " +
            r.cpu_sug_mcores +
            " m · req " +
            r.cpu_req_mcores +
            " m · Δ " +
            fmtDeltaM(r.cpu_delta_mcores) +
            "</dd>";
          html +=
            "<dt>Mem</dt><dd>" +
            escapeHTML(r.mem_verdict || "?") +
            " · sug " +
            formatBytes(r.mem_sug_bytes) +
            " · req " +
            formatBytes(r.mem_req_bytes) +
            " · Δ " +
            fmtDeltaMem(r.mem_delta_bytes) +
            "</dd>";
        });
        html += "</dl>";
        var patchKey = recs[0].grain_key || grain;
        html +=
          '<div class="drawer-actions">' +
          '<button type="button" class="btn primary" data-patch="' +
          escapeAttr(patchKey) +
          '" data-format="strategic">Copy strategic patch</button>' +
          '<button type="button" class="btn" data-patch="' +
          escapeAttr(patchKey) +
          '" data-format="kustomize">Download kustomize hint</button>' +
          "</div>";
        html += '<pre class="meta" id="patch-preview" hidden></pre>';
      }
      html += "</section><section><h3>Live</h3>";
      if (!live.available) {
        html += "<p class=\"meta\">" + escapeHTML(live.error || "Kube unavailable") + "</p>";
      } else if (!liveRows.length) {
        html += "<p class=\"meta\">No live rows for this workload.</p>";
      } else {
        html += "<dl>";
        liveRows.slice(0, 8).forEach(function (r) {
          html +=
            "<dt>" +
            escapeHTML(r.pod + "/" + r.container) +
            "</dt><dd>cpu " +
            r.cpu_util_mcores +
            " / req " +
            r.cpu_request_mcores +
            " m</dd>";
        });
        html += "</dl>";
      }
      html += "</section>";
      el.drawerBody.innerHTML = html;
    });
  }

  function closeDrawer() {
    el.drawer.classList.remove("open");
    el.backdrop.classList.remove("open");
    el.drawer.setAttribute("aria-hidden", "true");
  }

  function exportPatch(grainKey, format) {
    fetchJSON("/api/v1/export/patch", { grain_key: grainKey, format: format })
      .then(function (exp) {
        var preview = document.getElementById("patch-preview");
        if (preview) {
          preview.hidden = false;
          preview.textContent = exp.content;
        }
        if (format === "kustomize") {
          downloadText(exp.filename || "kpm-patch.md", exp.content);
          return;
        }
        if (navigator.clipboard && navigator.clipboard.writeText) {
          return navigator.clipboard.writeText(exp.content).catch(function () {
            downloadText(exp.filename || "patch.yaml", exp.content);
          });
        }
        downloadText(exp.filename || "patch.yaml", exp.content);
      })
      .catch(function (err) {
        var preview = document.getElementById("patch-preview");
        if (preview) {
          preview.hidden = false;
          preview.textContent = err.message || String(err);
        }
      });
  }

  function downloadText(filename, text) {
    var blob = new Blob([text], { type: "text/plain;charset=utf-8" });
    var url = URL.createObjectURL(blob);
    var a = document.createElement("a");
    a.href = url;
    a.download = filename;
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  }

  function currentTheme() {
    return document.documentElement.getAttribute("data-theme") === "dark" ? "dark" : "light";
  }

  function applyTheme(theme) {
    theme = theme === "dark" ? "dark" : "light";
    document.documentElement.setAttribute("data-theme", theme);
    try {
      localStorage.setItem(THEME_KEY, theme);
    } catch (e) {}
    var btn = document.getElementById("theme-toggle");
    if (btn) {
      var dark = theme === "dark";
      btn.textContent = dark ? "Light" : "Dark";
      btn.setAttribute("aria-pressed", dark ? "true" : "false");
      btn.setAttribute("aria-label", dark ? "Switch to light mode" : "Switch to dark mode");
    }
  }

  function bindTheme() {
    applyTheme(currentTheme());
    var btn = document.getElementById("theme-toggle");
    if (!btn) return;
    btn.addEventListener("click", function () {
      applyTheme(currentTheme() === "dark" ? "light" : "dark");
      if (!el.app.classList.contains("hidden")) loadOverview();
    });
  }

  function readFiltersFromDOM() {
    state.filters.namespace = (document.getElementById("filter-ns").value || "").trim();
    state.filters.owner = (document.getElementById("filter-owner").value || "").trim();
    state.filters.grain = (document.getElementById("filter-grain").value || "").trim();
    state.filters.cpu = document.getElementById("filter-cpu").value || "";
    state.filters.mem = document.getElementById("filter-mem").value || "";
  }

  function scheduleFilterReload() {
    if (filterTimer) clearTimeout(filterTimer);
    filterTimer = setTimeout(function () {
      filterTimer = null;
      readFiltersFromDOM();
      loadOverview();
    }, 200);
  }

  function bindFilters() {
    ["filter-ns", "filter-owner", "filter-grain"].forEach(function (id) {
      var node = document.getElementById(id);
      if (!node) return;
      node.addEventListener("input", scheduleFilterReload);
    });
    ["filter-cpu", "filter-mem"].forEach(function (id) {
      var node = document.getElementById(id);
      if (!node) return;
      node.addEventListener("change", function () {
        readFiltersFromDOM();
        loadOverview();
      });
    });
    var clear = document.getElementById("filter-clear");
    if (clear) {
      clear.addEventListener("click", function () {
        ["filter-ns", "filter-owner", "filter-grain"].forEach(function (id) {
          document.getElementById(id).value = "";
        });
        document.getElementById("filter-cpu").value = "";
        document.getElementById("filter-mem").value = "";
        state.filters = { namespace: "", owner: "", grain: "", cpu: "", mem: "" };
        loadOverview();
      });
    }
  }

  function bindWindows() {
    document.querySelectorAll("[data-window]").forEach(function (btn) {
      btn.addEventListener("click", function () {
        state.window = btn.getAttribute("data-window");
        state.since = "";
        state.until = "";
        document.querySelectorAll("[data-window]").forEach(function (b) {
          b.classList.toggle("active", b === btn);
        });
        loadOverview();
      });
    });
    document.getElementById("apply-custom").addEventListener("click", function () {
      var since = document.getElementById("since").value;
      var until = document.getElementById("until").value;
      if (!since || !until) return;
      state.since = new Date(since).toISOString();
      state.until = new Date(until).toISOString();
      document.querySelectorAll("[data-window]").forEach(function (b) {
        b.classList.remove("active");
      });
      loadOverview();
    });
    document.getElementById("metric").addEventListener("change", function (e) {
      state.metric = e.target.value;
      loadOverview();
    });
  }

  document.getElementById("auth-form").addEventListener("submit", function (e) {
    e.preventDefault();
    state.token = (el.tokenInput.value || "").trim();
    sessionStorage.setItem(TOKEN_KEY, state.token);
    el.authErr.textContent = "";
    showApp();
    loadOverview();
  });

  el.grid.addEventListener("click", function (e) {
    var btn = e.target.closest(".details");
    if (!btn) return;
    openDrawer(btn.closest(".card"));
  });
  el.drawerBody.addEventListener("click", function (e) {
    var btn = e.target.closest("[data-patch]");
    if (!btn) return;
    exportPatch(btn.getAttribute("data-patch"), btn.getAttribute("data-format") || "strategic");
  });
  document.getElementById("drawer-close").addEventListener("click", closeDrawer);
  el.backdrop.addEventListener("click", closeDrawer);
  document.addEventListener("keydown", function (e) {
    if (e.key === "Escape") closeDrawer();
  });

  bindTheme();
  bindWindows();
  bindFilters();
  // Probe status; 401 → auth form.
  fetchJSON("/api/v1/status")
    .then(function () {
      showApp();
      loadOverview();
    })
    .catch(function (err) {
      if (err.code !== 401) {
        showApp();
        el.grid.innerHTML = '<div class="error">' + escapeHTML(err.message || String(err)) + "</div>";
      }
    });
})();
