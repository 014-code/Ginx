<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import GameCanvas from "./GameCanvas.vue";
import {
  connect,
  disconnect,
  input,
  join,
  refreshMetrics,
  state,
} from "./client";
import { controls } from "./game";

const name = ref("Explorer-" + Math.floor(100 + Math.random() * 900));
const chosenRoom = ref(1);
const activeRoom = computed(() => state.room || chosenRoom.value);
watch(
  () => state.room,
  (room) => {
    if (room > 0) chosenRoom.value = room;
  },
);
const me = computed(() =>
  state.snapshot?.players.find((p) => p.id === state.id),
);
const leaderboard = computed(() =>
  [...(state.snapshot?.players ?? [])].sort(
    (a, b) => b.score - a.score || a.id - b.id,
  ),
);
const statusLabel = computed(
  () =>
    ({ online: "已连接", offline: "未连接", connecting: "连接中" })[
      state.status
    ],
);
const bytes = (n: number) =>
  n >= 1024 ? `${(n / 1024).toFixed(1)} KB` : `${n} B`;
function move(x: number, y: number, e: PointerEvent) {
  e.preventDefault();
  (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
  controls.x = x;
  controls.y = y;
}
function stopMove() {
  controls.x = 0;
  controls.y = 0;
  input(0, 0, true);
}
function switchRoom(room: number) {
  if (state.status === "online") join(room);
  else if (state.status === "offline") chosenRoom.value = room;
}
let metricTimer: ReturnType<typeof setInterval>;
onMounted(() => {
  void refreshMetrics();
  metricTimer = setInterval(() => {
    void refreshMetrics();
  }, 3000);
});
onBeforeUnmount(() => {
  clearInterval(metricTimer);
  disconnect();
});
</script>

<template>
  <div class="app-shell">
    <header class="topbar">
      <a class="brand" href="/" aria-label="Ginx Arena 首页"
        ><span class="brand-mark">G<span>↗</span></span
        ><strong>GINX<span class="brand-light"> / ARENA</span></strong></a
      >
      <nav aria-label="页面导航">
        <span class="nav-active">联机实验场</span
        ><a href="#diagnostics">框架诊断</a>
      </nav>
      <span class="version">WEB DEMO <b>v0.1</b></span>
    </header>
    <main>
      <section class="intro">
        <div>
          <div class="eyebrow">
            <span></span> BUILT ON GINX · POWERED BY PHASER
          </div>
          <h1>让框架，<span>真正跑起来。</span></h1>
          <p>
            进入竞技场，争抢水晶。在每一次移动背后，看见实时连接、路由与广播。
          </p>
        </div>
        <div class="intro-note">
          <span>01 / REALTIME LAB</span
          ><small>Vue 3 + TypeScript<br />Phaser 4 · Go · WebSocket</small>
        </div>
      </section>
      <section class="workbench">
        <div class="play-column">
          <div class="panel arena-panel">
            <div class="panel-heading">
              <div class="title-row">
                <span class="live-square"></span>
                <h2>水晶竞技场</h2>
                <span class="subtle">CRYSTAL RUN</span>
              </div>
              <span
                class="status"
                :class="state.status"
                data-testid="connection-status"
                ><i></i>{{ statusLabel }}</span
              >
            </div>
            <div class="arena-toolbar">
              <div class="room-tabs">
                <button
                  v-for="room in [1, 2]"
                  :key="room"
                  :class="{ selected: activeRoom === room }"
                  :aria-pressed="activeRoom === room"
                  @click="switchRoom(room)"
                  :disabled="state.status === 'connecting'"
                >
                  房间 0{{ room }}
                  <span>{{ room === 1 ? "草原" : "边境" }}</span>
                </button>
              </div>
              <span class="arena-capacity"
                ><i></i
                ><b data-testid="player-count">{{
                  state.snapshot?.players.length ?? 0
                }}</b>
                / 8 在线</span
              >
            </div>
            <div class="arena-stage">
              <GameCanvas />
              <div v-if="state.status !== 'online'" class="arena-overlay">
                <span class="overlay-icon">⌁</span
                ><strong>{{
                  state.status === "connecting"
                    ? "正在建立连接…"
                    : "竞技场已就绪"
                }}</strong>
                <p>取一个名字，开始你的联机测试</p>
                <span class="overlay-tag"
                  >服务端权威 · 多人同步 · 实时采集</span
                >
              </div>
              <div v-if="state.status === 'online'" class="arena-hud">
                <span
                  ><i></i> LIVE <b>房间 0{{ state.room }}</b></span
                ><span
                  >TICK
                  <b data-testid="server-tick">{{
                    state.snapshot?.tick ?? "—"
                  }}</b></span
                >
              </div>
            </div>
            <div class="arena-footer">
              <span
                ><kbd>W</kbd><kbd>A</kbd><kbd>S</kbd><kbd>D</kbd> / 方向键移动
                <em>·</em> 点击地图前往</span
              ><span class="collect-hint">◆ 水晶 +1 <em>·</em> 5 秒刷新</span>
            </div>
          </div>
          <div class="lower-grid">
            <section class="panel leaderboard">
              <div class="panel-heading">
                <h2>房间排行榜</h2>
                <span class="subtle">本局得分 · 不存档</span>
              </div>
              <div v-if="!leaderboard.length" class="empty-state">
                等待第一位探索者加入
              </div>
              <ol v-else>
                <li v-for="(player, index) in leaderboard" :key="player.id">
                  <span class="rank">{{
                    String(index + 1).padStart(2, "0")
                  }}</span
                  ><span
                    class="avatar"
                    :class="{ mine: player.id === state.id }"
                    >{{ player.name.slice(0, 1).toUpperCase() }}</span
                  ><span class="player-name"
                    >{{ player.name
                    }}<small v-if="player.id === state.id">YOU</small></span
                  ><b>{{ player.score }}<span> ◆</span></b>
                </li>
              </ol>
            </section>
            <section class="panel event-panel">
              <div class="panel-heading">
                <h2>事件流</h2>
                <span class="subtle">最近 50 条</span>
              </div>
              <div class="event-list" aria-live="polite">
                <div v-if="!state.logs.length" class="empty-state">
                  连接后，框架事件会在这里出现。
                </div>
                <div
                  v-for="item in state.logs"
                  :key="item.key"
                  class="event-row"
                >
                  <time>{{ item.time }}</time
                  ><span :class="item.kind">{{ item.message }}</span>
                </div>
              </div>
            </section>
          </div>
        </div>
        <aside class="side-column">
          <section class="panel identity-panel">
            <div class="panel-heading">
              <h2>你的探索者</h2>
              <span class="subtle">GUEST</span>
            </div>
            <form @submit.prevent="connect(name, chosenRoom)">
              <label for="nickname">玩家昵称</label
              ><input
                id="nickname"
                v-model="name"
                maxlength="16"
                placeholder="给探索者起个名字"
                :disabled="state.status !== 'offline'"
                autocomplete="off"
              />
              <p class="field-note">临时访客身份，无需注册。</p>
              <button
                v-if="state.status !== 'online'"
                type="submit"
                class="primary-button"
                :disabled="state.status === 'connecting' || !name.trim()"
              >
                {{ state.status === "connecting" ? "连接中…" : "进入竞技场" }}
                <span>↗</span></button
              ><button
                v-else
                type="button"
                class="disconnect-button"
                @click="disconnect"
              >
                断开连接 <span>↗</span>
              </button>
              <p v-if="state.error" class="error-message" role="alert">
                {{ state.error }}
              </p>
            </form>
            <div class="player-stats">
              <div>
                <span>本局水晶</span
                ><strong data-testid="my-score"
                  >{{ me?.score ?? 0 }}<small> ◆</small></strong
                >
              </div>
              <div>
                <span>往返延迟</span
                ><strong
                  >{{ state.status === "online" ? state.latency : "—"
                  }}<small> ms</small></strong
                >
              </div>
            </div>
            <div class="position-line">
              <span
                >POS
                <b data-testid="my-position">{{
                  me ? `${Math.round(me.x)}, ${Math.round(me.y)}` : "—, —"
                }}</b></span
              ><span
                >AOI 邻近 <b>{{ me?.nearby ?? 0 }}</b></span
              >
            </div>
          </section>
          <section class="panel controls-panel">
            <div class="panel-heading">
              <h2>移动控制</h2>
              <span class="subtle">TOUCH / KEYBOARD</span>
            </div>
            <div class="direction-pad">
              <button
                aria-label="向上移动"
                @pointerdown="move(0, -1, $event)"
                @pointerup="stopMove"
                @pointercancel="stopMove"
                @lostpointercapture="stopMove"
              >
                ↑
              </button>
              <div>
                <button
                  aria-label="向左移动"
                  @pointerdown="move(-1, 0, $event)"
                  @pointerup="stopMove"
                  @pointercancel="stopMove"
                  @lostpointercapture="stopMove"
                >
                  ←</button
                ><span>✥</span
                ><button
                  aria-label="向右移动"
                  @pointerdown="move(1, 0, $event)"
                  @pointerup="stopMove"
                  @pointercancel="stopMove"
                  @lostpointercapture="stopMove"
                >
                  →
                </button>
              </div>
              <button
                aria-label="向下移动"
                @pointerdown="move(0, 1, $event)"
                @pointerup="stopMove"
                @pointercancel="stopMove"
                @lostpointercapture="stopMove"
              >
                ↓
              </button>
            </div>
            <p>打开另一个浏览器标签页，<br />加入同一房间，观察双人同步。</p>
          </section>
          <section id="diagnostics" class="panel diagnostics">
            <div class="panel-heading">
              <h2>框架仪表</h2>
              <span class="backend-state" :class="{ ready: state.metrics }">{{
                state.metrics ? "后端在线" : "后端未连接"
              }}</span>
            </div>
            <dl>
              <div>
                <dt>TCP 连接</dt>
                <dd data-testid="tcp-connections">
                  {{ state.metrics?.transport.Connections ?? "—" }}
                </dd>
              </div>
              <div>
                <dt>Worker 池</dt>
                <dd>
                  {{ state.metrics?.workers ?? "—" }} <small>workers</small>
                </dd>
              </div>
              <div>
                <dt>模拟 / 广播</dt>
                <dd>
                  {{ state.metrics?.tick_rate ?? "—" }} /
                  {{ state.metrics?.snapshot_rate ?? "—" }} <small>Hz</small>
                </dd>
              </div>
              <div>
                <dt>入站消息</dt>
                <dd>{{ state.metrics?.transport.Messages ?? "—" }}</dd>
              </div>
              <div>
                <dt>接收 / 发送字节</dt>
                <dd>
                  {{ bytes(state.metrics?.transport.BytesIn ?? 0) }} /
                  {{ bytes(state.metrics?.transport.BytesOut ?? 0) }}
                </dd>
              </div>
              <div>
                <dt>路由异常</dt>
                <dd>{{ state.metrics?.transport.RouterPanics ?? "—" }}</dd>
              </div>
            </dl>
            <div class="packet-counter">
              <span>本端 ↑ {{ state.sent }}</span
              ><span>↓ {{ state.received }}</span
              ><small>packets</small>
            </div>
          </section>
        </aside>
      </section>
      <section class="pipeline">
        <span class="eyebrow">真实消息链路</span>
        <div>
          <span>Phaser 输入</span><i>→</i><span>WebSocket</span><i>→</i
          ><span>TCP 桥接</span><i>→</i><span>Ginx Worker</span><i>→</i
          ><span>房间 / AOI</span><i>→</i><span>广播同步</span>
        </div>
      </section>
      <footer class="page-footer">
        <span>GINX ARENA <i>/</i> 一个可观察的游戏框架实验场</span
        ><span>本地演示 · 临时身份 · 内存状态</span>
      </footer>
    </main>
  </div>
</template>
