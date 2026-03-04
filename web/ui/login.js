const loginStatus = document.getElementById("loginStatus");
const registerStatus = document.getElementById("registerStatus");

function setLoginStatus(msg, isError = false) {
  loginStatus.textContent = msg;
  loginStatus.style.color = isError ? "#ffb3b3" : "#bfd8ff";
}

function setRegisterStatus(msg, isError = false) {
  registerStatus.textContent = msg;
  registerStatus.style.color = isError ? "#ffb3b3" : "#bfd8ff";
}

(function clearOldSessions() {
  // Force explicit login from this page.
  localStorage.removeItem("adminSessionToken");
  localStorage.removeItem("userToken");
})();

document.getElementById("loginForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const username = document.getElementById("username").value.trim();
  const password = document.getElementById("password").value;

  try {
    const adminRes = await fetch("/api/admin/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });

    if (adminRes.ok) {
      const out = await adminRes.json();
      localStorage.setItem("adminSessionToken", out.token);
      setLoginStatus("Admin login successful. Redirecting...");
      location.href = "/ui/admin.html";
      return;
    }

    const userRes = await fetch("/api/user/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });

    if (userRes.ok) {
      const out = await userRes.json();
      localStorage.setItem("userToken", out.token);
      setLoginStatus("User login successful. Redirecting...");
      location.href = "/ui/user.html";
      return;
    }

    const errText = await userRes.text();
    throw new Error(errText || "Invalid credentials");
  } catch (err) {
    setLoginStatus(err.message, true);
  }
});

document.getElementById("registerForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const payload = {
    username: document.getElementById("regUsername").value.trim(),
    password: document.getElementById("regPassword").value,
    display_name: document.getElementById("regDisplayName").value.trim(),
  };
  try {
    const res = await fetch("/api/user/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(payload),
    });
    const txt = await res.text();
    if (!res.ok) {
      throw new Error(txt || `HTTP ${res.status}`);
    }
    const out = txt ? JSON.parse(txt) : {};
    setRegisterStatus(`Registration successful. Package: ${out.package || "default"}`);
  } catch (err) {
    setRegisterStatus(err.message, true);
  }
});