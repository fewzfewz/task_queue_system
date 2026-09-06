sed -i '/async function login(){/i \
    window.addEventListener("DOMContentLoaded", async function() {\
      try{\
        const res = await fetch("/api/v1/session");\
        if (res.ok) {\
          const data = await res.json();\
          window.location.href = (data && data.role === "viewer") ? "/" : "/ui";\
        }\
      }catch(e){}\
    });\
' internal/api/handler/ui_handler.go
