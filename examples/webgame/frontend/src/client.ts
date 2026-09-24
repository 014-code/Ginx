import { reactive } from "vue";
import { decode, encode, MSG, type Metrics, type Snapshot } from "./protocol";

export const state = reactive({
  status: "offline" as "offline" | "connecting" | "online",
  id: null as number | null,
  room: 0,
  latency: 0,
  sent: 0,
  received: 0,
  error: "",
  snapshot: null as Snapshot | null,
  metrics: null as Metrics | null,
  logs: [] as { key: number; time: string; message: string; kind: string }[],
});
let serial = 0;
function log(message: string, kind = "system") {
  state.logs.unshift({
    key: ++serial,
    time: new Date().toLocaleTimeString("zh-CN", { hour12: false }),
    message,
    kind,
  });
  state.logs.splice(50);
}
let socket: WebSocket | null = null;
let timer: ReturnType<typeof setInterval> | undefined;
let openingTimer: ReturnType<typeof setTimeout> | undefined;
let generation = 0;
let sequence = 0;
let switching = false;
let lastInput = 0;
let metricsPending = false;
let desiredRoom = 1;

function send(id: number, data: unknown) {
  if (!socket || socket.readyState !== WebSocket.OPEN) return false;
  if (socket.bufferedAmount > 128 * 1024) {
    state.error = "发送队列积压，连接已关闭";
    disconnect();
    return false;
  }
  socket.send(encode(id, data));
  state.sent++;
  return true;
}
export async function refreshMetrics() {
  if (metricsPending) return;
  metricsPending = true;
  try {
    const response = await fetch("/api/metrics", {
      signal: AbortSignal.timeout(3000),
    });
    state.metrics = response.ok ? ((await response.json()) as Metrics) : null;
  } catch {
    state.metrics = null;
  } finally {
    metricsPending = false;
  }
}
export function disconnect() {
  generation++;
  clearInterval(timer);
  clearTimeout(openingTimer);
  const previous = socket;
  socket = null;
  previous?.close(1000, "leave demo");
  state.status = "offline";
  state.id = null;
  state.room = 0;
  state.snapshot = null;
  switching = false;
}
export async function connect(name: string, room = 1) {
  disconnect();
  const current = generation;
  state.error = "";
  state.status = "connecting";
  state.sent = 0;
  state.received = 0;
  state.latency = 0;
  sequence = 0;
  desiredRoom = room;
  try {
    const response = await fetch("/api/guest", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ name }),
      signal: AbortSignal.timeout(5000),
    });
    const guest = (await response.json()) as { token: string; error?: string };
    if (current !== generation) return;
    if (!response.ok) throw new Error(guest.error || "访客登录失败");
    const url = new URL("/ws", location.href);
    url.protocol = location.protocol === "https:" ? "wss:" : "ws:";
    const ws = new WebSocket(url);
    socket = ws;
    ws.binaryType = "arraybuffer";
    openingTimer = setTimeout(() => {
      if (current === generation && state.status !== "online") {
        state.error = "连接或鉴权超时";
        disconnect();
      }
    }, 7000);
    ws.onopen = () => {
      if (current !== generation) {
        ws.close();
        return;
      }
      log("WebSocket 已连接 · 正在通过 Ginx 路由鉴权");
      send(MSG.Authenticate, { token: guest.token });
    };
    ws.onmessage = (event: MessageEvent<ArrayBuffer>) => {
      if (current !== generation) return;
      try {
        const packet = decode(event.data);
        state.received++;
        // 数据来自本 Demo 服务；未知业务消息不更新 UI。
        const value = packet.data as Record<string, unknown>;
        if (packet.id === MSG.Authenticate) {
          if (typeof value.id !== "number") throw new Error("鉴权响应无效");
          state.id = value.id;
          log(`鉴权成功 · connection #${value.id}`, "success");
          send(MSG.Join, { room_id: desiredRoom });
          timer = setInterval(() => {
            send(MSG.Heartbeat, { sent_at: Date.now() });
            void refreshMetrics();
          }, 2000);
        } else if (packet.id === MSG.Join) {
          if (state.room !== Number(value.room_id)) state.snapshot = null;
          state.room = Number(value.room_id);
          state.error = "";
          state.status = "online";
          switching = false;
          clearTimeout(openingTimer);
          void refreshMetrics();
          log(`进入房间 0${state.room} · 服务端权威模拟`, "success");
        } else if (packet.id === MSG.Snapshot) {
          const snapshot = packet.data as Snapshot;
          if (snapshot.room_id === state.room) state.snapshot = snapshot;
        } else if (packet.id === MSG.Heartbeat) {
          state.latency = Math.max(0, Date.now() - Number(value.sent_at));
        } else if (packet.id === MSG.Event) {
          log(String(value.message), String(value.kind));
        } else if (packet.id === MSG.Error) {
          state.error = String(value.code);
          log(`请求被拒绝 · ${value.code}`, "error");
          switching = false;
          if (
            value.request_id === MSG.Authenticate ||
            (value.request_id === MSG.Join && state.room === 0) ||
            value.code === "unauthorized"
          )
            disconnect();
        }
      } catch (error) {
        state.error = error instanceof Error ? error.message : "协议解析失败";
        disconnect();
      }
    };
    ws.onclose = () => {
      if (current !== generation) return;
      const unexpected = state.status !== "offline";
      disconnect();
      if (unexpected) {
        state.error ||= "连接已断开，请重新进入";
        log("连接关闭 · 房间由后端清理", "error");
      }
    };
    ws.onerror = () => {
      if (current === generation)
        state.error = "WebSocket 连接失败，请检查后端和 Origin 配置";
    };
  } catch (error) {
    if (current !== generation) return;
    state.error = error instanceof Error ? error.message : "连接失败";
    disconnect();
  }
}
export function join(room: number) {
  if (state.status !== "online" || switching || room === state.room) return;
  input(0, 0, true);
  if (state.status !== "online") return;
  switching = true;
  send(MSG.Join, { room_id: room });
}
export function input(dx: number, dy: number, force = false) {
  if (state.status !== "online" || switching) return;
  const now = performance.now();
  if (!force && now - lastInput < 50) return;
  lastInput = now;
  send(MSG.Input, { dx, dy, seq: ++sequence });
}
