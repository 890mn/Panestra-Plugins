import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { test } from "node:test";
const source = readFileSync(
  new URL("../plugins/netease/timeline.js", import.meta.url),
  "utf8",
);
function player() {
  const playing = {
    resourceTrackId: "one",
    resourceName: "First",
    resourceDuration: 200,
    playId: "one_session",
    playingState: 1,
    isLoadingFirst: false,
    loadingSeekDuration: 0,
  };
  const callbacks = new Map([
    ["audioplayer.onPlayProgress", []],
    [
      "audioplayer.onSeek",
      [
        ([id]) => {
          if (id === playing.playId) playing.loadingSeekDuration = 0;
        },
      ],
    ],
  ]);
  const calls = [],
    dispatches = [];
  const emit = (name, data) => {
    for (const callback of [...callbacks.get(name)]) callback(data);
  };
  const tool = {
    inited: true,
    getStore: () => ({ playing }),
    getDispatch: () => (action) => {
      dispatches.push(action);
    },
  };
  const commands = {
    rxFromNativeEvent: (name) => ({
      subscribe: (callback) => {
        callbacks.get(name).push(callback);
        return {
          unsubscribe: () =>
            callbacks.set(
              name,
              callbacks.get(name).filter((item) => item !== callback),
            ),
        };
      },
    }),
    call: async (...args) => {
      calls.push(args);
      emit("audioplayer.onSeek", [args[1], args[2], 0, args[3]]);
    },
  };
  const require = () => ({ tool });
  require.m = {
    store: "getStore(){ getDispatch(){",
    seek: "setPlayingPosition( AudioPlayer.seek",
  };
  require.c = { commands: { exports: { commands } } };
  const context = vm.createContext({
    window: { __panestraTimelineRequire: require },
    webpackJsonp: [],
    legacyNativeCmder: {
      appendRegisterCall: () => {
        throw Error(
          "Legacy callback registration replaces the modern player registry",
        );
      },
    },
    document: {
      querySelectorAll: () => [
        { min: "0", max: String(playing.resourceDuration), value: "79" },
      ],
    },
    setTimeout,
  });
  vm.runInContext(source, context);
  return {
    playing,
    calls,
    dispatches,
    callbacks,
    commands,
    emit,
    context,
    read: (request = {}) =>
      context.panestraTimeline({
        action: "read",
        title: playing.resourceName,
        ...request,
      }),
  };
}
test("timeline reads preserve native seek cleanup and do not send playback commands", async () => {
  const p = player();
  const before = await p.read();
  assert.equal(before.positionSeconds, 79);
  assert.equal(p.callbacks.get("audioplayer.onSeek").length, 2);
  await p.read();
  assert.equal(p.callbacks.get("audioplayer.onSeek").length, 2);
  p.playing.loadingSeekDuration = 79;
  p.emit("audioplayer.onSeek", [p.playing.playId, "native", 0, 79]);
  assert.equal(p.playing.loadingSeekDuration, 0);
  assert.equal(p.calls.length, 0);
  assert.equal(p.dispatches.length, 0);
  p.playing.resourceTrackId = "two";
  p.playing.resourceName = "Second";
  p.playing.playId = "two_session";
  p.emit("audioplayer.onPlayProgress", [
    p.playing.playId,
    p.playing.loadingSeekDuration,
  ]);
  const next = await p.read();
  assert.equal(next.positionSeconds, 0);
  assert.equal(next.trackId, "two|two_session");
});
test("explicit seek shares callbacks with the player and stale track requests cannot seek the next song", async () => {
  const p = player(),
    before = await p.read();
  p.playing.loadingSeekDuration = 79;
  const after = await p.read({
    action: "seek",
    trackId: before.trackId,
    positionSeconds: 100,
  });
  assert.equal(after.positionSeconds, 100);
  assert.equal(p.playing.loadingSeekDuration, 0);
  assert.equal(p.calls.length, 1);
  assert.equal(p.calls[0][0], "audioplayer.seek");
  assert.equal(p.calls[0][3], 100);
  p.playing.resourceTrackId = "two";
  p.playing.resourceName = "Second";
  p.playing.playId = "two_session";
  p.emit("audioplayer.onPlayProgress", [p.playing.playId, 0]);
  await assert.rejects(
    p.read({ action: "seek", trackId: before.trackId, positionSeconds: 100 }),
    /invalid seek/,
  );
  assert.equal(p.calls.length, 1);
});
test("an unsupported shared event bridge fails without using the conflicting legacy registry", async () => {
  const p = player();
  delete p.commands.rxFromNativeEvent;
  await assert.rejects(p.read(), /unsupported/);
  assert.equal(p.callbacks.get("audioplayer.onSeek").length, 1);
  assert.equal(p.calls.length, 0);
});
test("partial event subscription failure releases its observer", async () => {
  const p = player(),
    original = p.commands.rxFromNativeEvent;
  p.commands.rxFromNativeEvent = (name) => {
    if (name.endsWith("onSeek")) throw Error("subscription unavailable");
    return original(name);
  };
  await assert.rejects(p.read(), /subscription unavailable/);
  assert.equal(p.callbacks.get("audioplayer.onPlayProgress").length, 0);
  assert.equal(p.callbacks.get("audioplayer.onSeek").length, 1);
});
