import {
  test,
  expect,
  type Page,
} from "../../examples/webgame/frontend/node_modules/@playwright/test/index";
import { encode, MSG } from "../../examples/webgame/frontend/src/protocol";

async function enter(page: Page, name: string) {
  await page.goto("/");
  await page.getByLabel("玩家昵称").fill(name);
  await page.getByRole("button", { name: "进入竞技场" }).click();
  await expect(page.getByTestId("connection-status")).toHaveText("已连接");
  await expect(page.locator("canvas")).toBeVisible();
}

test("two browsers share server movement, scoring, isolated rooms and reconnect", async ({
  browser,
  baseURL,
}, testInfo) => {
  const contextA = await browser.newContext({
    baseURL,
    viewport: { width: 1440, height: 1050 },
  });
  const contextB = await browser.newContext({
    baseURL,
    viewport: { width: 1440, height: 1050 },
  });
  const a = await contextA.newPage(),
    b = await contextB.newPage();
  const errors: string[] = [];
  for (const page of [a, b])
    page.on("pageerror", (e) => errors.push(e.message));
  try {
    await enter(a, "Explorer-A");
    await enter(b, "Explorer-B");
    await expect(a.getByTestId("player-count")).toHaveText("2");
    await expect(b.getByTestId("player-count")).toHaveText("2");
    await expect(a.getByTestId("tcp-connections")).toHaveText("2");
    const initial = await a.getByTestId("my-position").innerText();
    await a.locator("canvas").focus();
    await a.keyboard.down("d");
    await expect
      .poll(async () =>
        Number((await a.getByTestId("my-position").innerText()).split(",")[0]),
      )
      .toBeGreaterThan(230);
    await a.keyboard.up("d");
    await expect(a.getByTestId("my-score")).toContainText("1");
    await expect(
      b.locator(".leaderboard li").filter({ hasText: "Explorer-A" }),
    ).toContainText("1");
    expect(await a.getByTestId("my-position").innerText()).not.toBe(initial);
    await a.screenshot({
      path: testInfo.outputPath("arena-desktop.png"),
      fullPage: true,
    });
    await b.getByRole("button", { name: "房间 02 边境" }).click();
    await expect(a.getByTestId("player-count")).toHaveText("1");
    await expect(b.getByTestId("player-count")).toHaveText("1");
    await expect(b.locator(".leaderboard")).not.toContainText("Explorer-A");
    await b.getByRole("button", { name: "房间 01 草原" }).click();
    await expect(a.getByTestId("player-count")).toHaveText("2");
    await b.getByRole("button", { name: "断开连接" }).click();
    await expect(a.getByTestId("player-count")).toHaveText("1");
    await expect(a.getByTestId("tcp-connections")).toHaveText("1");
    await b.getByRole("button", { name: "进入竞技场" }).click();
    await expect(b.getByTestId("connection-status")).toHaveText("已连接");
    await expect(a.getByTestId("player-count")).toHaveText("2");
    expect(errors).toEqual([]);
  } finally {
    await contextA.close();
    await contextB.close();
  }
});

test("mobile layout and malformed websocket rejection", async ({
  page,
  baseURL,
}, testInfo) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await enter(page, "Mobile");
  await expect(page.getByTestId("player-count")).toHaveText("1");
  await expect(page.getByTestId("tcp-connections")).toHaveText("1");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  // 先聚焦画布，再按方向按钮，确保按钮不因画布 blur 丢失输入。
  await page.locator("canvas").focus();
  const right = page.getByRole("button", { name: "向右移动" });
  await right.scrollIntoViewIfNeeded();
  const bounds = await right.boundingBox();
  if (!bounds) throw new Error("direction control not visible");
  await page.mouse.move(
    bounds.x + bounds.width / 2,
    bounds.y + bounds.height / 2,
  );
  await page.mouse.down();
  await expect
    .poll(async () =>
      Number((await page.getByTestId("my-position").innerText()).split(",")[0]),
    )
    .toBeGreaterThan(150);
  await page.mouse.up();
  await page.evaluate(() => window.scrollTo(0, 0));
  await page.screenshot({
    path: testInfo.outputPath("arena-mobile.png"),
    fullPage: true,
  });
  const closeCode = await page.evaluate(async () => {
    const url = new URL("/ws", location.href);
    url.protocol = "ws:";
    return await new Promise<number>((resolve, reject) => {
      const socket = new WebSocket(url);
      const timeout = setTimeout(() => {
        socket.close();
        reject(new Error("invalid packet was not rejected"));
      }, 5000);
      socket.onopen = () => socket.send("not a binary Ginx frame");
      socket.onclose = (event) => {
        clearTimeout(timeout);
        resolve(event.code);
      };
    });
  });
  expect(closeCode).toBe(1008);
  const response = await page.request.get(new URL("/healthz", baseURL).href);
  expect(response.ok()).toBe(true);
});

test("rejected room switch preserves the actual selected room and allows retry", async ({
  page,
}) => {
  let rejectNextSwitch = true;
  await page.routeWebSocket("**/ws", (ws) => {
    const upstream = ws.connectToServer();
    ws.onMessage((message) => {
      if (Buffer.isBuffer(message) && message.readUInt32LE(4) === MSG.Join) {
        const body = JSON.parse(message.subarray(8).toString());
        if (body.room_id === 2 && rejectNextSwitch) {
          rejectNextSwitch = false;
          ws.send(
            Buffer.from(
              encode(MSG.Error, { request_id: MSG.Join, code: "room_full" }),
            ),
          );
          return;
        }
      }
      upstream.send(message);
    });
  });
  await enter(page, "RoomRetry");
  await page.getByRole("button", { name: "房间 02 边境" }).click();
  await expect(page.getByRole("alert")).toHaveText("room_full");
  await expect(page.getByRole("button", { name: "房间 01 草原" })).toHaveClass(
    /selected/,
  );
  await expect(page.getByTestId("connection-status")).toHaveText("已连接");
  await page.getByRole("button", { name: "房间 02 边境" }).click();
  await expect(page.getByRole("button", { name: "房间 02 边境" })).toHaveClass(
    /selected/,
  );
  await expect(page.locator(".arena-hud")).toContainText("房间 02");
  await expect(page.getByRole("alert")).toHaveCount(0);
});

test("rejected initial join immediately releases connection and preserves error", async ({
  page,
}) => {
  await page.routeWebSocket("**/ws", (ws) => {
    const upstream = ws.connectToServer();
    ws.onMessage((message) => {
      if (Buffer.isBuffer(message) && message.readUInt32LE(4) === MSG.Join) {
        ws.send(
          Buffer.from(
            encode(MSG.Error, { request_id: MSG.Join, code: "room_full" }),
          ),
        );
        return;
      }
      upstream.send(message);
    });
  });
  await page.goto("/");
  await page.getByLabel("玩家昵称").fill("FullRoom");
  await page.getByRole("button", { name: "进入竞技场" }).click();
  await expect(page.getByRole("alert")).toHaveText("room_full");
  await expect(page.getByTestId("connection-status")).toHaveText("未连接");
  await expect(page.getByRole("button", { name: "进入竞技场" })).toBeEnabled();
});

test("failed metrics request clears the previous backend health indicator", async ({
  page,
}) => {
  await page.goto("/");
  await expect(page.locator(".backend-state")).toHaveText("后端在线");
  await page.route("**/api/metrics", (route) =>
    route.fulfill({ status: 503, json: { error: "unavailable" } }),
  );
  await expect(page.locator(".backend-state")).toHaveText("后端未连接");
  await page.unroute("**/api/metrics");
  await expect(page.locator(".backend-state")).toHaveText("后端在线");
});
