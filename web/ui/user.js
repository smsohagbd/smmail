const state = {
  token: localStorage.getItem("userToken") || "",
  eventsLimit: 25,
  eventsOffset: 0,
  eventsTotal: 0,
  pendingLimit: 25,
  pendingOffset: 0,
  pendingTotal: 0,
  me: null,
};

const loginStatus = document.getElementById("loginStatus");
if (!state.token) location.href = "/ui/index.html";
const smtpTestState = { smtpID: 0 };

function setLoginStatus(msg, isError = false) {
  loginStatus.textContent = msg;
  loginStatus.style.color = isError ? "#ff8f8f" : "#8fb6ff";
}

function initLayoutNav() {
  const shell = document.querySelector(".app-shell");
  const toggle = document.getElementById("sidebarToggle");
  const links = Array.from(document.querySelectorAll(".nav-link"));
  const panels = Array.from(document.querySelectorAll(".panel-section"));

  function showSection(name) {
    links.forEach((l) => l.classList.toggle("active", l.dataset.section === name));
    panels.forEach((p) => p.classList.toggle("hidden", p.dataset.panel !== name));
    shell.classList.remove("sidebar-open");
  }

  links.forEach((link) => link.addEventListener("click", () => showSection(link.dataset.section)));
  toggle.addEventListener("click", () => shell.classList.toggle("sidebar-open"));
  showSection("overview");
}

initLayoutNav();

document.getElementById("logoutBtn").addEventListener("click", () => {
  localStorage.removeItem("userToken");
  location.href = "/ui/index.html";
});

document.getElementById("refreshUserEvents").addEventListener("click", async () => {
  state.eventsOffset = 0;
  await refreshUser();
});
document.getElementById("userEventsPrev").addEventListener("click", async () => {
  state.eventsOffset = Math.max(0, state.eventsOffset - state.eventsLimit);
  await refreshUser();
});
document.getElementById("userEventsNext").addEventListener("click", async () => {
  if (state.eventsOffset + state.eventsLimit >= state.eventsTotal) return;
  state.eventsOffset += state.eventsLimit;
  await refreshUser();
});
document.getElementById("userDeleteLogsBtn").addEventListener("click", deleteLogsUser);
document.getElementById("refreshPendingUser").addEventListener("click", async () => {
  state.pendingOffset = 0;
  await refreshUser();
});
document.getElementById("userPendingPrev").addEventListener("click", async () => {
  state.pendingOffset = Math.max(0, state.pendingOffset - state.pendingLimit);
  await refreshUser();
});
document.getElementById("userPendingNext").addEventListener("click", async () => {
  if (state.pendingOffset + state.pendingLimit >= state.pendingTotal) return;
  state.pendingOffset += state.pendingLimit;
  await refreshUser();
});
document.getElementById("userDeletePendingBtn").addEventListener("click", deletePendingUser);
document.getElementById("userSmtpTestClose").addEventListener("click", closeUserSMTPTestModal);
document.getElementById("userSmtpTestForm").addEventListener("submit", submitUserSMTPTest);

document.getElementById("userSMTPForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = new FormData(e.target);
  try {
    await authed("/api/user/smtps", {
      method: "POST",
      body: JSON.stringify({
        name: f.get("name"),
        host: f.get("host"),
        port: Number(f.get("port")),
        username: f.get("username"),
        password: f.get("password"),
        from_email: f.get("from_email"),
        enabled: f.get("enabled") === "on",
      }),
    });
    setLoginStatus("SMTP added.");
    await refreshUser();
  } catch (err) {
    setLoginStatus(err.message, true);
  }
});

document.getElementById("assignSMTPBtn").addEventListener("click", async () => {
  const smtpID = Number(document.getElementById("assignSMTPId").value);
  const weight = Number(document.getElementById("assignWeight").value);
  if (!smtpID) return;
  try {
    await authed("/api/user/smtps/assign", {
      method: "POST",
      body: JSON.stringify({ smtp_id: smtpID, weight, rotation: document.getElementById("rotationToggle").checked }),
    });
    setLoginStatus("SMTP assigned to your account.");
    await refreshUser();
  } catch (err) {
    setLoginStatus(err.message, true);
  }
});

document.getElementById("saveRotationBtn").addEventListener("click", async () => {
  try {
    await authed("/api/user/rotation", {
      method: "POST",
      body: JSON.stringify({ enabled: document.getElementById("rotationToggle").checked }),
    });
    setLoginStatus("Rotation updated.");
    await refreshUser();
  } catch (err) {
    setLoginStatus(err.message, true);
  }
});

document.getElementById("changePlanBtn").addEventListener("click", async () => {
  const plan = document.getElementById("planSelect").value;
  if (!plan) return;
  try {
    await authed("/api/user/plan", {
      method: "POST",
      body: JSON.stringify({ package_name: plan }),
    });
    setLoginStatus(`Plan changed to ${plan}.`);
    await refreshUser();
  } catch (err) {
    setLoginStatus(err.message, true);
  }
});

async function refreshUser() {
  try {
    const me = await authed("/api/user/me");
    const usage = await authedFallback("/api/user/usage", {
      user_id: me.id,
      username: me.username,
      display_name: me.display_name || "",
      plan_name: me.plan_name || "starter",
      monthly_limit: me.monthly_limit || 0,
      month_sent: 0,
      month_failed: 0,
      day_sent: 0,
      day_failed: 0,
    });
    const events = await authedFallback(`/api/user/events?limit=${state.eventsLimit}&offset=${state.eventsOffset}`, {
      items: [],
      total: 0,
    });
    const pending = await authedFallback(`/api/user/pending?limit=${state.pendingLimit}&offset=${state.pendingOffset}`, {
      items: [],
      total: 0,
    });
    const mySMTPs = await authedOptionalArray("/api/user/smtps");
    const available = await authedOptionalArray("/api/user/smtps/available");
    const assigned = await authedOptionalArray("/api/user/smtps/assigned");
    const packages = await authedOptionalArray("/api/user/packages");
    state.me = me;
    applySMTPMode(me);
    renderStats(me, usage);
    renderEvents(events);
    renderPending(pending);
    renderMySMTPs(mySMTPs);
    renderAvailableSMTPs(available);
    renderAssignedSMTPs(assigned);
    renderPackageCatalog(packages, me.plan_name);
    document.getElementById("rotationToggle").checked = !!me.rotation_on;
    setLoginStatus(`Logged in as ${me.username}`);
  } catch (err) {
    if (String(err.message).includes("401")) {
      localStorage.removeItem("userToken");
      location.href = "/ui/index.html";
      return;
    }
    setLoginStatus(err.message, true);
  }
}

function applySMTPMode(me) {
  const allowOwn = me.allow_user_smtp !== false;
  const smtpNav = document.querySelector('.nav-link[data-section="smtp"]');
  const smtpPanels = Array.from(document.querySelectorAll('.panel-section[data-panel="smtp"]'));
  const notice = document.getElementById("smtpModeNotice");
  if (smtpNav) smtpNav.style.display = allowOwn ? "" : "none";
  smtpPanels.forEach((p) => { p.style.display = allowOwn ? "" : "none"; });
  if (!allowOwn) {
    if (smtpNav && smtpNav.classList.contains("active")) {
      const ov = document.querySelector('.nav-link[data-section="overview"]');
      if (ov) ov.click();
    }
    if (notice) notice.textContent = "SMTP mode: System SMTP only (set by admin). Your own SMTP section is hidden.";
  } else if (notice) {
    notice.textContent = "SMTP mode: You can use your own SMTP and system SMTP.";
  }
}

function renderStats(me, usage) {
  const pct = usage.monthly_limit > 0 ? Math.min(100, Math.round((usage.month_sent / usage.monthly_limit) * 100)) : 0;
  document.getElementById("userStats").innerHTML = `
    <article class="stat"><p>User</p><h3>${me.username}</h3><small>${me.display_name || "-"}</small></article>
    <article class="stat"><p>Plan</p><h3>${me.plan_name}</h3><small>Limit ${me.monthly_limit}/month</small></article>
    <article class="stat"><p>Rotation</p><h3>${me.rotation_on ? "ON" : "OFF"}</h3><small>Multi SMTP routing</small></article>
    <article class="stat"><p>Sent (Month)</p><h3>${usage.month_sent}</h3><small>Failed ${usage.month_failed}</small></article>
    <article class="stat" style="grid-column: span 2;"><p>Monthly Usage</p><h3>${usage.month_sent}/${usage.monthly_limit}</h3><div class="meter"><span style="width:${pct}%"></span></div></article>
  `;
}

function renderMySMTPs(items) {
  items = Array.isArray(items) ? items : [];
  document.getElementById("mySMTPRows").innerHTML = items.length === 0 ? `<tr><td colspan="8" class="muted">No SMTP accounts yet.</td></tr>` : items.map((s) => `
    <tr><td>${s.id}</td><td>${s.name}</td><td>${s.host}</td><td>${s.port}</td><td>${s.username}</td><td>${s.from_email}</td><td>${s.enabled ? "Yes" : "No"}</td><td><button class="table-btn" onclick="testMySMTP(${s.id}, '${String(s.from_email || "").replace(/'/g, "\\'")}')">Test</button><button class="table-btn danger-btn" onclick="deleteMySMTP(${s.id})">Delete</button></td></tr>
  `).join("");
}

function renderAvailableSMTPs(items) {
  items = Array.isArray(items) ? items : [];
  document.getElementById("availableSMTPRows").innerHTML = items.length === 0 ? `<tr><td colspan="6" class="muted">No available SMTP accounts.</td></tr>` : items.map((s) => `
    <tr><td>${s.id}</td><td>${s.owner_user_id === 0 ? "Admin" : "User"}</td><td>${s.name}</td><td>${s.host}</td><td>${s.port}</td><td>${s.from_email}</td></tr>
  `).join("");
}

function renderAssignedSMTPs(items) {
  items = Array.isArray(items) ? items : [];
  document.getElementById("assignedSMTPRows").innerHTML = items.length === 0 ? `<tr><td colspan="7" class="muted">No assigned SMTP accounts.</td></tr>` : items.map((s) => `
    <tr><td>${s.id}</td><td>${s.name}</td><td>${s.host}</td><td>${s.port}</td><td>${s.owner_user_id === 0 ? "Admin" : "Me"}</td><td>${s.from_email}</td><td><button class="table-btn" onclick="testMySMTP(${s.id}, '${String(s.from_email || "").replace(/'/g, "\\'")}')">Test</button><button class="table-btn danger-btn" onclick="unassignMySMTP(${s.id})">Unassign</button></td></tr>
  `).join("");
}

function renderEvents(events) {
  const items = events.items || [];
  state.eventsTotal = events.total || 0;
  document.getElementById("userEventRows").innerHTML = items.length === 0 ? `<tr><td colspan="5" class="muted">No logs found.</td></tr>` : items.map((e) => `
    <tr><td>${new Date(e.created_at).toLocaleString()}</td><td>${e.mail_from}</td><td>${e.rcpt_to}</td><td><span class="badge ${e.status}">${e.status}</span></td><td>${e.reason || "-"}</td></tr>
  `).join("");
  const page = Math.floor(state.eventsOffset / state.eventsLimit) + 1;
  const pages = Math.max(1, Math.ceil(state.eventsTotal / state.eventsLimit));
  document.getElementById("userEventsPageInfo").textContent = `Page ${page}/${pages} (${state.eventsTotal} logs)`;
  document.getElementById("userEventsPrev").disabled = state.eventsOffset === 0;
  document.getElementById("userEventsNext").disabled = state.eventsOffset + state.eventsLimit >= state.eventsTotal;
}

function renderPackageCatalog(items, currentPlan) {
  items = Array.isArray(items) ? items : [];
  const select = document.getElementById("planSelect");
  select.innerHTML = items.map((p) => `<option value="${p.name}" ${p.name === currentPlan ? "selected" : ""}>${p.name}</option>`).join("");
  document.getElementById("packageCatalogRows").innerHTML = items.length === 0 ? `<tr><td colspan="7" class="muted">No packages found.</td></tr>` : items.map((p) => `
    <tr><td>${p.name}</td><td>${p.monthly_limit}</td><td>${p.limit_per_sec}</td><td>${p.limit_per_min}</td><td>${p.limit_per_hour}</td><td>${p.limit_per_day}</td><td>${p.is_default ? "Yes" : "No"}</td></tr>
  `).join("");
}

function renderPending(pending) {
  const items = pending.items || [];
  state.pendingTotal = pending.total || 0;
  document.getElementById("userPendingRows").innerHTML = items.length === 0 ? `<tr><td colspan="6" class="muted">No pending emails.</td></tr>` : items.map((p) => `
    <tr><td>${new Date(p.created_at).toLocaleString()}</td><td>${p.mail_from}</td><td>${p.rcpt_to}</td><td>${p.attempts}</td><td>${new Date(p.next_attempt_at).toLocaleString()}</td><td>${p.last_error || "-"}</td></tr>
  `).join("");
  const page = Math.floor(state.pendingOffset / state.pendingLimit) + 1;
  const pages = Math.max(1, Math.ceil(state.pendingTotal / state.pendingLimit));
  document.getElementById("userPendingPageInfo").textContent = `Page ${page}/${pages} (${state.pendingTotal} pending)`;
  document.getElementById("userPendingPrev").disabled = state.pendingOffset === 0;
  document.getElementById("userPendingNext").disabled = state.pendingOffset + state.pendingLimit >= state.pendingTotal;
}

async function authed(url, options = {}) {
  return fetchJson(url, { ...options, headers: { ...(options.headers || {}), "X-User-Token": state.token } });
}

async function authedOptionalArray(url) {
  try {
    const out = await authed(url);
    return Array.isArray(out) ? out : [];
  } catch (err) {
    if (String(err.message).includes("404")) return [];
    throw err;
  }
}

async function authedFallback(url, fallbackValue) {
  try {
    return await authed(url);
  } catch (err) {
    const msg = String(err.message || "");
    if (msg.includes("401")) throw err;
    return fallbackValue;
  }
}

async function fetchJson(url, options = {}) {
  const res = await fetch(url, {
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(options.headers || {}),
    },
  });
  const text = await res.text();
  if (!res.ok) {
    let msg = text || `HTTP ${res.status}`;
    try {
      const obj = text ? JSON.parse(text) : null;
      if (obj && obj.error) msg = obj.error;
    } catch (_) {}
    throw new Error(msg);
  }
  return text ? JSON.parse(text) : null;
}

window.deleteMySMTP = async function(id) {
  if (!confirm(`Delete SMTP ${id}?`)) return;
  await authed(`/api/user/smtps/${id}`, { method: "DELETE" });
  await refreshUser();
};

window.unassignMySMTP = async function(smtpID) {
  if (!confirm(`Unassign SMTP ${smtpID}?`)) return;
  await authed(`/api/user/smtps/assign/${smtpID}`, { method: "DELETE" });
  await refreshUser();
};

window.testMySMTP = async function(id, fallbackTo) {
  openUserSMTPTestModal(id, fallbackTo || "");
};

function openUserSMTPTestModal(id, toDefault) {
  smtpTestState.smtpID = id;
  document.getElementById("userSmtpTestTitle").textContent = `SMTP Test #${id}`;
  document.getElementById("userSmtpTestTo").value = toDefault;
  document.getElementById("userSmtpTestSubject").value = "SMTP Account Test";
  document.getElementById("userSmtpTestBody").value = "This is a test email from user dashboard.";
  const res = document.getElementById("userSmtpTestResult");
  res.className = "muted";
  res.textContent = "Ready.";
  document.getElementById("userSmtpTestModal").classList.remove("hidden");
}

function closeUserSMTPTestModal() {
  document.getElementById("userSmtpTestModal").classList.add("hidden");
}

async function submitUserSMTPTest(e) {
  e.preventDefault();
  const id = smtpTestState.smtpID;
  if (!id) return;
  const to = document.getElementById("userSmtpTestTo").value.trim();
  const subject = document.getElementById("userSmtpTestSubject").value;
  const body = document.getElementById("userSmtpTestBody").value;
  const res = document.getElementById("userSmtpTestResult");
  const btn = document.getElementById("userSmtpTestSendBtn");
  if (!to) {
    res.className = "result-err";
    res.textContent = "Recipient is required.";
    return;
  }
  btn.disabled = true;
  res.className = "muted";
  res.textContent = "Sending...";
  try {
    const out = await authed(`/api/user/smtps/${id}/test`, {
      method: "POST",
      body: JSON.stringify({ to, subject, body }),
    });
    res.className = "result-ok";
    res.textContent = out?.message || "Test email sent.";
    setLoginStatus(`SMTP ${id} test sent successfully.`);
  } catch (err) {
    res.className = "result-err";
    res.textContent = err.message;
    setLoginStatus(err.message, true);
  } finally {
    btn.disabled = false;
  }
}

async function deleteLogsUser() {
  const mode = document.getElementById("userLogDeleteMode").value;
  const fromDate = document.getElementById("userLogFromDate").value;
  const toDate = document.getElementById("userLogToDate").value;
  if (!confirm("Delete your logs with selected filter?")) return;
  await authed("/api/user/events/delete", {
    method: "POST",
    body: JSON.stringify({
      mode,
      from_date: fromDate,
      to_date: toDate,
    }),
  });
  state.eventsOffset = 0;
  await refreshUser();
}

async function deletePendingUser() {
  const mode = document.getElementById("userPendingDeleteMode").value;
  const fromDate = document.getElementById("userPendingFromDate").value;
  const toDate = document.getElementById("userPendingToDate").value;
  if (!confirm("Delete your pending emails with selected filter?")) return;
  await authed("/api/user/pending/delete", {
    method: "POST",
    body: JSON.stringify({
      mode,
      from_date: fromDate,
      to_date: toDate,
    }),
  });
  state.pendingOffset = 0;
  await refreshUser();
}

refreshUser();
