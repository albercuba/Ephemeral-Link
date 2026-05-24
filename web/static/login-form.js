document.addEventListener("DOMContentLoaded", function () {
  const form = document.getElementById("login-form");
  const username = document.getElementById("login-username-input");
  const password = document.getElementById("login-password-input");

  if (!form || !username || !password) {
    return;
  }

  if (form.dataset.clearOnError === "true") {
    username.value = "";
    password.value = "";
    username.focus();
  }

  form.addEventListener("submit", function () {
    // Avoid leaving a failed password value available through browser page restore.
    window.setTimeout(function () {
      password.value = "";
    }, 0);
  });

  window.addEventListener("pageshow", function (event) {
    if (event.persisted && form.dataset.clearOnError === "true") {
      username.value = "";
      password.value = "";
    }
  });
});
