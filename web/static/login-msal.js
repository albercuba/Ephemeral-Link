(function () {
  const button = document.getElementById("login-microsoft-button");
  const csrf = document.getElementById("login-csrf")?.value || "";
  const errorBox = document.getElementById("login-error-alert");

  function showError(message) {
    if (errorBox) {
      errorBox.textContent = message;
      errorBox.hidden = false;
      return;
    }
    alert(message);
  }

  async function loadConfig() {
    const response = await fetch("/auth/microsoft/config", {
      credentials: "same-origin",
    });
    if (!response.ok) {
      throw new Error("Microsoft login configuration could not be loaded.");
    }
    return response.json();
  }

  function loadScript(src) {
    return new Promise(function (resolve, reject) {
      const existing = document.querySelector('script[src="' + src + '"]');
      if (existing) {
        if (window.msal) {
          resolve();
          return;
        }
        existing.addEventListener("load", resolve, { once: true });
        existing.addEventListener("error", reject, { once: true });
        return;
      }
      const script = document.createElement("script");
      script.src = src;
      script.async = true;
      script.onload = resolve;
      script.onerror = reject;
      document.head.appendChild(script);
    });
  }

  async function ensureMsal() {
    if (window.msal) {
      return;
    }
    try {
      await loadScript(
        "https://cdn.jsdelivr.net/npm/@azure/msal-browser@2.38.3/lib/msal-browser.min.js",
      );
    } catch (_) {
      // The explicit check below produces the operator-facing error.
    }
    if (!window.msal) {
      throw new Error(
        "Microsoft login library could not be loaded. Check that the browser can access alcdn.msauth.net or cdn.jsdelivr.net.",
      );
    }
  }

  function createMsalClient(config) {
    return new window.msal.PublicClientApplication({
      auth: {
        clientId: config.clientId,
        authority:
          config.authorityUrl ||
          "https://login.microsoftonline.com/" + config.tenantId,
        redirectUri: window.location.origin + "/login",
        navigateToLoginRequestUrl: false,
      },
      cache: { cacheLocation: "sessionStorage" },
    });
  }

  function clearMsalSession() {
    try {
      for (let index = sessionStorage.length - 1; index >= 0; index -= 1) {
        const key = sessionStorage.key(index) || "";
        if (key.toLowerCase().includes("msal")) {
          sessionStorage.removeItem(key);
        }
      }
    } catch (_) {
      // Ignore storage access errors and fall back to prompt=select_account on the next login attempt.
    }
  }

  async function completeLogin(accessToken) {
    const body = new URLSearchParams();
    body.set("csrf", csrf);
    body.set("token", accessToken);
    const response = await fetch("/auth/microsoft", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body,
    });
    if (!response.ok) {
      clearMsalSession();
      const text = await response.text();
      throw new Error(
        text
          .replace(/<[^>]+>/g, " ")
          .replace(/\s+/g, " ")
          .trim() || "Microsoft login failed.",
      );
    }
    window.location.assign("/");
  }

  async function handleRedirect() {
    await ensureMsal();
    const config = await loadConfig();
    if (
      !config.enabled ||
      !config.clientId ||
      !config.tenantId ||
      !config.scope
    ) {
      return;
    }
    const client = createMsalClient(config);
    const result = await client.handleRedirectPromise();
    if (result && result.accessToken) {
      await completeLogin(result.accessToken);
    }
  }

  handleRedirect().catch(function (error) {
    showError(error.message || "Microsoft login failed.");
  });

  if (!button) {
    return;
  }

  button.addEventListener("click", async function (event) {
    event.preventDefault();
    try {
      await ensureMsal();
      const config = await loadConfig();
      if (
        !config.enabled ||
        !config.clientId ||
        !config.tenantId ||
        !config.scope
      ) {
        throw new Error("Microsoft login is not fully configured.");
      }
      const client = createMsalClient(config);
      await client.loginRedirect({
        scopes: [config.scope],
        prompt: "select_account",
      });
    } catch (error) {
      showError(error.message || "Microsoft login failed.");
    }
  });
})();
