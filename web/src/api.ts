// The API client. Every write carries the X-BitDispatcher header (the server's CSRF check).

export class ApiError extends Error {
  status: number
  field?: string
  data?: Record<string, unknown>
  constructor(status: number, message: string, field?: string, data?: Record<string, unknown>) {
    super(message)
    this.status = status
    this.field = field
    this.data = data
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { 'X-BitDispatcher': '1' }
  if (body !== undefined) headers['Content-Type'] = 'application/json'
  const res = await fetch('./api' + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body), credentials: 'same-origin' })
  if (res.status === 204) return undefined as T
  if (res.status === 202) {
    // The server needs something more (sign-in: a duty, a code): the caller reads it from the error.
    const d = (await res.json().catch(() => ({}))) as Record<string, unknown>
    throw new ApiError(202, String(d.message ?? 'more is needed'), undefined, d)
  }
  const text = await res.text()
  let data: unknown = undefined
  try {
    data = text ? JSON.parse(text) : undefined
  } catch {
    data = undefined
  }
  if (!res.ok) {
    const d = (data ?? {}) as { message?: string; field?: string }
    if (res.status === 401 && !path.startsWith('/auth/')) window.dispatchEvent(new Event('bd:signedout'))
    // A locked console: the lock screen covers everything until its person unlocks it.
    if (res.status === 423) window.dispatchEvent(new Event('bd:locked'))
    throw new ApiError(res.status, d.message ?? (text || res.statusText), d.field, data as Record<string, unknown>)
  }
  return data as T
}

export const api = {
  get: <T>(path: string) => request<T>('GET', path),
  post: <T = void>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  put: <T = void>(path: string, body: unknown) => request<T>('PUT', path, body),
  patch: <T = void>(path: string, body: unknown) => request<T>('PATCH', path, body),
  del: <T = void>(path: string, body?: unknown) => request<T>('DELETE', path, body),
}

export function errText(e: unknown): string {
  if (e instanceof Error) return e.message
  return String(e)
}
