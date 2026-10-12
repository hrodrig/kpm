/* kpm analyze-local charts — fetch API + canvas series */
(function () {
  "use strict";

  var TOKEN_KEY = "kpm_api_token";
  var state = {
    window: "24h",
    metric: "cpu_util_mcores",
    since: "",
    until: "",
    token: sessionStorage.getItem(TOKEN_KEY) || "",
  };

  var el = {
    auth: document.getElementById("auth"),
    app: document.getElementById("app"),
    grid: document.getElementById("grid"),
    status: document.getElementById("status"),
    tokenInput: document.getElementById("token"),
    authErr: document.getElementById("auth-err"),
    drawer: document.getElementById("drawer"),
    backdrop: document.getElementById("drawer-backdrop"),
    drawerTitle: document.getElementById("drawer-title"),
    drawerBody: document.getElementById("drawer-body"),
  };

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
      return r.json().then(function (body) {
        if (!r.ok) {
          var e = new Error((body && body.error) || r.statusText);
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

  function drawSeries(canvas, points) {
    var ctx = canvas.getContext("2d");
    var dpr = window.devicePixelRatio || 1;
    var w = canvas.clientWidth;
    var h = canvas.clientHeight;
    canvas.width = Math.floor(w * dpr);
    canvas.height = Math.floor(h * dpr);
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    if (!points || !points.length) {
      ctx.fillStyle = "#5c645c";
      ctx.font = "12px ui-monospace, monospace";
      ctx.fillText("No samples in window", 8, h / 2);
      return;
    }
    var vals = points.map(function (p) {
      return p.v;
    });
    var min = Math.min.apply(null, vals);
    var max = Math.max.apply(null, vals);
    if (min === max) {
      min -= 1;
      max += 1;
    }
    var pad = 8;
    ctx.strokeStyle = "#c8cec8";
    ctx.lineWidth = 1;
    ctx.beginPath();
    ctx.moveTo(pad, h - pad);
    ctx.lineTo(w - pad, h - pad);
    ctx.stroke();
    ctx.strokeStyle = "#1f6b4a";
    ctx.lineWidth = 2;
    ctx.beginPath();
    points.forEach(function (p, i) {
      var x = pad + (i / Math.max(points.length - 1, 1)) * (w - pad * 2);
      var y = h - pad - ((p.v - min) / (max - min)) * (h - pad * 2);
      if (i === 0) ctx.moveTo(x, y);
      else ctx.lineTo(x, y);
    });
    ctx.stroke();
  }

  function cardHTML(item) {
    var title = item.owner_name || item.grain_key;
    var id = encodeURIComponent(item.grain_key);
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

  function setLoading() {
    el.grid.innerHTML =
      '<div class="skeleton"></div><div class="skeleton"></div><div class="skeleton"></div>';
  }

  function loadOverview() {
    setLoading();
    var wp = windowParams();
    return fetchJSON("/api/v1/status")
      .then(function (st) {
        el.status.textContent =
          "store " +
          (st.store_path || "?") +
          " · samples " +
          (st.sample_count || 0) +
          " · v" +
          (st.version || "");
        return fetchJSON("/api/v1/workloads", wp);
      })
      .then(function (wl) {
        if (!wl.items || !wl.items.length) {
          el.grid.innerHTML = '<div class="empty">No workloads with samples in this window.</div>';
          return;
        }
        el.grid.innerHTML = wl.items.map(cardHTML).join("");
        var cards = el.grid.querySelectorAll(".card");
        var chain = Promise.resolve();
        cards.forEach(function (card) {
          chain = chain.then(function () {
            return loadCardSeries(card);
          });
        });
        return chain;
      })
      .catch(function (err) {
        if (err.code === 401) return;
        el.grid.innerHTML = '<div class="error">' + escapeHTML(err.message || String(err)) + "</div>";
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
      drawSeries(card.querySelector("canvas"), series.points || []);
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
            "<dt>CPU suggest</dt><dd>" +
            r.cpu_sug_mcores +
            " m · req " +
            r.cpu_req_mcores +
            " m</dd>";
          html +=
            "<dt>Mem suggest</dt><dd>" +
            r.mem_sug_bytes +
            " B · req " +
            r.mem_req_bytes +
            " B</dd>";
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
          return navigator.clipboard.writeText(exp.content);
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

  bindWindows();
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
