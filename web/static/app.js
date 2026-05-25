/* ─── Ephemeral Link — app.js ────────────────────────────────────────────── */
"use strict";

document.addEventListener("DOMContentLoaded", () => {
  /* ── Tabs ─────────────────────────────────────────────────────────────── */
  const tabBtns = document.querySelectorAll(".tab-btn[data-tab]");
  const typeInput = document.getElementById("secret-type-input");

  tabBtns.forEach((btn) => {
    btn.addEventListener("click", () => {
      const target = btn.dataset.tab;

      tabBtns.forEach((b) => {
        b.classList.remove("active");
        b.setAttribute("aria-selected", "false");
      });
      document.querySelectorAll(".tab-panel").forEach((p) => {
        p.classList.remove("active");
        p.hidden = true;
      });

      btn.classList.add("active");
      btn.setAttribute("aria-selected", "true");

      const panel = document.getElementById("panel-" + target);
      if (panel) {
        panel.hidden = false;
        panel.classList.add("active");
      }
      if (typeInput) typeInput.value = target;
    });
  });

  /* ── Language menu ───────────────────────────────────────────────────── */
  document.querySelectorAll("[data-lang-toggle]").forEach((toggle) => {
    const form = toggle.closest(".lang");
    const menu = form?.querySelector(".lang-menu");
    if (!form || !menu) return;

    const setOpen = (open) => {
      form.classList.toggle("open", open);
      menu.hidden = !open;
      toggle.setAttribute("aria-expanded", open ? "true" : "false");
    };

    toggle.addEventListener("click", (event) => {
      event.stopPropagation();
      setOpen(menu.hidden);
    });

    document.addEventListener("click", (event) => {
      if (!form.contains(event.target)) setOpen(false);
    });

    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape") {
        setOpen(false);
        toggle.focus();
      }
    });
  });

  /* ── Admin section menu ──────────────────────────────────────────────── */
  const adminMenuButtons = document.querySelectorAll("[data-admin-tab]");
  const adminPanels = document.querySelectorAll("[data-admin-panel]");

  function showAdminPanel(target, updateHash = true) {
    if (!target) return;
    let matched = false;
    adminPanels.forEach((panel) => {
      const isActive = panel.dataset.adminPanel === target;
      matched = matched || isActive;
      panel.hidden = !isActive;
      panel.classList.toggle("active", isActive);
    });
    if (!matched) return;
    adminMenuButtons.forEach((button) => {
      button.classList.toggle("active", button.dataset.adminTab === target);
    });
    if (updateHash) window.history.replaceState(null, "", `#${target}`);
  }

  adminMenuButtons.forEach((btn) => {
    btn.addEventListener("click", () => showAdminPanel(btn.dataset.adminTab));
  });

  if (adminPanels.length) {
    showAdminPanel(window.location.hash.replace("#", "") || "analytics", false);
  }

  document.querySelectorAll("[data-user-edit]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const dialog = document.getElementById(btn.dataset.userEdit || "");
      if (!dialog) return;
      if (typeof dialog.showModal === "function") dialog.showModal();
      else dialog.setAttribute("open", "");
    });
  });

  document.querySelectorAll("[data-dialog-close]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const dialog = btn.closest("dialog");
      if (dialog?.close) dialog.close();
      else dialog?.removeAttribute("open");
    });
  });

  const usersSearch = document.getElementById("admin-users-search");
  const usersTable = document.getElementById("admin-users-table");
  if (usersSearch && usersTable) {
    const userRows = Array.from(usersTable.querySelectorAll("tbody tr"));
    usersSearch.addEventListener("input", () => {
      const query = usersSearch.value.trim().toLowerCase();
      userRows.forEach((row) => {
        row.hidden =
          query !== "" && !row.textContent.toLowerCase().includes(query);
      });
    });
  }

  /* ── Character counter ────────────────────────────────────────────────── */
  const textarea = document.getElementById("secret-text");
  const counter = document.getElementById("char-counter");

  if (textarea && counter) {
    const max = parseInt(textarea.getAttribute("maxlength") || "0", 10);
    const update = () => {
      const len = textarea.value.length;
      counter.textContent =
        len.toLocaleString() + (max ? " / " + max.toLocaleString() : "");
      counter.className = "char-counter";
      if (max) {
        const ratio = len / max;
        if (ratio > 0.95) counter.classList.add("danger");
        else if (ratio > 0.8) counter.classList.add("warning");
      }
    };
    textarea.addEventListener("input", update);
    update();
  }

  /* ── Passphrase toggle ────────────────────────────────────────────────── */
  const ppToggle = document.getElementById("passphrase-toggle");
  const ppWrap = document.getElementById("passphrase-field-wrap");
  const ppInput = document.getElementById("passphrase");

  if (ppToggle && ppWrap) {
    ppToggle.addEventListener("change", () => {
      const show = ppToggle.checked;
      ppWrap.style.display = show ? "block" : "none";
      if (show && ppInput) ppInput.focus();
      else if (!show && ppInput) ppInput.value = "";
    });
  }

  /* ── Email link toggle ───────────────────────────────────────────────── */
  document.querySelectorAll("[data-email-toggle]").forEach((toggle) => {
    const field = document.getElementById(toggle.dataset.emailTarget || "");
    const input = field?.querySelector('input[name="recipient_email"]');
    const delivery = toggle
      .closest("form")
      ?.querySelector("[data-email-delivery]");

    const update = () => {
      const enabled = toggle.checked;
      if (field) field.hidden = !enabled;
      if (input) {
        input.required = enabled;
        if (!enabled) input.value = "";
        else input.focus();
      }
      if (delivery) delivery.value = enabled ? "email" : "link";
    };

    toggle.addEventListener("change", update);
    update();
  });

  /* ── Passphrase visibility reveal ────────────────────────────────────── */
  document.querySelectorAll("#pp-reveal").forEach((btn) => {
    btn.addEventListener("click", () => {
      const field = btn.closest(".passphrase-field")?.querySelector("input");
      if (!field) return;
      const isText = field.type === "text";
      field.type = isText ? "password" : "text";
      const icon = btn.querySelector("svg");
      if (icon) {
        icon.innerHTML = isText
          ? '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>'
          : '<path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19m-6.72-1.07a3 3 0 11-4.24-4.24"/><line x1="1" y1="1" x2="23" y2="23"/>';
      }
    });
  });

  /* ── Passphrase generation ────────────────────────────────────────────── */
  const passphraseChars =
    "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz23456789!@#$%^&*-_=+?";

  function generatePassphrase(length = 24) {
    if (!window.crypto?.getRandomValues) return "";
    const chars = passphraseChars;
    const maxByte = 256 - (256 % chars.length);
    let result = "";

    while (result.length < length) {
      const bytes = new Uint8Array(length - result.length);
      window.crypto.getRandomValues(bytes);
      bytes.forEach((byte) => {
        if (result.length < length && byte < maxByte) {
          result += chars[byte % chars.length];
        }
      });
    }

    return result;
  }

  function updatePassphraseActions(field) {
    const hasPassphrase = field.value.length > 0;
    const wrap = field.closest(".passphrase-input-wrap");
    wrap
      ?.querySelectorAll("[data-reveal-passphrase], [data-copy-passphrase]")
      .forEach((btn) => {
        btn.hidden = !hasPassphrase;
      });
    if (!hasPassphrase) field.type = "password";
  }

  async function copyText(text) {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }

    const textarea = document.createElement("textarea");
    textarea.value = text;
    textarea.setAttribute("readonly", "");
    textarea.style.position = "fixed";
    textarea.style.left = "-9999px";
    document.body.appendChild(textarea);
    textarea.select();
    const copied = document.execCommand("copy");
    textarea.remove();
    return copied;
  }

  document.querySelectorAll(".passphrase-input-wrap input").forEach((field) => {
    field.addEventListener("input", () => updatePassphraseActions(field));
    updatePassphraseActions(field);
  });

  document.querySelectorAll("[data-generate-passphrase]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const field = document.getElementById(
        btn.dataset.generatePassphrase || "",
      );
      if (!field) return;
      const passphrase = generatePassphrase();
      if (!passphrase) return;
      field.value = passphrase;
      field.dispatchEvent(new Event("input", { bubbles: true }));
      field.dispatchEvent(new Event("change", { bubbles: true }));
      field.focus();
    });
  });

  document.querySelectorAll("[data-reveal-passphrase]").forEach((btn) => {
    btn.addEventListener("click", () => {
      const field = document.getElementById(btn.dataset.revealPassphrase || "");
      if (!field || field.value.length === 0) return;
      field.type = field.type === "password" ? "text" : "password";
      field.focus();
    });
  });

  document.querySelectorAll("[data-copy-passphrase]").forEach((btn) => {
    btn.addEventListener("click", async () => {
      const field = document.getElementById(btn.dataset.copyPassphrase || "");
      if (!field || field.value.length === 0) return;
      try {
        await copyText(field.value);
        field.focus();
        field.select();
      } catch (_) {
        field.focus();
        field.select();
      }
    });
  });

  /* ── File drop area ───────────────────────────────────────────────────── */
  const fileDrop = document.getElementById("file-drop");
  const fileInput =
    document.getElementById("home-file-input") ||
    document.getElementById("file-input");
  const fileSelected = document.getElementById("file-selected");
  const fileName =
    document.getElementById("home-file-name") ||
    document.getElementById("file-name");
  const fileSize = document.getElementById("file-size");
  const fileClear = document.getElementById("file-clear");

  function formatBytes(bytes) {
    if (bytes < 1024) return bytes + " B";
    if (bytes < 1048576) return (bytes / 1024).toFixed(1) + " KB";
    return (bytes / 1048576).toFixed(1) + " MB";
  }

  function showFile(file) {
    if (!fileName) return;
    fileName.textContent = file.name;
    if (fileSize) fileSize.textContent = formatBytes(file.size);
    if (fileSelected) fileSelected.style.display = "flex";
    if (fileDrop) fileDrop.style.display = "none";
  }

  if (fileInput) {
    fileInput.addEventListener("change", () => {
      if (fileInput.files[0]) showFile(fileInput.files[0]);
    });
  }

  if (fileClear) {
    fileClear.addEventListener("click", () => {
      if (fileInput) fileInput.value = "";
      if (fileSelected) fileSelected.style.display = "none";
      if (fileDrop) fileDrop.style.display = "block";
    });
  }

  if (fileDrop) {
    ["dragenter", "dragover"].forEach((evt) => {
      fileDrop.addEventListener(evt, (e) => {
        e.preventDefault();
        fileDrop.classList.add("dragover");
      });
    });
    ["dragleave", "drop"].forEach((evt) => {
      fileDrop.addEventListener(evt, (e) => {
        e.preventDefault();
        fileDrop.classList.remove("dragover");
      });
    });
    fileDrop.addEventListener("drop", (e) => {
      const file = e.dataTransfer?.files?.[0];
      if (!file || !fileInput) return;
      const dt = new DataTransfer();
      dt.items.add(file);
      fileInput.files = dt.files;
      showFile(file);
    });
  }

  /* ── Copy buttons ─────────────────────────────────────────────────────── */
  document
    .querySelectorAll('[id$="copy-btn"], [id$="copy-secret-btn"], [data-copy]')
    .forEach((btn) => {
      btn.addEventListener("click", () => {
        const copyTarget = btn.dataset.copy
          ? document.querySelector(btn.dataset.copy)
          : null;
        const source = copyTarget
          ? copyTarget.value || copyTarget.textContent || ""
          : btn.dataset.value || "";
        copyText(source, btn);
      });
    });

  async function copyText(text, btn) {
    const value = String(text || "");
    let copied = false;
    if (navigator.clipboard?.writeText) {
      try {
        await navigator.clipboard.writeText(value);
        copied = true;
      } catch {
        copied = false;
      }
    }
    if (!copied) {
      const ta = document.createElement("textarea");
      ta.value = value;
      ta.setAttribute("readonly", "");
      ta.style.cssText = "position:fixed;opacity:0;top:-9999px;left:-9999px";
      document.body.appendChild(ta);
      ta.focus();
      ta.select();
      try {
        copied = document.execCommand("copy");
      } finally {
        document.body.removeChild(ta);
      }
    }
    if (!btn) return;
    const orig = btn.innerHTML;
    btn.classList.add(copied ? "copied" : "copy-failed");
    btn.textContent = copied ? "[ COPIED ]" : "[ COPY FAILED ]";
    setTimeout(() => {
      btn.classList.remove("copied", "copy-failed");
      btn.innerHTML = orig;
    }, 1800);
  }

  /* ── Form submit — progress bar ───────────────────────────────────────── */
  const form = document.getElementById("create-form");
  const progress = document.getElementById("progress");
  const submitBtn = document.getElementById("submit-btn");

  if (form && progress) {
    form.addEventListener("submit", () => {
      if (submitBtn) submitBtn.disabled = true;
      progress.style.display = "block";
      let w = 0;
      const iv = setInterval(() => {
        w = Math.min(w + Math.random() * 15, 85);
        progress.style.width = w + "%";
      }, 200);
      form._progressInterval = iv;
    });
  }

  /* ── Session receipts/history ────────────────────────────────────────── */
  const historyKey = "ephemeral-link.receipts";

  function getReceipts() {
    try {
      return JSON.parse(sessionStorage.getItem(historyKey) || "[]");
    } catch {
      return [];
    }
  }

  function saveReceipts(receipts) {
    sessionStorage.setItem(historyKey, JSON.stringify(receipts.slice(0, 25)));
  }

  function shortCode(link) {
    const tail =
      String(link || "")
        .split("/")
        .filter(Boolean)
        .pop() || "LINK";
    return tail.slice(0, 4).toUpperCase();
  }

  function escapeHTML(value) {
    return String(value || "").replace(
      /[&<>"]/g,
      (char) =>
        ({
          "&": "&amp;",
          "<": "&lt;",
          ">": "&gt;",
          '"': "&quot;",
        })[char],
    );
  }

  function detailURL(receipt) {
    const params = new URLSearchParams({ link: receipt.link });
    if (receipt.expires) params.set("expires", receipt.expires);
    if (receipt.ttl) params.set("ttl", receipt.ttl);
    return "/created?" + params.toString();
  }

  function addCurrentCreatedReceipt() {
    const body = document.getElementById("created-body");
    const link = body?.dataset.createdLink;
    if (!link) return;
    const existing = getReceipts().find((receipt) => receipt.link === link);
    const receipts = getReceipts().filter((receipt) => receipt.link !== link);
    receipts.unshift({
      link,
      direction: link.includes("/upload/") ? "receive" : "send",
      expires: body.dataset.createdExpires || existing?.expires || "",
      ttl: body.dataset.createdTtl || existing?.ttl || "",
      createdAt: existing?.createdAt || Date.now(),
      code: shortCode(link),
    });
    saveReceipts(receipts);
  }

  function uploadRequestID(link) {
    try {
      const url = new URL(link, window.location.origin);
      const parts = url.pathname.split("/").filter(Boolean);
      return parts[0] === "upload" && parts[1] ? parts[1] : "";
    } catch {
      return "";
    }
  }

  function hydrateReceiveButtons(root, getFileLabel) {
    root.querySelectorAll("[data-receive-status]").forEach(async (btn) => {
      const statusURL = btn.dataset.receiveStatus || "";
      if (
        !statusURL ||
        (statusURL.endsWith("/status") && statusURL.includes("//status"))
      )
        return;
      try {
        const res = await fetch(statusURL, {
          headers: { Accept: "application/json" },
        });
        if (!res.ok) return;
        const status = await res.json();
        if (!status.uploaded || !status.download_url) return;
        btn.disabled = false;
        btn.textContent = `[ ${getFileLabel} ]`;
        btn.dataset.historyDownload = status.download_url;
      } catch {
        return;
      }
    });
  }

  function renderHistory() {
    const section = document.getElementById("home-history-section");
    const list = document.getElementById("home-history-list");
    const count = document.getElementById("home-history-count");
    const itemLabel = section?.dataset.itemLabel || "Item";
    const itemsLabel = section?.dataset.itemsLabel || "Items";
    const sendLabel = section?.dataset.sendLabel || "Send";
    const receiveLabel = section?.dataset.receiveLabel || "Receive";
    const getFileLabel = section?.dataset.getFileLabel || "Get file";
    const waitingFileLabel =
      section?.dataset.waitingFileLabel || "Waiting for file";
    if (!list) return;
    const receipts = getReceipts();
    if (count)
      count.textContent =
        receipts.length +
        " " +
        (receipts.length === 1 ? itemLabel : itemsLabel);
    if (!receipts.length) {
      list.innerHTML = "";
      if (section) section.hidden = true;
      return;
    }
    if (section) section.hidden = false;
    list.innerHTML = receipts
      .map((receipt, index) => {
        const code = shortCode(receipt.link);
        const age = receipt.createdAt
          ? Math.max(1, Math.round((Date.now() - receipt.createdAt) / 60000))
          : 1;
        const expires = escapeHTML(
          receipt.expires || "expires based on selection",
        );
        const rawLink = String(receipt.link || "");
        const direction =
          receipt.direction ||
          (rawLink.includes("/upload/") ? "receive" : "send");
        const directionLabel =
          direction === "receive" ? receiveLabel : sendLabel;
        const receiveID =
          direction === "receive" ? uploadRequestID(rawLink) : "";
        const link = escapeHTML(receipt.link);
        const detail = escapeHTML(detailURL(receipt));
        const secondaryAction =
          direction === "receive"
            ? `<button id="history-item-${index}-download" type="button" class="history-open" data-receive-status="/upload-requests/${escapeHTML(receiveID)}/status" disabled>[ ${escapeHTML(waitingFileLabel)} ]</button>`
            : `<button id="history-item-${index}-open" type="button" class="history-open" data-history-open="${link}">[ OPEN ↗ ]</button>`;
        return `<article id="history-item-${index}" class="history-item" data-detail="${detail}" data-link="${link}">
          <div id="history-item-${index}-meta" class="history-item-meta">
            <span id="history-item-${index}-number" class="history-number">#${receipts.length - index}</span>
            <span id="history-item-${index}-lock" class="history-lock"><i class="fa-solid fa-link" aria-hidden="true"></i></span>
            <span id="history-item-${index}-direction" class="history-direction history-direction-${escapeHTML(direction)}">${escapeHTML(directionLabel)}</span>
            <strong id="history-item-${index}-status" class="history-status">PREVIEWED</strong>
            <span id="history-item-${index}-code" class="history-code">${escapeHTML(code)}</span>
            <small id="history-item-${index}-expires" class="history-expires">└ expires: ${expires}</small>
          </div>
          <div id="history-item-${index}-ghost" class="history-ghost">${escapeHTML(code)}</div>
          <div id="history-item-${index}-actions" class="history-actions">
            <button id="history-item-${index}-copy" type="button" class="history-copy" data-history-copy="${link}">[ COPY ]</button>
            ${secondaryAction}
            <small id="history-item-${index}-age" class="history-age">about ${age} minute${age === 1 ? "" : "s"} ago</small>
          </div>
        </article>`;
      })
      .join("");

    hydrateReceiveButtons(list, getFileLabel);

    list.querySelectorAll(".history-item").forEach((item) => {
      item.addEventListener("click", () => {
        window.location.href = item.dataset.detail;
      });
    });
    list.querySelectorAll("[data-history-copy]").forEach((btn) => {
      btn.addEventListener("click", (event) => {
        event.stopPropagation();
        copyText(btn.dataset.historyCopy || "", btn);
      });
    });
    list.querySelectorAll("[data-history-open]").forEach((btn) => {
      btn.addEventListener("click", (event) => {
        event.stopPropagation();
        window.location.href = btn.dataset.historyOpen;
      });
    });
    list.querySelectorAll("[data-receive-status]").forEach((btn) => {
      btn.addEventListener("click", (event) => {
        event.stopPropagation();
        if (btn.disabled || !btn.dataset.historyDownload) return;
        window.location.href = btn.dataset.historyDownload;
      });
    });
  }

  document
    .getElementById("home-history-clear")
    ?.addEventListener("click", () => {
      saveReceipts([]);
      renderHistory();
    });

  addCurrentCreatedReceipt();
  renderHistory();

  /* ── Burn confirmation ───────────────────────────────────────────────── */
  const burnButton = document.getElementById("created-burn-button");
  const burnOverlay = document.getElementById("burn-confirm-overlay");
  const burnCancel = document.getElementById("burn-cancel-button");
  const burnConfirm = document.getElementById("burn-confirm-button");

  if (burnButton && burnOverlay) {
    burnButton.addEventListener("click", () => {
      burnOverlay.hidden = false;
      burnCancel?.focus();
    });
  }

  burnCancel?.addEventListener("click", () => {
    if (burnOverlay) burnOverlay.hidden = true;
    burnButton?.focus();
  });

  burnOverlay?.addEventListener("click", (event) => {
    if (event.target === burnOverlay) {
      burnOverlay.hidden = true;
      burnButton?.focus();
    }
  });

  burnConfirm?.addEventListener("click", () => {
    const link = document.getElementById("created-body")?.dataset.createdLink;
    if (link)
      saveReceipts(getReceipts().filter((receipt) => receipt.link !== link));
    window.location.href = "/";
  });

  document.addEventListener("keydown", (event) => {
    if (event.key === "Escape" && burnOverlay && !burnOverlay.hidden) {
      burnOverlay.hidden = true;
      burnButton?.focus();
    }
  });

  /* ── Created page progress ───────────────────────────────────────────── */
  const progressBar = document.getElementById(
    "created-expiration-progress-bar",
  );
  const expiresText = document.getElementById("created-timeline-expires-time");
  const createdBody = document.getElementById("created-body");
  const ttl = parseInt(createdBody?.dataset.createdTtl || "0", 10);
  const expiresAt = Date.parse(createdBody?.dataset.createdExpires || "");

  function formatRemaining(seconds) {
    if (seconds <= 0) return "expired";
    const days = Math.floor(seconds / 86400);
    const hours = Math.floor((seconds % 86400) / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;

    if (days > 0) return `${days}d ${hours}h ${minutes}m ${secs}s remaining`;
    if (hours > 0) return `${hours}h ${minutes}m ${secs}s remaining`;
    if (minutes > 0) return `${minutes}m ${secs}s remaining`;
    return `${secs}s remaining`;
  }

  function updateCreatedProgress() {
    if (!progressBar || !ttl || Number.isNaN(expiresAt)) return false;
    const remainingSeconds = Math.max(
      0,
      Math.ceil((expiresAt - Date.now()) / 1000),
    );
    const percent = Math.max(0, Math.min(100, (remainingSeconds / ttl) * 100));
    progressBar.style.width = percent + "%";
    if (expiresText) {
      expiresText.textContent = `${createdBody.dataset.createdExpires} — ${formatRemaining(remainingSeconds)}`;
    }
    return remainingSeconds > 0;
  }

  if (updateCreatedProgress()) {
    const createdProgressInterval = setInterval(() => {
      if (!updateCreatedProgress()) clearInterval(createdProgressInterval);
    }, 1000);
  }

  /* ── Keyboard shortcut: Ctrl/Cmd+Enter to submit ─────────────────────── */
  const textForm = document.getElementById("home-text-form");
  const textArea = document.getElementById("home-text-secret-input");
  if (textArea && textForm) {
    textArea.addEventListener("keydown", (e) => {
      if ((e.ctrlKey || e.metaKey) && e.key === "Enter")
        textForm.requestSubmit();
    });
  }
});
