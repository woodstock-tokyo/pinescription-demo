import { ServerMsg, ClientMsg } from './types'

type ConnectionEvent =
  | { type: '__connected' }
  | { type: '__disconnected' }

type InboundMsg = ServerMsg | ConnectionEvent
type Handler = (msg: InboundMsg) => void

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
      this.emit({ type: '__connected' })
    }

    this.ws.onmessage = (ev: MessageEvent) => {
      try {
        const msg = JSON.parse(ev.data as string) as ServerMsg
        this.emit(msg)
      } catch (e) {
        console.error('[ws] parse error', e)
      }
    }

    this.ws.onclose = () => {
      this._connected = false
      this.emit({ type: '__disconnected' })
      this.timer = setTimeout(() => this.connect(), 2500)
    }

    this.ws.onerror = () => { /* close fires too */ }
  }

  private emit(msg: InboundMsg) {
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
