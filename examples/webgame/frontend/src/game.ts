import Phaser from "phaser";
import { input, state } from "./client";
import type { Player } from "./protocol";

export const controls = { x: 0, y: 0 };
export function createGame(parent: HTMLElement) {
  class Arena extends Phaser.Scene {
    private units = new Map<
      number,
      { body: Phaser.GameObjects.Container; name: Phaser.GameObjects.Text }
    >();
    private gems = new Map<number, Phaser.GameObjects.Container>();
    private keys = new Set<string>();
    private target: { x: number; y: number } | null = null;
    private marker!: Phaser.GameObjects.Arc;
    private previousRoom = 0;
    private stepAt = 0;
    constructor() {
      super("Arena");
    }
    create() {
      const g = this.add.graphics();
      g.fillStyle(0x151f26);
      g.fillRect(0, 0, 960, 540);
      g.lineStyle(1, 0x293841, 0.55);
      for (let x = 0; x <= 960; x += 40) g.lineBetween(x, 0, x, 540);
      for (let y = 0; y <= 540; y += 40) g.lineBetween(0, y, 960, y);
      g.lineStyle(1, 0x49635d, 0.6);
      g.strokeRoundedRect(24, 24, 912, 492, 20);
      g.lineStyle(1, 0x789c88, 0.15);
      g.strokeCircle(480, 270, 140);
      g.strokeCircle(480, 270, 145);
      g.lineBetween(480, 32, 480, 508);
      g.lineBetween(32, 270, 928, 270);
      for (const [x, y] of [
        [54, 54],
        [906, 54],
        [54, 486],
        [906, 486],
      ]) {
        g.lineStyle(3, 0xa6cf8b, 0.7);
        g.lineBetween(x - 8, y, x + 8, y);
        g.lineBetween(x, y - 8, x, y + 8);
      }
      this.add
        .text(480, 249, "GINX", {
          fontFamily: "monospace",
          fontSize: "46px",
          color: "#2f4446",
          fontStyle: "bold",
        })
        .setOrigin(0.5);
      this.add
        .text(480, 293, "REALTIME PLAYGROUND", {
          fontFamily: "monospace",
          fontSize: "12px",
          color: "#425759",
          letterSpacing: 3,
        })
        .setOrigin(0.5);
      this.add.text(42, 38, "SECTOR 01 / 960 × 540", {
        fontFamily: "monospace",
        fontSize: "11px",
        color: "#779084",
      });
      this.add
        .text(918, 502, "SERVER AUTHORITY", {
          fontFamily: "monospace",
          fontSize: "11px",
          color: "#779084",
        })
        .setOrigin(1);
      this.marker = this.add
        .circle(0, 0, 13)
        .setStrokeStyle(1, 0xd4eca7)
        .setVisible(false);
      const canvas = this.game.canvas;
      canvas.tabIndex = 0;
      canvas.setAttribute(
        "aria-label",
        "竞技场，使用方向键或 WASD 移动，也可点击目的地",
      );
      const keydown = (e: KeyboardEvent) => {
        if (
          document.activeElement !== canvas ||
          ![
            "w",
            "a",
            "s",
            "d",
            "ArrowUp",
            "ArrowDown",
            "ArrowLeft",
            "ArrowRight",
          ].includes(e.key)
        )
          return;
        e.preventDefault();
        this.keys.add(e.key);
        this.target = null;
      };
      const keyup = (e: KeyboardEvent) => {
        this.keys.delete(e.key);
      };
      const blur = () => {
        this.keys.clear();
        this.target = null;
        controls.x = 0;
        controls.y = 0;
        input(0, 0, true);
      };
      window.addEventListener("keydown", keydown);
      window.addEventListener("keyup", keyup);
      window.addEventListener("blur", blur);
      const visibility = () => {
        if (document.hidden) blur();
      };
      document.addEventListener("visibilitychange", visibility);
      canvas.addEventListener("blur", blur);
      this.input.on("pointerdown", (pointer: Phaser.Input.Pointer) => {
        canvas.focus({ preventScroll: true });
        if (state.status === "online")
          this.target = {
            x: Phaser.Math.Clamp(pointer.x, 24, 936),
            y: Phaser.Math.Clamp(pointer.y, 24, 516),
          };
      });
      this.events.once("shutdown", () => {
        window.removeEventListener("keydown", keydown);
        window.removeEventListener("keyup", keyup);
        window.removeEventListener("blur", blur);
        document.removeEventListener("visibilitychange", visibility);
        canvas.removeEventListener("blur", blur);
        controls.x = 0;
        controls.y = 0;
      });
    }
    private makePlayer(p: Player) {
      const mine = p.id === state.id;
      const color = mine ? 0xc4ee93 : 0x77bfea;
      const shadow = this.add.ellipse(0, 16, 40, 15, 0x050b0e, 0.4);
      const halo = this.add
        .circle(0, 0, 25, color, 0.06)
        .setStrokeStyle(1, color, 0.25);
      const body = this.add.circle(0, 0, 16, color);
      const visor = this.add.rectangle(0, -2, 23, 8, 0x1b3033).setAlpha(0.9);
      const eye = this.add.rectangle(5, -2, 6, 3, 0xffffff);
      const name = this.add
        .text(0, -36, p.name + (mine ? " · YOU" : ""), {
          fontFamily: "sans-serif",
          fontSize: "12px",
          color: mine ? "#d7f3b5" : "#a3d5f3",
          backgroundColor: "#152129",
          padding: { x: 5, y: 3 },
        })
        .setOrigin(0.5);
      const container = this.add.container(p.x, p.y, [
        shadow,
        halo,
        body,
        visor,
        eye,
        name,
      ]);
      container.setDepth(10);
      return { body: container, name };
    }
    update(time: number, delta: number) {
      const snapshot = state.snapshot;
      if (state.room !== this.previousRoom) {
        this.target = null;
        this.previousRoom = state.room;
      }
      const players = snapshot?.players ?? [];
      const ids = new Set(players.map((p) => p.id));
      for (const [id, unit] of this.units)
        if (!ids.has(id)) {
          unit.body.destroy();
          this.units.delete(id);
        }
      for (const p of players) {
        let unit = this.units.get(p.id);
        if (!unit) {
          unit = this.makePlayer(p);
          this.units.set(p.id, unit);
        }
        const factor = Math.min(1, delta / 70);
        unit.body.x += (p.x - unit.body.x) * factor;
        unit.body.y += (p.y - unit.body.y) * factor;
      }
      const gems = snapshot?.crystals ?? [];
      const gemIds = new Set(gems.map((gem) => gem.id));
      for (const [id, gem] of this.gems)
        if (!gemIds.has(id)) {
          gem.destroy();
          this.gems.delete(id);
        }
      for (const c of gems) {
        let gem = this.gems.get(c.id);
        if (!gem) {
          const glow = this.add.circle(0, 0, 20, 0xdfc082, 0.06);
          const diamond = this.add
            .rectangle(0, 0, 13, 13, 0xf0cf88)
            .setAngle(45)
            .setStrokeStyle(1, 0xffe9b4);
          const dot = this.add.rectangle(-2, -2, 4, 4, 0xfff2d2).setAngle(45);
          gem = this.add.container(c.x, c.y, [glow, diamond, dot]);
          this.gems.set(c.id, gem);
        }
        gem.y = c.y + Math.sin(time / 450 + c.id) * 3;
      }
      const me = players.find((p) => p.id === state.id);
      let dx =
        controls.x +
        Number(this.keys.has("d") || this.keys.has("ArrowRight")) -
        Number(this.keys.has("a") || this.keys.has("ArrowLeft"));
      let dy =
        controls.y +
        Number(this.keys.has("s") || this.keys.has("ArrowDown")) -
        Number(this.keys.has("w") || this.keys.has("ArrowUp"));
      if ((dx || dy) && this.target) this.target = null;
      if (me && this.target) {
        const distance = Math.hypot(this.target.x - me.x, this.target.y - me.y);
        if (distance < 18) this.target = null;
        else {
          dx = (this.target.x - me.x) / distance;
          dy = (this.target.y - me.y) / distance;
        }
      }
      this.marker.setVisible(!!this.target);
      if (this.target) this.marker.setPosition(this.target.x, this.target.y);
      const length = Math.hypot(dx, dy);
      if (length > 1) {
        dx /= length;
        dy /= length;
      }
      if (me && time - this.stepAt >= 50) {
        input(dx, dy);
        this.stepAt = time;
      }
    }
  }
  return new Phaser.Game({
    type: Phaser.AUTO,
    parent,
    backgroundColor: "#151f26",
    width: 960,
    height: 540,
    scene: Arena,
    scale: { mode: Phaser.Scale.FIT, autoCenter: Phaser.Scale.CENTER_BOTH },
    input: { keyboard: false },
    render: { antialias: true },
    audio: { noAudio: true },
    banner: false,
  });
}
