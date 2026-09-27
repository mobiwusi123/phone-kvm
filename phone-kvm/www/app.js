(function () {
  "use strict";

  // 消息类型必须和 server.go 里的常量保持一致
  const C = { MOVE: 1, BUTTON: 2, WHEEL: 3, KEY: 4, TEXT: 5, PING: 6, CENTER: 8 };
  const S = { PONG: 7, CURSOR: 10 };

  const VK = {
    CTRL: 17, ALT: 18, SHIFT: 16, WIN: 91, SPACE: 32,
    ENTER: 13, ESC: 27, TAB: 9, BACK: 8, DEL: 46, INSERT: 45,
    LEFT: 37, UP: 38, RIGHT: 39, DOWN: 40,
    HOME: 36, END: 35, PGUP: 33, PGDN: 34,
    F1: 112, F2: 113, F3: 114, F4: 115, F5: 116, F6: 117,
    F7: 118, F8: 119, F9: 120, F10: 121, F11: 122, F12: 123
  };

  const NAME2VK = {
    Enter: VK.ENTER, Escape: VK.ESC, Tab: VK.TAB, Backspace: VK.BACK, Delete: VK.DEL,
    Insert: VK.INSERT, " ": VK.SPACE,
    ArrowLeft: VK.LEFT, ArrowUp: VK.UP, ArrowRight: VK.RIGHT, ArrowDown: VK.DOWN,
    Home: VK.HOME, End: VK.END, PageUp: VK.PGUP, PageDown: VK.PGDN,
    F1: VK.F1, F2: VK.F2, F3: VK.F3, F4: VK.F4, F5: VK.F5, F6: VK.F6,
    F7: VK.F7, F8: VK.F8, F9: VK.F9, F10: VK.F10, F11: VK.F11, F12: VK.F12
  };

  const $ = (sel) => document.querySelector(sel);
  const pad = $("#pad");
  const hint = $("#hint");
  const panel = $("#panel");
  const settings = $("#settings");
  const ime = $("#ime");
  const statusEl = $("#status");
  const statusText = $("#statusText");
  const rttEl = $("#rtt");

  const token = location.pathname.split("/").filter(Boolean)[0] || "";
  const wsURL = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/" + token + "/ws";

  // ------------------------------------------------------------ 本地设置

  const store = {
    get(k, def) {
      try {
        const v = localStorage.getItem("phonekvm." + k);
        return v === null ? def : v;
      } catch (e) {
        return def;
      }
    },
    set(k, v) {
      try { localStorage.setItem("phonekvm." + k, String(v)); } catch (e) { /* 忽略 */ }
    }
  };

  let sens = parseFloat(store.get("sens", "1")) || 1;
  let scrollGain = parseFloat(store.get("scroll", "1.4")) || 1.4;
  let natural = store.get("natural", "1") === "1";

  // ------------------------------------------------------------ 发送封装

  let ws = null;
  let connected = false;
  let retry = 0;
  let retryTimer = null;
  let seq = 0;
  let pingAt = 0;

  function send(bytes) {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(bytes);
  }

  function sendMove(dx, dy) {
    const b = new Uint8Array(5);
    const v = new DataView(b.buffer);
    b[0] = C.MOVE;
    v.setInt16(1, dx, true);
    v.setInt16(3, dy, true);
    send(b);
  }

  function sendButton(btn, down) {
    send(new Uint8Array([C.BUTTON, btn, down ? 1 : 0]));
  }

  function sendWheel(dv, dh) {
    const b = new Uint8Array(5);
    const v = new DataView(b.buffer);
    b[0] = C.WHEEL;
    v.setInt16(1, dv, true);
    v.setInt16(3, dh, true);
    send(b);
  }

  function sendKey(vk, down) {
    const b = new Uint8Array(4);
    const v = new DataView(b.buffer);
    b[0] = C.KEY;
    v.setUint16(1, vk, true);
    b[3] = down ? 1 : 0;
    send(b);
  }

  function sendText(str) {
    if (!str) return;
    const u = new TextEncoder().encode(str);
    if (u.length > 60000) return;
    const b = new Uint8Array(3 + u.length);
    const v = new DataView(b.buffer);
    b[0] = C.TEXT;
    v.setUint16(1, u.length, true);
    b.set(u, 3);
    send(b);
  }

  // -------------------------------------------------------- 粘滞修饰键

  const MOD_ORDER = [VK.CTRL, VK.ALT, VK.SHIFT, VK.WIN];
  const sticky = new Set();

  function syncMods() {
    document.querySelectorAll(".key.mod").forEach((b) => {
      b.classList.toggle("active", sticky.has(Number(b.dataset.vk)));
    });
  }

  // 粘滞修饰键：点一下 Ctrl 点亮，再点别的键就发出 Ctrl+X，随后 Ctrl 自动弹起。
  function pressWithMods(vk) {
    const active = MOD_ORDER.filter((m) => sticky.has(m));
    for (const m of active) sendKey(m, true);
    sendKey(vk, true);
    sendKey(vk, false);
    for (const m of active.slice().reverse()) sendKey(m, false);
    if (active.length) {
      for (const m of active) sticky.delete(m);
      syncMods();
    }
  }

  function charVk(ch) {
    const c = ch.toLowerCase();
    if (c >= "a" && c <= "z") return c.charCodeAt(0) - 32;
    if (c >= "0" && c <= "9") return c.charCodeAt(0);
    return 0;
  }

  // ------------------------------------------------------------- 触摸板

  let mode = null;
  let dragging = false;
  let lastX = 0, lastY = 0, scrollX = 0, scrollY = 0;
  let accX = 0, accY = 0, accW = 0, accH = 0;
  let downAt = 0, movedDist = 0, twoFingers = false, lastTapEnd = 0;

  pad.addEventListener("touchstart", (e) => {
    if (e.target.closest("button")) return;
    e.preventDefault();
    hint.style.opacity = "0.18";
    const now = Date.now();
    if (e.touches.length === 1) {
      const t = e.touches[0];
      if (!dragging && now - lastTapEnd < 320) {
        // 双击后不抬手 = 按住左键拖拽（笔记本触摸板的老习惯）
        dragging = true;
        sendButton(1, true);
      }
      mode = "move";
      lastX = t.clientX;
      lastY = t.clientY;
      downAt = now;
      movedDist = 0;
      twoFingers = false;
    } else if (e.touches.length === 2) {
      twoFingers = true;
      if (dragging) {
        dragging = false;
        sendButton(1, false);
      }
      mode = "scroll";
      scrollX = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      scrollY = (e.touches[0].clientY + e.touches[1].clientY) / 2;
    }
  }, { passive: false });

  pad.addEventListener("touchmove", (e) => {
    if (e.target.closest("button")) return;
    e.preventDefault();
    if (mode === "move" && e.touches.length === 1) {
      const t = e.touches[0];
      const dx = t.clientX - lastX;
      const dy = t.clientY - lastY;
      accX += dx;
      accY += dy;
      movedDist += Math.abs(dx) + Math.abs(dy);
      lastX = t.clientX;
      lastY = t.clientY;
    } else if (mode === "scroll" && e.touches.length === 2) {
      const cx = (e.touches[0].clientX + e.touches[1].clientX) / 2;
      const cy = (e.touches[0].clientY + e.touches[1].clientY) / 2;
      const dx = cx - scrollX;
      const dy = cy - scrollY;
      // 160 像素的手指滑动 ≈ 1 格滚轮（1 格 = 120 单位），这里换算成 1/120 格
      accW += (dy / 160) * 120 * scrollGain;
      accH += (dx / 160) * 120 * scrollGain;
      movedDist += Math.abs(dx) + Math.abs(dy);
      scrollX = cx;
      scrollY = cy;
    }
  }, { passive: false });

  function endTouch(e) {
    if (e.touches.length > 0) return;
    e.preventDefault();
    const dur = Date.now() - downAt;
    if (dragging) {
      dragging = false;
      sendButton(1, false);
    } else if (mode && movedDist < 14 && dur < 280) {
      if (twoFingers) {
        sendButton(2, true);
        sendButton(2, false);
      } else {
        sendButton(1, true);
        sendButton(1, false);
        lastTapEnd = Date.now();
      }
    }
    mode = null;
  }

  pad.addEventListener("touchend", endTouch, { passive: false });
  pad.addEventListener("touchcancel", endTouch, { passive: false });
  pad.addEventListener("contextmenu", (e) => e.preventDefault());

  // 每帧把累积的位移/滚轮整发一次，避免每个触摸事件都单独发包
  function flush() {
    if (accX || accY) {
      const mx = accX * sens;
      const my = accY * sens;
      const ix = Math.trunc(mx);
      const iy = Math.trunc(my);
      accX = (mx - ix) / sens;
      accY = (my - iy) / sens;
      if (ix || iy) sendMove(ix, iy);
    }
    if (accW || accH) {
      const iw = Math.trunc(accW);
      const ih = Math.trunc(accH);
      accW -= iw;
      accH -= ih;
      if (iw || ih) sendWheel(natural ? iw : -iw, natural ? ih : -ih);
    }
    requestAnimationFrame(flush);
  }
  requestAnimationFrame(flush);

  // ------------------------------------------------------- 按键 / 输入法

  document.querySelectorAll("button[data-vk]").forEach((btn) => {
    const vk = Number(btn.dataset.vk);
    const isMod = btn.classList.contains("mod");
    btn.addEventListener("click", (e) => {
      e.preventDefault();
      if (isMod) {
        if (sticky.has(vk)) sticky.delete(vk);
        else sticky.add(vk);
        syncMods();
      } else {
        pressWithMods(vk);
      }
    });
  });

  let composing = false;

  ime.addEventListener("keydown", (e) => {
    if (e.isComposing || composing || e.keyCode === 229) return;
    if (e.key === "Unidentified" || e.key === "Process") return;
    const vk = NAME2VK[e.key];
    if (vk) {
      e.preventDefault();
      pressWithMods(vk);
      return;
    }
    if (e.ctrlKey || e.altKey || e.metaKey) return;
    if (e.key.length === 1) {
      e.preventDefault();
      if (sticky.size) {
        const v = charVk(e.key);
        if (v) {
          pressWithMods(v);
          return;
        }
      }
      sendText(e.key);
    }
  });

  ime.addEventListener("compositionstart", () => { composing = true; });
  ime.addEventListener("compositionend", (e) => {
    composing = false;
    ime.value = "";
    // 中文在这里才上屏：组字过程中绝不把拼音字母发出去
    if (e.data) sendText(e.data);
  });

  // 语音输入、粘贴这类没有 keydown 的输入走这里兜底
  ime.addEventListener("input", (e) => {
    if (composing || e.isComposing) return;
    if (e.inputType === "insertCompositionText") return;
    const v = ime.value;
    if (!v) return;
    ime.value = "";
    sendText(v);
  });

  // ------------------------------------------------------------ 界面开关

  $("#btnKeyboard").addEventListener("click", () => {
    panel.hidden = !panel.hidden;
    $("#btnKeyboard").classList.toggle("active", !panel.hidden);
    if (panel.hidden) ime.blur();
    else ime.focus();
  });

  $("#btnMore").addEventListener("click", () => {
    settings.hidden = !settings.hidden;
    $("#btnMore").classList.toggle("active", !settings.hidden);
  });

  $("#btnCenter").addEventListener("click", () => send(new Uint8Array([C.CENTER])));

  $("#btnFull").addEventListener("click", () => {
    const el = document.documentElement;
    if (document.fullscreenElement) document.exitFullscreen();
    else if (el.requestFullscreen) el.requestFullscreen().catch(() => {});
  });

  const sensRange = $("#sens");
  const scrollRange = $("#scroll");
  sensRange.value = sens;
  scrollRange.value = scrollGain;
  $("#sensVal").textContent = sens.toFixed(2);
  $("#scrollVal").textContent = scrollGain.toFixed(1);
  $("#natural").checked = natural;

  sensRange.addEventListener("input", () => {
    sens = parseFloat(sensRange.value);
    $("#sensVal").textContent = sens.toFixed(2);
    store.set("sens", sens);
  });
  scrollRange.addEventListener("input", () => {
    scrollGain = parseFloat(scrollRange.value);
    $("#scrollVal").textContent = scrollGain.toFixed(1);
    store.set("scroll", scrollGain);
  });
  $("#natural").addEventListener("change", (e) => {
    natural = e.target.checked;
    store.set("natural", natural ? "1" : "0");
  });

  let wantWake = false;
  let wakeLock = null;
  async function acquireWake() {
    if (!wantWake || wakeLock || !("wakeLock" in navigator)) return;
    try {
      wakeLock = await navigator.wakeLock.request("screen");
      wakeLock.addEventListener("release", () => { wakeLock = null; });
    } catch (err) {
      wantWake = false;
      $("#wake").checked = false;
      $("#tips").textContent = "这个页面是 HTTP 的（局域网 IP 不算安全来源），浏览器不允许保持屏幕常亮。" +
        "建议把手机「设置 → 显示 → 息屏时间」调到 10 分钟。";
    }
  }
  $("#wake").addEventListener("change", (e) => {
    wantWake = e.target.checked;
    if (!wantWake) {
      if (wakeLock) { wakeLock.release(); wakeLock = null; }
      return;
    }
    acquireWake();
  });

  // ------------------------------------------------------------ 连接管理

  function setStatus(on, text) {
    statusEl.className = on ? "on" : "off";
    statusText.textContent = text;
  }

  function connect() {
    clearTimeout(retryTimer);
    setStatus(false, "连接中…");
    try {
      ws = new WebSocket(wsURL);
    } catch (err) {
      scheduleReconnect();
      return;
    }
    ws.binaryType = "arraybuffer";
    ws.onopen = () => {
      connected = true;
      retry = 0;
      setStatus(true, "已连接");
      acquireWake();
    };
    ws.onmessage = (ev) => handleMessage(ev.data);
    ws.onclose = () => {
      connected = false;
      setStatus(false, "已断开，重连中…");
      scheduleReconnect();
    };
    ws.onerror = () => { /* onclose 随后会触发 */ };
  }

  function scheduleReconnect() {
    clearTimeout(retryTimer);
    retry = Math.min(retry + 1, 8);
    retryTimer = setTimeout(connect, Math.min(400 * retry, 4000));
  }

  function handleMessage(data) {
    if (typeof data === "string") {
      try {
        const m = JSON.parse(data);
        if (m.t === "bye") setStatus(false, "电脑端已退出");
      } catch (err) { /* 忽略无法解析的消息 */ }
      return;
    }
    const b = new Uint8Array(data);
    if (b[0] === S.PONG && b.length >= 5) {
      const rtt = Math.round(performance.now() - pingAt);
      rttEl.textContent = rtt + " ms";
    }
  }

  setInterval(() => {
    if (!connected) return;
    seq++;
    pingAt = performance.now();
    const b = new Uint8Array(5);
    const v = new DataView(b.buffer);
    b[0] = C.PING;
    v.setUint32(1, seq, true);
    send(b);
  }, 2000);

  document.addEventListener("visibilitychange", () => {
    if (document.visibilityState !== "visible") return;
    if (!connected) {
      retry = 0;
      connect();
    }
    acquireWake();
  });

  connect();
})();
