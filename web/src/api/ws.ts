import { WsMessage } from '../types/api';

type WsListener = (msg: WsMessage) => void;

/**
 * WebSocket-менеджер с автоматическим реконнектом.
 * Сервер шлёт сообщения, клиент ничего не отправляет.
 */
export class WsManager {
  private ws: WebSocket | null = null;
  private listeners: Set<WsListener> = new Set();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private token: string | null = null;
  private intentionallyClosed = false;

  connect(token?: string | null) {
    this.intentionallyClosed = false;
    this.token = token ?? null;
    this.doConnect();
  }

  disconnect() {
    this.intentionallyClosed = true;
    this.clearReconnect();
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
  }

  subscribe(listener: WsListener): () => void {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  }

  private doConnect() {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const base = `${proto}//${window.location.host}`;
    const url = this.token ? `${base}/ws?token=${this.token}` : `${base}/ws`;

    const ws = new WebSocket(url);

    ws.onmessage = (event) => {
      try {
        const msg: WsMessage = JSON.parse(event.data);
        this.listeners.forEach((fn) => fn(msg));
      } catch {
        console.warn('[WS] Failed to parse message:', event.data);
      }
    };

    ws.onclose = () => {
      this.ws = null;
      if (!this.intentionallyClosed) {
        this.scheduleReconnect();
      }
    };

    ws.onerror = () => {
      // onclose будет вызван после onerror
    };

    this.ws = ws;
  }

  private scheduleReconnect() {
    this.clearReconnect();
    // Пауза 1-5 секунд (как в контракте)
    const delay = 1000 + Math.random() * 4000;
    this.reconnectTimer = setTimeout(() => {
      this.doConnect();
    }, delay);
  }

  private clearReconnect() {
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
  }
}

export const wsManager = new WsManager();

