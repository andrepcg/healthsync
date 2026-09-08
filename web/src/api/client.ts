export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

export type Params = Record<string, string | number | boolean | undefined | null>

export function withParams(path: string, params?: Params): string {
  if (!params) return path
  const q = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    q.set(k, String(v))
  }
  const s = q.toString()
  return s ? `${path}?${s}` : path
}

export async function fetchJson<T>(path: string, params?: Params, init?: RequestInit): Promise<T> {
  const res = await fetch(withParams(path, params), init)
  if (!res.ok) {
    let msg = res.statusText
    try {
      const body = await res.json()
      if (body && typeof body.error === 'string') msg = body.error
    } catch {
      /* not json */
    }
    throw new ApiError(res.status, msg)
  }
  if (res.status === 204) return undefined as T
  return (await res.json()) as T
}

export function postJson<T>(path: string, body: unknown, method = 'POST'): Promise<T> {
  return fetchJson<T>(path, undefined, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
}

export const personPath = (id: string, rest = '') => `/api/people/${encodeURIComponent(id)}${rest}`

/** Upload with progress, resolving when the server accepts the file (202). */
export function uploadExport(personId: string, file: File, onProgress: (pct: number) => void): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('POST', personPath(personId, '/upload'))
    xhr.upload.onprogress = (e) => {
      if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
    }
    xhr.onload = () => {
      if (xhr.status >= 200 && xhr.status < 300) return resolve()
      let msg = xhr.statusText
      try {
        msg = JSON.parse(xhr.responseText).error ?? msg
      } catch {
        /* ignore */
      }
      reject(new ApiError(xhr.status, msg))
    }
    xhr.onerror = () => reject(new ApiError(0, 'network error'))
    const form = new FormData()
    form.append('file', file)
    xhr.send(form)
  })
}
