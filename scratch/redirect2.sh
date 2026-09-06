sed -i '/function toggleVis()/i \
  window.addEventListener("DOMContentLoaded", async function() {\
    const key = localStorage.getItem("tq_api_key");\
    if (key) {\
      try {\
        const res = await fetch("/api/v1/client/me", { headers: {"X-API-Key": key} });\
        if (res.ok) { window.location.href = "/client/dashboard"; }\
        else { localStorage.removeItem("tq_api_key"); }\
      } catch(e) {}\
    }\
  });\
' internal/api/handler/client_ui_handler.go
