// 示例自己的业务协议；8 字节头与 Ginx DataPack 完全一致。
export const MSG = {
  Authenticate: 4101,
  Join: 4102,
  Input: 4103,
  Leave: 4104,
  Heartbeat: 4105,
  Snapshot: 4201,
  Error: 4202,
  Event: 4203,
} as const;
export const MAX_PACKET = 64 * 1024;
export interface Player {
  id: number;
  name: string;
  x: number;
  y: number;
  score: number;
  seq: number;
  nearby: number;
}
export interface Crystal {
  id: number;
  x: number;
  y: number;
}
export interface Snapshot {
  tick: number;
  room_id: number;
  players: Player[];
  crystals: Crystal[];
}
export interface Metrics {
  transport: {
    Connections: number;
    Messages: number;
    BytesIn: number;
    BytesOut: number;
    RouterPanics: number;
  };
  tick_rate: number;
  snapshot_rate: number;
  workers: number;
}

export function encode(id: number, data: unknown): ArrayBuffer {
  const bytes = new TextEncoder().encode(JSON.stringify(data));
  if (bytes.length > MAX_PACKET) throw new Error("消息超过大小限制");
  const packet = new ArrayBuffer(8 + bytes.length);
  const view = new DataView(packet);
  view.setUint32(0, bytes.length, true);
  view.setUint32(4, id, true);
  new Uint8Array(packet, 8).set(bytes);
  return packet;
}
export function decode(packet: ArrayBuffer): { id: number; data: unknown } {
  if (!(packet instanceof ArrayBuffer) || packet.byteLength < 8)
    throw new Error("消息头不完整");
  const view = new DataView(packet);
  const length = view.getUint32(0, true);
  if (length > MAX_PACKET || length + 8 !== packet.byteLength)
    throw new Error("消息长度不匹配");
  return {
    id: view.getUint32(4, true),
    data: JSON.parse(
      new TextDecoder("utf-8", { fatal: true }).decode(
        new Uint8Array(packet, 8),
      ),
    ),
  };
}
