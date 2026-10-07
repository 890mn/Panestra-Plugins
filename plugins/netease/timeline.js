// Fixed player operations only; no account, credentials, queue or arbitrary actions
async function panestraTimeline(request) {
  if (typeof webpackJsonp !== "object") throw new Error("unsupported");
  if (!window.__panestraTimelineRequire) {
    webpackJsonp.push([
      [987662],
      {
        987662: function (module, exports, require) {
          window.__panestraTimelineRequire = require;
        },
      },
      [[987662]],
    ]);
  }
  const require = window.__panestraTimelineRequire;
  let bridge = window.__panestraTimelineBridge;
  if (!bridge || bridge.version !== 3) {
    let tool;
    for (const [id, factory] of Object.entries(require.m || {})) {
      const source = String(factory);
      if (!source.includes("getStore(){") || !source.includes("getDispatch(){"))
        continue;
      const exports = require(id);
      tool = Object.values(exports).find(
        (value) =>
          typeof value?.getStore === "function" &&
          typeof value?.getDispatch === "function" &&
          value.inited,
      );
      if (tool) break;
    }
    const supportsSeek = Object.values(require.m || {}).some((factory) => {
      const source = String(factory);
      return (
        source.includes("setPlayingPosition(") &&
        source.includes("AudioPlayer.seek")
      );
    });
    // Use the player's own shared event bridge. The legacy adapter owns a separate
    // callback registry and can replace modern native callbacks, including seek cleanup.
    const commands = Object.values(require.c || {})
      .flatMap((module) => Object.values(module.exports || {}))
      .find(
        (value) =>
          typeof value?.rxFromNativeEvent === "function" &&
          typeof value.call === "function",
      );
    if (!tool || !supportsSeek || !commands) throw new Error("unsupported");
    for (const subscription of bridge?.subscriptions || [])
      subscription.unsubscribe();
    bridge = { tool, commands, event: null, subscriptions: [], version: 3 };
    const observe = (name, handler) => {
      const subscription = commands.rxFromNativeEvent(name).subscribe(handler);
      if (typeof subscription?.unsubscribe !== "function")
        throw new Error("unsupported");
      bridge.subscriptions.push(subscription);
    };
    try {
      observe("audioplayer.onPlayProgress", ([playId, position]) => {
        if (typeof playId === "string" && Number.isFinite(position))
          bridge.event = { playId, position, at: Date.now() };
      });
      observe("audioplayer.onSeek", ([playId, seekId, code, position]) => {
        if (
          code === 0 &&
          typeof playId === "string" &&
          Number.isFinite(position)
        )
          bridge.event = { playId, position, at: Date.now() };
      });
    } catch (error) {
      for (const subscription of bridge.subscriptions)
        subscription.unsubscribe();
      throw error;
    }
    window.__panestraTimelineBridge = bridge;
  }
  const snapshot = () => {
    const playing = bridge.tool.getStore().playing;
    if (
      !playing ||
      playing.isPlayingVideo ||
      !playing.resourceTrackId ||
      playing.resourceName !== request.title
    )
      throw new Error("changed");
    const duration = playing.resourceDuration;
    if (!Number.isFinite(duration) || duration <= 0 || duration > 604800)
      throw new Error("no timeline");
    const sliders = Array.from(
      document.querySelectorAll(
        'input[type="range"][aria-orientation="horizontal"]',
      ),
    ).filter(
      (el) =>
        Number(el.min) === 0 && Math.abs(Number(el.max) - duration) < 0.01,
    );
    const event = bridge.event;
    let position =
      event?.playId === playing.playId &&
      (playing.playingState !== 2 || Date.now() - event.at < 4000)
        ? event.position
        : null;
    if (position == null && playing.playingState !== 2 && sliders.length === 1)
      position = Number(sliders[0].value);
    if (!Number.isFinite(position) || position < 0 || position > duration)
      throw new Error("no position");
    return {
      title: playing.resourceName,
      trackId: `${playing.resourceTrackId}|${playing.playId || "restore"}`,
      positionSeconds: position,
      durationSeconds: duration,
      seek: !playing.isLoadingFirst,
    };
  };
  if (!bridge.event && bridge.tool.getStore().playing.playingState === 2) {
    const deadline = Date.now() + 1600;
    while (!bridge.event && Date.now() < deadline)
      await new Promise((resolve) => setTimeout(resolve, 80));
  }
  const before = snapshot();
  if (request.action === "read") return before;
  if (
    request.action !== "seek" ||
    !before.seek ||
    request.trackId !== before.trackId ||
    !Number.isFinite(request.positionSeconds) ||
    request.positionSeconds < 0 ||
    request.positionSeconds > before.durationSeconds
  )
    throw new Error("invalid seek");
  const playing = bridge.tool.getStore().playing;
  let failed = false;
  const operation = playing.restoreResource
    ? bridge.tool.getDispatch()({
        type: "playing/setPlayingPosition",
        payload: { duration: request.positionSeconds },
      })
    : bridge.commands.call(
        "audioplayer.seek",
        playing.playId,
        `panestra|seek|${Date.now()}`,
        request.positionSeconds,
      );
  Promise.resolve(operation).catch(() => {
    failed = true;
  });
  const started = Date.now();
  while (Date.now() - started < 2500) {
    if (failed) throw new Error("seek rejected");
    const after = snapshot();
    if (after.trackId !== before.trackId) throw new Error("changed");
    if (Math.abs(after.positionSeconds - request.positionSeconds) < 2)
      return after;
    await new Promise((resolve) => setTimeout(resolve, 100));
  }
  throw new Error("seek unconfirmed");
}
