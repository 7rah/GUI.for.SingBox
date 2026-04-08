(function () {
  if (window.WailsInvoke) {
    return;
  }

  window.__GUI_FOR_CORES_WEB__ = true;

  const listeners = new Map();
  const eventSource = new EventSource("/__web/events/stream");

  const dispatchEvent = (eventName, args) => {
    const entries = listeners.get(eventName);
    if (!entries || entries.length === 0) {
      return;
    }

    const remaining = [];

    entries.forEach((entry) => {
      try {
        entry.callback(...args);
      } catch (error) {
        console.error("[wails-shim] listener failed:", eventName, error);
      }

      entry.count += 1;
      if (entry.maxCallbacks < 0 || entry.count < entry.maxCallbacks) {
        remaining.push(entry);
      }
    });

    if (remaining.length > 0) {
      listeners.set(eventName, remaining);
    } else {
      listeners.delete(eventName);
    }
  };

  const onEvent = (eventName, callback, maxCallbacks) => {
    const entry = {
      callback,
      count: 0,
      maxCallbacks,
    };

    const entries = listeners.get(eventName) || [];
    entries.push(entry);
    listeners.set(eventName, entries);

    return () => {
      const currentEntries = listeners.get(eventName) || [];
      const filteredEntries = currentEntries.filter((item) => item !== entry);
      if (filteredEntries.length > 0) {
        listeners.set(eventName, filteredEntries);
      } else {
        listeners.delete(eventName);
      }
    };
  };

  const emitEvent = (eventName, args) => {
    dispatchEvent(eventName, args);

    fetch("/__web/events/emit", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        event: eventName,
        args,
      }),
      keepalive: true,
    }).catch((error) => {
      console.error("[wails-shim] emit failed:", eventName, error);
    });
  };

  eventSource.onmessage = (event) => {
    if (!event.data) {
      return;
    }

    try {
      const payload = JSON.parse(event.data);
      dispatchEvent(payload.event, payload.args || []);
    } catch (error) {
      console.error("[wails-shim] bad event payload:", error);
    }
  };

  eventSource.onerror = () => {
    console.warn(
      "[wails-shim] event stream disconnected, waiting for reconnect...",
    );
  };

  const rpc = async (method, args) => {
    const response = await fetch(`/__web/rpc/${encodeURIComponent(method)}`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({ args }),
    });

    if (!response.ok) {
      const message = await response.text();
      throw new Error(message || `RPC failed: ${method}`);
    }

    const text = await response.text();
    return text ? JSON.parse(text) : null;
  };

  const requestNotificationPermission = async () => {
    if (typeof Notification === "undefined") {
      return "denied";
    }
    return Notification.requestPermission();
  };

  const sendNotification = async (options) => {
    if (typeof Notification === "undefined") {
      return false;
    }

    if (Notification.permission !== "granted") {
      await requestNotificationPermission();
    }

    if (Notification.permission !== "granted") {
      return false;
    }

    new Notification(options.title || "", {
      body: options.body || "",
      icon: options.icon,
    });

    return true;
  };

  window.WailsInvoke = function () {};

  window.go = window.go || {};
  window.go.bridge = window.go.bridge || {};
  window.go.bridge.App = new Proxy(
    {},
    {
      get(_, method) {
        if (typeof method !== "string") {
          return undefined;
        }
        return (...args) => rpc(method, args);
      },
    },
  );

  window.runtime = {
    LogPrint: (...args) => console.log(...args),
    LogTrace: (...args) => console.debug(...args),
    LogDebug: (...args) => console.debug(...args),
    LogInfo: (...args) => console.info(...args),
    LogWarning: (...args) => console.warn(...args),
    LogError: (...args) => console.error(...args),
    LogFatal: (...args) => console.error(...args),
    EventsOnMultiple: (eventName, callback, maxCallbacks) =>
      onEvent(eventName, callback, maxCallbacks),
    EventsOn: (eventName, callback) => onEvent(eventName, callback, -1),
    EventsOnce: (eventName, callback) => onEvent(eventName, callback, 1),
    EventsOff: (eventName, ...additionalEventNames) => {
      [eventName, ...additionalEventNames].forEach((name) =>
        listeners.delete(name),
      );
    },
    EventsOffAll: () => listeners.clear(),
    EventsEmit: (eventName, ...args) => emitEvent(eventName, args),
    WindowReload: () => window.location.reload(),
    WindowReloadApp: () => window.location.reload(),
    WindowSetAlwaysOnTop: () => {},
    WindowSetSystemDefaultTheme: () => {},
    WindowSetLightTheme: () => {
      document.body?.setAttribute("theme-mode", "light");
    },
    WindowSetDarkTheme: () => {
      document.body?.setAttribute("theme-mode", "dark");
    },
    WindowCenter: () => {},
    WindowSetTitle: (title) => {
      document.title = title;
    },
    WindowFullscreen: () => document.documentElement.requestFullscreen?.(),
    WindowUnfullscreen: () => document.exitFullscreen?.(),
    WindowIsFullscreen: () => Boolean(document.fullscreenElement),
    WindowGetSize: () => [window.innerWidth, window.innerHeight],
    WindowSetSize: () => {},
    WindowSetMaxSize: () => {},
    WindowSetMinSize: () => {},
    WindowSetPosition: () => {},
    WindowGetPosition: () => [window.screenX || 0, window.screenY || 0],
    WindowHide: () => {},
    WindowShow: () => {},
    WindowMaximise: () => {},
    WindowToggleMaximise: () => {},
    WindowUnmaximise: () => {},
    WindowIsMaximised: async () => false,
    WindowMinimise: () => {},
    WindowUnminimise: () => {},
    WindowSetBackgroundColour: (r, g, b, a) => {
      document.body?.style.setProperty(
        "background-color",
        `rgba(${r}, ${g}, ${b}, ${a})`,
      );
    },
    ScreenGetAll: () => [
      {
        id: "primary",
        width: window.screen.width,
        height: window.screen.height,
      },
    ],
    WindowIsMinimised: async () => false,
    WindowIsNormal: async () => true,
    BrowserOpenURL: (url) => {
      const opened = window.open(url, "_blank", "noopener,noreferrer");
      if (!opened) {
        window.location.href = url;
      }
    },
    Environment: () => ({
      platform: navigator.platform,
      userAgent: navigator.userAgent,
    }),
    Quit: () => {},
    Hide: () => {},
    Show: () => {},
    ClipboardGetText: async () => {
      try {
        return await navigator.clipboard.readText();
      } catch {
        return "";
      }
    },
    ClipboardSetText: async (text) => {
      try {
        await navigator.clipboard.writeText(text);
        return true;
      } catch {
        return false;
      }
    },
    OnFileDrop: () => () => {},
    OnFileDropOff: () => {},
    CanResolveFilePaths: () => false,
    ResolveFilePaths: (files) => files,
    InitializeNotifications: async () => {},
    CleanupNotifications: async () => {},
    IsNotificationAvailable: async () => typeof Notification !== "undefined",
    RequestNotificationAuthorization: requestNotificationPermission,
    CheckNotificationAuthorization: async () =>
      typeof Notification === "undefined" ? "denied" : Notification.permission,
    SendNotification: sendNotification,
    SendNotificationWithActions: sendNotification,
    RegisterNotificationCategory: async () => {},
    RemoveNotificationCategory: async () => {},
    RemoveAllPendingNotifications: async () => {},
    RemovePendingNotification: async () => {},
    RemoveAllDeliveredNotifications: async () => {},
    RemoveDeliveredNotification: async () => {},
    RemoveNotification: async () => {},
  };
})();
