import { ServerMsg, ClientMsg } from './types'

type Handler = (msg: ServerMsg) => void

export class WSClient {
  private ws:    WebSocket | null = null
  private timer: ReturnType<typeof setTimeout> | null = null
  private handlers: Handler[] = []
  private _connected = false

  constructor(private readonly url: string) {
    this.connect()
  }

  private connect() {
    this.ws = new WebSocket(this.url)

    this.ws.onopen = () => {
      this._connected = true
      this.emit({ type: '__connected' } as unknown as ServerMsg)
    }

    this.ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data as string) as ServerMsg
        this.handlers.forEach(h => h(msg))
      } catch (e) {
        console.error('[ws] parse error', e)
      }
    }

    this.ws.onclose = () => {
      this._connected = false
      this.emit({ type: '__disconnected' } as unknown as ServerMsg)
      this.timer = setTimeout(() => this.connect(), 2500)
    }

    this.ws.onerror = () => { /* close fires too */ }
  }

  private emit(msg: ServerMsg) {
    this.handlers.forEach(h => h(msg))
  }

  send(msg: ClientMsg) {
    if (this.ws?.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(msg))
    }
  }

  on(handler: Handler): () => void {
    this.handlers.push(handler)
    return () => { this.handlers = this.handlers.filter(h => h !== handler) }
  }

  get connected() { return this._connected }

  destroy() {
    if (this.timer) clearTimeout(this.timer)
    this.ws?.close()
  }
}
