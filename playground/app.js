(() => {
  const sourceEl = document.getElementById("source");
  const diagEl = document.getElementById("diagnostics");
  const goEl = document.getElementById("go-out");
  const goHintEl = document.getElementById("go-hint");
  const statusEl = document.getElementById("status");
  const examplesEl = document.getElementById("examples");
  const btnCheck = document.getElementById("btn-check");
  const btnRun = document.getElementById("btn-run");
  const btnCopy = document.getElementById("btn-copy-go");

  let ready = false;
  let lastGo = "";

  function setStatus(text, kind) {
    statusEl.textContent = text;
    statusEl.className = "status" + (kind ? " " + kind : "");
  }

  function setBusy(busy) {
    btnCheck.disabled = busy || !ready;
    btnRun.disabled = busy || !ready;
    if (busy) setStatus("working…", "busy");
  }

  function setGoOutput(text, isPlaceholder) {
    lastGo = isPlaceholder ? "" : text || "";
    goEl.textContent = isPlaceholder ? "" : text || "";
    goHintEl.hidden = !isPlaceholder && !!text;
    if (isPlaceholder) {
      goHintEl.textContent = text || "Compile to see generated Go";
      goHintEl.hidden = false;
    }
    btnCopy.disabled = !lastGo;
  }

  function formatDiag(d) {
    const loc =
      d.line > 0
        ? `${d.file || "playground.goop"}:${d.line}:${d.column || 1}: `
        : "";
    const sev = (d.severity || "error").toUpperCase();
    return { text: `${sev} ${loc}${d.message}`, severity: d.severity || "error", line: d.line || 0, column: d.column || 1 };
  }

  function jumpToLine(line, column) {
    if (!line || line < 1) return;
    const lines = sourceEl.value.split("\n");
    let offset = 0;
    for (let i = 0; i < line - 1 && i < lines.length; i++) {
      offset += lines[i].length + 1;
    }
    const col = Math.max(1, column || 1) - 1;
    const start = offset + Math.min(col, (lines[line - 1] || "").length);
    sourceEl.focus();
    sourceEl.setSelectionRange(start, start);
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
      const { text, severity, line, column } = formatDiag(d);
      const row = document.createElement("div");
      row.className = severity === "warning" ? "diag-warning" : "diag-error";
      if (line > 0) {
        row.classList.add("diag-clickable");
        row.title = "Jump to source";
        row.addEventListener("click", () => jumpToLine(line, column));
      }
      row.textContent = text;
      diagEl.appendChild(row);
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
    setGoOutput("", true);
    goHintEl.textContent = "working…";
    // Yield so the UI can paint "working…" before the sync WASM call.
    setTimeout(() => {
      try {
        const fn = mode === "check" ? globalThis.goopCheck : globalThis.goopCompile;
        const res = parseResult(fn(src));
        if (res.error && (!res.diagnostics || res.diagnostics.length === 0)) {
          renderDiagnostics([{ severity: "error", message: res.error }]);
          setStatus("error", "err");
          setGoOutput("Compile to see generated Go", true);
          return;
        }
        renderDiagnostics(res.diagnostics || []);
        if (mode === "compile") {
          if (res.ok && res.go) {
            setGoOutput(res.go, false);
          } else {
            setGoOutput(res.ok ? "Compile to see generated Go" : "Compile failed — see diagnostics", true);
          }
        } else {
          setGoOutput(res.ok ? "Compile to see generated Go" : "Check failed — see diagnostics", true);
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
        setGoOutput("Compile to see generated Go", true);
      } finally {
        setBusy(false);
      }
    }, 0);
  }

  async function copyGo() {
    if (!lastGo) return;
    try {
      await navigator.clipboard.writeText(lastGo);
      setStatus("Copied", "ok");
      setTimeout(() => {
        if (statusEl.textContent === "Copied") setStatus("ready", "ok");
      }, 1200);
    } catch (e) {
      setStatus("copy failed", "err");
    }
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
      setGoOutput("Compile to see generated Go", true);
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

  document.addEventListener("keydown", (e) => {
    if (!(e.ctrlKey || e.metaKey) || e.key !== "Enter") return;
    e.preventDefault();
    if (e.shiftKey) run("check");
    else run("compile");
  });

  btnCheck.addEventListener("click", () => run("check"));
  btnRun.addEventListener("click", () => run("compile"));
  btnCopy.addEventListener("click", () => copyGo());

  async function boot() {
    loadExamples();
    setGoOutput("Compile to see generated Go", true);
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
