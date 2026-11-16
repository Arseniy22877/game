import { WsMessage } from '../types/api';

type WsListener = (msg: WsMessage) => void;

/**
 * WebSocket-менеджер с автоматическим реконнектом и поддержкой смены токена.
 */
export class WsManager {
  private ws: WebSocket | null = null;
  private listeners: Set<WsListener> = new Set();
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private token: string | null = null;
  private intentionallyClosed = false;
  private connected = false;

  connect(token?: string | null) {
    const newToken = token ?? null;
    // Если токен изменился или соединения нет — переподключаемся
    if (this.token !== newToken || !this.ws || this.ws.readyState !== WebSocket.OPEN) {
      this.intentionallyClosed = false;
      this.token = newToken;
      this.doConnect();
    }
  }

  isConnected(): boolean {
    return this.connected && this.ws !== null && this.ws.readyState === WebSocket.OPEN;
  }

  disconnect() {
    this.intentionallyClosed = true;
    this.connected = false;
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
    this.clearReconnect();
    if (this.ws) {
      try {
        this.ws.close();
      } catch {
        // ignore
      }
      this.ws = null;
    }

    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const base = `${proto}//${window.location.host}`;
    const url = this.token ? `${base}/ws?token=${encodeURIComponent(this.token)}` : `${base}/ws`;

    try {
      const ws = new WebSocket(url);

      ws.onopen = () => {
        this.connected = true;
        console.log('[WS] Connected to', url);
      };

      ws.onmessage = (event) => {
        try {
          const msg: WsMessage = JSON.parse(event.data);
          this.listeners.forEach((fn) => {
            try {
              fn(msg);
            } catch (err) {
              console.error('[WS] Listener error:', err);
            }
          });
        } catch {
          console.warn('[WS] Failed to parse message:', event.data);
        }
      };

      ws.onclose = () => {
        this.connected = false;
        this.ws = null;
        if (!this.intentionallyClosed) {
          this.scheduleReconnect();
        }
      };

      ws.onerror = () => {
        this.connected = false;
      };

      this.ws = ws;
    } catch (e) {
      console.warn('[WS] Connection failed:', e);
      this.scheduleReconnect();
    }
  }

  private scheduleReconnect() {
    this.clearReconnect();
    // Пауза 1.5 - 3.5 секунды
    const delay = 1500 + Math.random() * 2000;
    this.reconnectTimer = setTimeout(() => {
      if (!this.intentionallyClosed) {
        this.doConnect();
      }
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
