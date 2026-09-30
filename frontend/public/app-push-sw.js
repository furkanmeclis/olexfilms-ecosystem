/* global self, clients */
self.addEventListener("push", (event) => {
  let title = "Notification";
  let body = "";
  let data = {};
  try {
    const payload = event.data ? event.data.json() : {};
    title = payload.title || title;
    body = payload.body || "";
    data = payload.data || {};
  } catch {
    body = event.data ? event.data.text() : "";
  }
  event.waitUntil(
    self.registration.showNotification(title, {
      body,
      data,
    }),
  );
});

function resolveNotificationUrl(data) {
  if (
    data &&
    typeof data.action_url === "string" &&
    data.action_url.startsWith("/")
  ) {
    return data.action_url;
  }
  return "/platform/notifications";
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const targetUrl = resolveNotificationUrl(event.notification.data);
  event.waitUntil(
    clients
      .matchAll({ type: "window", includeUncontrolled: true })
      .then((windowClients) => {
        for (const client of windowClients) {
          if ("focus" in client) {
            return client.focus();
          }
        }
        if (clients.openWindow) {
          return clients.openWindow(targetUrl);
        }
        return undefined;
      }),
  );
});
