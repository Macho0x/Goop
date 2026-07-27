(() => {
  const sourceEl = document.getElementById("source");
  const diagEl = document.getElementById("diagnostics");
  const goEl = document.getElementById("go-out");
  const statusEl = document.getElementById("status");
  const examplesEl = document.getElementById("examples");
  const btnCheck = document.getElementById("btn-check");
  const btnRun = document.getElementById("btn-run");

  let ready = false;

  function setStatus(text, kind) {
    statusEl.textContent = text;
    statusEl.className = "status" + (kind ? " " + kind : "");
  }

  function setBusy(busy) {
    btnCheck.disabled = busy || !ready;
    btnRun.disabled = busy || !ready;
    if (busy) setStatus("working…", "busy");
  }

  function formatDiag(d) {
    const loc =
      d.line > 0
        ? `${d.file || "playground.goop"}:${d.line}:${d.column || 1}: `
        : "";
    const sev = (d.severity || "error").toUpperCase();
    return { text: `${sev} ${loc}${d.message}`, severity: d.severity || "error" };
  }

  function renderDiagnostics(diags) {
    diagEl.textContent = "";
    if (!diags || diags.length === 0) {
      const empty = document.createElement("span");
      empty.className = "diag-empty";
      empty.textContent = "No diagnostics.";
      diagEl.appendChild(empty);
      return;
    }
    diags.forEach((d, i) => {
      const { text, severity } = formatDiag(d);
      const line = document.createElement("div");
      line.className = severity === "warning" ? "diag-warning" : "diag-error";
      line.textContent = text;
      diagEl.appendChild(line);
      if (i < diags.length - 1) diagEl.appendChild(document.createTextNode("\n"));
    });
  }

  function parseResult(raw) {
    if (typeof raw !== "string") {
      return { ok: false, diagnostics: [], error: "unexpected non-string result from WASM" };
    }
    try {
      return JSON.parse(raw);
    } catch (e) {
      return { ok: false, diagnostics: [], error: "invalid JSON from WASM: " + e.message };
    }
  }

  function run(mode) {
    if (!ready) return;
    const src = sourceEl.value;
    setBusy(true);
    goEl.textContent = "";
    // Yield so the UI can paint "working…" before the sync WASM call.
    setTimeout(() => {
      try {
        const fn = mode === "check" ? globalThis.goopCheck : globalThis.goopCompile;
        const res = parseResult(fn(src));
        if (res.error && (!res.diagnostics || res.diagnostics.length === 0)) {
          renderDiagnostics([{ severity: "error", message: res.error }]);
          setStatus("error", "err");
          return;
        }
        renderDiagnostics(res.diagnostics || []);
        if (mode === "compile") {
          goEl.textContent = res.go || (res.ok ? "" : "(no Go generated)");
        } else {
          goEl.textContent = res.ok ? "(check only — press Compile for Go)" : "(check failed)";
        }
        if (res.ok) {
          const warns = (res.diagnostics || []).filter((d) => d.severity === "warning");
          setStatus(warns.length ? `ok (${warns.length} warning${warns.length === 1 ? "" : "s"})` : "ok", warns.length ? "warn" : "ok");
        } else {
          setStatus("failed", "err");
        }
      } catch (e) {
        renderDiagnostics([{ severity: "error", message: String(e) }]);
        setStatus("error", "err");
      } finally {
        setBusy(false);
      }
    }, 0);
  }

  function loadExamples() {
    const examples = window.GOOP_EXAMPLES || [];
    examplesEl.innerHTML = "";
    examples.forEach((ex, i) => {
      const opt = document.createElement("option");
      opt.value = String(i);
      opt.textContent = ex.title;
      examplesEl.appendChild(opt);
    });
    if (examples.length) {
      sourceEl.value = examples[0].source;
    }
    examplesEl.addEventListener("change", () => {
      const ex = examples[Number(examplesEl.value)];
      if (ex) sourceEl.value = ex.source;
    });
  }

  // Tab inserts two spaces in the textarea.
  sourceEl.addEventListener("keydown", (e) => {
    if (e.key !== "Tab") return;
    e.preventDefault();
    const start = sourceEl.selectionStart;
    const end = sourceEl.selectionEnd;
    const v = sourceEl.value;
    sourceEl.value = v.slice(0, start) + "  " + v.slice(end);
    sourceEl.selectionStart = sourceEl.selectionEnd = start + 2;
  });

  btnCheck.addEventListener("click", () => run("check"));
  btnRun.addEventListener("click", () => run("compile"));

  async function boot() {
    loadExamples();
    setBusy(true);
    setStatus("loading wasm…", "busy");
    try {
      const go = new Go();
      const resp = await fetch("goop.wasm");
      if (!resp.ok) {
        throw new Error(`failed to fetch goop.wasm (${resp.status}) — run playground/build.sh first`);
      }
      const result = await WebAssembly.instantiateStreaming(resp, go.importObject);
      go.run(result.instance);
      ready = typeof globalThis.goopCheck === "function" && typeof globalThis.goopCompile === "function";
      if (!ready) throw new Error("WASM loaded but goopCheck/goopCompile were not registered");
      setStatus("ready", "ok");
      setBusy(false);
    } catch (e) {
      setStatus("load failed", "err");
      renderDiagnostics([{ severity: "error", message: String(e) }]);
      btnCheck.disabled = true;
      btnRun.disabled = true;
    }
  }

  boot();
})();
