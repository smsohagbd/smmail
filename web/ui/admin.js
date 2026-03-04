const state = {
  session: localStorage.getItem("adminSessionToken") || "",
  eventsLimit: 25,
  eventsOffset: 0,
  eventsTotal: 0,
  pendingLimit: 25,
  pendingOffset: 0,
  pendingTotal: 0,
  packages: [],
};

const statusEl = document.getElementById("status");
if (!state.session) location.href = "/ui/index.html";
const smtpTestState = { smtpID: 0 };

function setStatus(msg, isError = false) {
  statusEl.textContent = msg;
  statusEl.style.color = isError ? "#ff8f8f" : "#8fb6ff";
}

function initLayoutNav() {
  const shell = document.getElementById("appShell");
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
  localStorage.removeItem("adminSessionToken");
  location.href = "/ui/index.html";
});

document.getElementById("refreshAll").addEventListener("click", refreshAll);
document.getElementById("refreshUsage").addEventListener("click", loadUsageTable);
document.getElementById("refreshEvents").addEventListener("click", async () => {
  state.eventsOffset = 0;
  await loadEvents();
});
document.getElementById("eventsPrev").addEventListener("click", async () => {
  state.eventsOffset = Math.max(0, state.eventsOffset - state.eventsLimit);
  await loadEvents();
});
document.getElementById("eventsNext").addEventListener("click", async () => {
  if (state.eventsOffset + state.eventsLimit >= state.eventsTotal) return;
  state.eventsOffset += state.eventsLimit;
  await loadEvents();
});
document.getElementById("deleteLogsBtn").addEventListener("click", deleteLogsAdmin);
document.getElementById("refreshPending").addEventListener("click", async () => {
  state.pendingOffset = 0;
  await loadPending();
});
document.getElementById("pendingPrev").addEventListener("click", async () => {
  state.pendingOffset = Math.max(0, state.pendingOffset - state.pendingLimit);
  await loadPending();
});
document.getElementById("pendingNext").addEventListener("click", async () => {
  if (state.pendingOffset + state.pendingLimit >= state.pendingTotal) return;
  state.pendingOffset += state.pendingLimit;
  await loadPending();
});
document.getElementById("deletePendingBtn").addEventListener("click", deletePendingAdmin);

document.getElementById("createUserForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = new FormData(e.target);
  try {
    await api("/api/admin/users", {
      method: "POST",
      body: JSON.stringify({
        username: f.get("username"),
        password: f.get("password"),
        display_name: f.get("display_name"),
        plan_name: f.get("plan_name"),
        monthly_limit: Number(f.get("monthly_limit")),
        rotation_on: f.get("rotation_on") === "on",
        enabled: f.get("enabled") === "on",
        allow_user_smtp: f.get("allow_user_smtp") === "on",
      }),
    });
    setStatus("User created.");
    e.target.reset();
    await refreshAll();
  } catch (err) {
    setStatus(err.message, true);
  }
});

document.getElementById("packageForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = new FormData(e.target);
  try {
    await api("/api/admin/packages", {
      method: "POST",
      body: JSON.stringify({
        name: f.get("name"),
        monthly_limit: Number(f.get("monthly_limit")),
        limit_per_sec: Number(f.get("limit_per_sec")),
        limit_per_min: Number(f.get("limit_per_min")),
        limit_per_hour: Number(f.get("limit_per_hour")),
        limit_per_day: Number(f.get("limit_per_day")),
        throttle_ms: Number(f.get("throttle_ms")),
      }),
    });
    setStatus("Package saved.");
    await loadPackages();
  } catch (err) {
    setStatus(err.message, true);
  }
});

document.getElementById("setDefaultPackage").addEventListener("click", async () => {
  const name = document.getElementById("defaultPackageName").value.trim();
  if (!name) return;
  try {
    await api("/api/admin/packages/default", { method: "POST", body: JSON.stringify({ name }) });
    setStatus("Default package updated.");
    await loadPackages();
  } catch (err) {
    setStatus(err.message, true);
  }
});

document.getElementById("adminSMTPForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = new FormData(e.target);
  try {
    await api("/api/admin/smtps", {
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
    setStatus("Admin SMTP added.");
    await loadAdminSMTPs();
  } catch (err) {
    setStatus(err.message, true);
  }
});

document.getElementById("assignSMTPForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = new FormData(e.target);
  const userID = Number(f.get("user_id"));
  try {
    await api(`/api/admin/users/${userID}/smtp-assign`, {
      method: "POST",
      body: JSON.stringify({
        smtp_id: Number(f.get("smtp_id")),
        weight: Number(f.get("weight")),
        enabled: f.get("enabled") === "on",
      }),
    });
    setStatus("SMTP assigned to user.");
  } catch (err) {
    setStatus(err.message, true);
  }
});

document.getElementById("loadAssignedBtn").addEventListener("click", loadAssignedSMTPs);
document.getElementById("smtpTestClose").addEventListener("click", closeSMTPTestModal);
document.getElementById("smtpTestForm").addEventListener("submit", submitSMTPTestAdmin);

async function refreshAll() {
  try {
    await loadOverview();
    await loadPackages();
    await loadUsageTable();
    await loadAdminSMTPs();
    await loadEvents();
    await loadPending();
    setStatus("Dashboard refreshed.");
  } catch (err) {
    setStatus(err.message, true);
  }
}

async function loadOverview() {
  const data = await api("/api/admin/overview");
  const cards = [
    ["Total Users", data.totals.users_total],
    ["Active Users", data.totals.users_active],
    ["New Users 24h", data.totals.new_users_24h],
    ["Total Logs", data.totals.logs_total],
    ["Sent 24h", data.totals.sent_24h],
    ["Failed 24h", data.totals.failed_24h],
    ["Month Sent", data.totals.month_sent],
    ["Queue Pending", data.totals.queue_pending],
    ["Queue Failed", data.totals.queue_failed],
  ];
  document.getElementById("statCards").innerHTML = cards.map(([k, v]) => `<article class="stat"><p>${k}</p><h3>${v}</h3></article>`).join("");
}

async function loadUsageTable() {
  const packageOptions = state.packages.map((p) => `<option value="${p.name}">${p.name}</option>`).join("");
  let users = [];
  try {
    const usersRaw = await api("/api/admin/users");
    users = Array.isArray(usersRaw) ? usersRaw : [];
  } catch (err) {
    setStatus(`Users endpoint failed: ${err.message}`, true);
  }
  let usageByUser = {};
  try {
    const usageRaw = await api("/api/admin/user-usage?limit=300");
    const usage = Array.isArray(usageRaw) ? usageRaw : [];
    usageByUser = usage.reduce((acc, x) => {
      acc[x.user_id] = x;
      return acc;
    }, {});
    if (users.length === 0 && usage.length > 0) {
      users = usage.map((u) => ({
        id: u.user_id,
        username: u.username,
        display_name: u.display_name || "",
        plan_name: u.plan_name || "starter",
        monthly_limit: u.monthly_limit || 0,
        allow_user_smtp: u.allow_user_smtp !== false,
      }));
    }
  } catch (err) {
    setStatus("Usage endpoint failed, showing users without usage counters.", true);
  }
  const items = users.map((u) => {
    const m = usageByUser[u.id] || {};
    return {
      user_id: u.id,
      username: u.username,
      display_name: u.display_name || "",
      plan_name: u.plan_name || "starter",
      monthly_limit: u.monthly_limit || 0,
      allow_user_smtp: u.allow_user_smtp !== false,
      month_sent: Number(m.month_sent || 0),
      day_sent: Number(m.day_sent || 0),
    };
  });
  document.getElementById("usageRows").innerHTML = items.length === 0 ? `<tr><td colspan="6" class="muted">No users found.</td></tr>` : items.map((u) => {
    const pct = u.monthly_limit > 0 ? Math.min(100, Math.round((u.month_sent / u.monthly_limit) * 100)) : 0;
    return `
      <tr>
        <td>#${u.user_id} ${u.username}<div class="muted">${u.display_name || "-"}</div></td>
        <td>${u.plan_name}</td>
        <td>${u.month_sent}</td>
        <td>${u.day_sent}</td>
        <td><div>${u.month_sent}/${u.monthly_limit}</div><div class="meter"><span style="width:${pct}%"></span></div></td>
        <td>
          <div class="inline-row">
            <select id="plan-user-${u.user_id}">${packageOptions}</select>
            <button class="table-btn" onclick="assignPackage(${u.user_id})">Apply</button>
          </div>
          <div class="inline-row" style="margin-top:6px;">
            <select id="smtp-mode-user-${u.user_id}">
              <option value="system" ${u.allow_user_smtp ? "" : "selected"}>System SMTP only</option>
              <option value="own" ${u.allow_user_smtp ? "selected" : ""}>Allow own SMTP</option>
            </select>
            <button class="table-btn" onclick="setSMTPMode(${u.user_id})">Save</button>
          </div>
          <div style="margin-top:6px;">
            <button class="table-btn" onclick="resetPass(${u.user_id})">Reset</button>
            <button class="table-btn danger-btn" onclick="deleteUser(${u.user_id})">Delete</button>
          </div>
        </td>
      </tr>
    `;
  }).join("");
  items.forEach((u) => {
    const sel = document.getElementById(`plan-user-${u.user_id}`);
    if (sel) sel.value = u.plan_name || "";
  });
}

async function loadEvents() {
  const userID = document.getElementById("eventsUserId").value.trim();
  const q = userID
    ? `?limit=${state.eventsLimit}&offset=${state.eventsOffset}&user_id=${encodeURIComponent(userID)}`
    : `?limit=${state.eventsLimit}&offset=${state.eventsOffset}`;
  const out = await api(`/api/admin/events${q}`);
  state.eventsTotal = out.total || 0;
  const events = out.items || [];
  document.getElementById("eventRows").innerHTML = events.length === 0 ? `<tr><td colspan="6" class="muted">No logs found for this filter.</td></tr>` : events.map((e) => `
    <tr><td>${new Date(e.created_at).toLocaleString()}</td><td>${e.user_id}</td><td>${e.mail_from}</td><td>${e.rcpt_to}</td><td><span class="badge ${e.status}">${e.status}</span></td><td>${e.reason || "-"}</td></tr>
  `).join("");
  const page = Math.floor(state.eventsOffset / state.eventsLimit) + 1;
  const pages = Math.max(1, Math.ceil(state.eventsTotal / state.eventsLimit));
  document.getElementById("eventsPageInfo").textContent = `Page ${page}/${pages} (${state.eventsTotal} logs)`;
  document.getElementById("eventsPrev").disabled = state.eventsOffset === 0;
  document.getElementById("eventsNext").disabled = state.eventsOffset + state.eventsLimit >= state.eventsTotal;
}

async function loadPending() {
  const userID = document.getElementById("pendingUserId").value.trim();
  const q = userID
    ? `?limit=${state.pendingLimit}&offset=${state.pendingOffset}&user_id=${encodeURIComponent(userID)}`
    : `?limit=${state.pendingLimit}&offset=${state.pendingOffset}`;
  const out = await api(`/api/admin/pending${q}`);
  state.pendingTotal = out.total || 0;
  const items = out.items || [];
  document.getElementById("pendingRows").innerHTML = items.length === 0 ? `<tr><td colspan="7" class="muted">No pending emails found.</td></tr>` : items.map((p) => `
    <tr><td>${new Date(p.created_at).toLocaleString()}</td><td>${p.user_id}</td><td>${p.mail_from}</td><td>${p.rcpt_to}</td><td>${p.attempts}</td><td>${new Date(p.next_attempt_at).toLocaleString()}</td><td>${p.last_error || "-"}</td></tr>
  `).join("");
  const page = Math.floor(state.pendingOffset / state.pendingLimit) + 1;
  const pages = Math.max(1, Math.ceil(state.pendingTotal / state.pendingLimit));
  document.getElementById("pendingPageInfo").textContent = `Page ${page}/${pages} (${state.pendingTotal} pending)`;
  document.getElementById("pendingPrev").disabled = state.pendingOffset === 0;
  document.getElementById("pendingNext").disabled = state.pendingOffset + state.pendingLimit >= state.pendingTotal;
}

async function loadPackages() {
  const raw = await api("/api/admin/packages");
  const items = Array.isArray(raw) ? raw : [];
  state.packages = items;
  document.getElementById("packageRows").innerHTML = items.length === 0 ? `<tr><td colspan="9" class="muted">No packages found.</td></tr>` : items.map((p) => `
    <tr>
      <td>${p.name}</td><td>${p.monthly_limit}</td><td>${p.limit_per_sec}</td><td>${p.limit_per_min}</td><td>${p.limit_per_hour}</td><td>${p.limit_per_day}</td><td>${p.throttle_ms}ms</td><td>${p.is_default ? "Yes" : "No"}</td><td><button class="table-btn danger-btn" ${p.is_default ? "disabled" : ""} onclick="deletePackage('${p.name}')">Delete</button></td>
    </tr>
  `).join("");
}

async function loadAdminSMTPs() {
  const raw = await api("/api/admin/smtps");
  const items = Array.isArray(raw) ? raw : [];
  document.getElementById("adminSMTPRows").innerHTML = items.length === 0 ? `<tr><td colspan="8" class="muted">No SMTP accounts found.</td></tr>` : items.map((s) => `
    <tr><td>${s.id}</td><td>${s.name}</td><td>${s.host}</td><td>${s.port}</td><td>${s.username}</td><td>${s.from_email}</td><td>${s.enabled ? "Yes" : "No"}</td><td><button class="table-btn" onclick="testSMTPAdmin(${s.id}, '${String(s.from_email || "").replace(/'/g, "\\'")}')">Test</button><button class="table-btn danger-btn" onclick="deleteSMTP(${s.id})">Delete</button></td></tr>
  `).join("");
}

async function loadAssignedSMTPs() {
  const userID = Number(document.getElementById("assignedUserId").value);
  if (!userID) return;
  try {
    const raw = await api(`/api/admin/users/${userID}/smtp-assign`);
    const items = Array.isArray(raw) ? raw : [];
    document.getElementById("assignedSMTPRows").innerHTML = items.length === 0 ? `<tr><td colspan="7" class="muted">No assigned SMTPs for this user.</td></tr>` : items.map((s) => `
      <tr><td>${s.id}</td><td>${s.name}</td><td>${s.host}</td><td>${s.port}</td><td>${s.username}</td><td>${s.from_email}</td><td><button class="table-btn danger-btn" onclick="unassignSMTP(${userID}, ${s.id})">Unassign</button></td></tr>
    `).join("");
  } catch (err) {
    setStatus(err.message, true);
  }
}

window.resetPass = async function(id) {
  const password = prompt("New password:");
  if (!password) return;
  await api(`/api/admin/users/${id}/password`, { method: "POST", body: JSON.stringify({ password }) });
  setStatus(`Password updated for user ${id}.`);
};

window.deleteUser = async function(id) {
  if (!confirm(`Delete user ${id}? This is permanent.`)) return;
  await api(`/api/admin/users/${id}`, { method: "DELETE" });
  await refreshAll();
};

window.deletePackage = async function(name) {
  if (!confirm(`Delete package '${name}'?`)) return;
  await api(`/api/admin/packages/${encodeURIComponent(name)}`, { method: "DELETE" });
  await loadPackages();
};

window.deleteSMTP = async function(id) {
  if (!confirm(`Delete SMTP ${id}?`)) return;
  await api(`/api/admin/smtps/${id}`, { method: "DELETE" });
  await loadAdminSMTPs();
};

window.testSMTPAdmin = async function(id, fallbackTo) {
  openSMTPTestModal(id, fallbackTo || "");
};

function openSMTPTestModal(id, toDefault) {
  smtpTestState.smtpID = id;
  document.getElementById("smtpTestTitle").textContent = `SMTP Test #${id}`;
  document.getElementById("smtpTestTo").value = toDefault;
  document.getElementById("smtpTestSubject").value = "SMTP Account Test";
  document.getElementById("smtpTestBody").value = "This is a test email from admin dashboard.";
  const res = document.getElementById("smtpTestResult");
  res.className = "muted";
  res.textContent = "Ready.";
  document.getElementById("smtpTestModal").classList.remove("hidden");
}

function closeSMTPTestModal() {
  document.getElementById("smtpTestModal").classList.add("hidden");
}

async function submitSMTPTestAdmin(e) {
  e.preventDefault();
  const id = smtpTestState.smtpID;
  if (!id) return;
  const to = document.getElementById("smtpTestTo").value.trim();
  const subject = document.getElementById("smtpTestSubject").value;
  const body = document.getElementById("smtpTestBody").value;
  const res = document.getElementById("smtpTestResult");
  const btn = document.getElementById("smtpTestSendBtn");
  if (!to) {
    res.className = "result-err";
    res.textContent = "Recipient is required.";
    return;
  }
  btn.disabled = true;
  res.className = "muted";
  res.textContent = "Sending...";
  try {
    const out = await api(`/api/admin/smtps/${id}/test`, {
      method: "POST",
      body: JSON.stringify({ to, subject, body }),
    });
    res.className = "result-ok";
    res.textContent = out?.message || "Test email sent.";
    setStatus(`SMTP ${id} test sent successfully.`);
  } catch (err) {
    res.className = "result-err";
    res.textContent = err.message;
    setStatus(err.message, true);
  } finally {
    btn.disabled = false;
  }
}

window.unassignSMTP = async function(userID, smtpID) {
  if (!confirm(`Unassign SMTP ${smtpID} from user ${userID}?`)) return;
  await api(`/api/admin/users/${userID}/smtp-assign/${smtpID}`, { method: "DELETE" });
  await loadAssignedSMTPs();
};

window.assignPackage = async function(userID) {
  const select = document.getElementById(`plan-user-${userID}`);
  const packageName = select ? select.value : "";
  if (!packageName) return;
  await api(`/api/admin/users/${userID}/package`, { method: "POST", body: JSON.stringify({ package_name: packageName }) });
  await refreshAll();
};

window.setSMTPMode = async function(userID) {
  const select = document.getElementById(`smtp-mode-user-${userID}`);
  if (!select) return;
  await api(`/api/admin/users/${userID}/smtp-mode`, {
    method: "POST",
    body: JSON.stringify({ allow_user_smtp: select.value === "own" }),
  });
  await refreshAll();
};

async function deleteLogsAdmin() {
  const mode = document.getElementById("logDeleteMode").value;
  const userIDRaw = document.getElementById("eventsUserId").value.trim();
  const fromDate = document.getElementById("logFromDate").value;
  const toDate = document.getElementById("logToDate").value;
  const userID = userIDRaw ? Number(userIDRaw) : 0;
  if (!confirm("Delete logs with selected filter?")) return;
  await api("/api/admin/events/delete", {
    method: "POST",
    body: JSON.stringify({
      mode,
      user_id: userID,
      from_date: fromDate,
      to_date: toDate,
    }),
  });
  state.eventsOffset = 0;
  await loadEvents();
  setStatus("Logs deleted.");
}

async function deletePendingAdmin() {
  const mode = document.getElementById("pendingDeleteMode").value;
  const userIDRaw = document.getElementById("pendingUserId").value.trim();
  const fromDate = document.getElementById("pendingFromDate").value;
  const toDate = document.getElementById("pendingToDate").value;
  const userID = userIDRaw ? Number(userIDRaw) : 0;
  if (!confirm("Delete pending emails with selected filter?")) return;
  await api("/api/admin/pending/delete", {
    method: "POST",
    body: JSON.stringify({
      mode,
      user_id: userID,
      from_date: fromDate,
      to_date: toDate,
    }),
  });
  state.pendingOffset = 0;
  await loadPending();
  setStatus("Pending emails deleted.");
}

async function api(url, options = {}) {
  return fetchJson(url, { ...options, headers: { ...(options.headers || {}), "X-Admin-Session": state.session } });
}

async function fetchJson(url, options = {}) {
  const res = await fetch(url, {
    ...options,
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
  });
  const text = await res.text();
  if (!res.ok) {
    if (res.status === 401) {
      localStorage.removeItem("adminSessionToken");
      location.href = "/ui/index.html";
    }
    let msg = text || `HTTP ${res.status}`;
    try {
      const obj = text ? JSON.parse(text) : null;
      if (obj && obj.error) msg = obj.error;
    } catch (_) {}
    throw new Error(msg);
  }
  return text ? JSON.parse(text) : null;
}

refreshAll();
