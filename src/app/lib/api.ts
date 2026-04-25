export class ApiError extends Error {
  status: number;
  code?: string;
  details?: unknown;

  constructor(message: string, status: number, code?: string, details?: unknown) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
    this.code = code;
    this.details = details;
  }
}

type JsonLike = Record<string, unknown>;

async function readJsonSafe(res: Response): Promise<JsonLike | null> {
  try {
    const parsed = await res.json();
    if (parsed && typeof parsed === 'object') {
      return parsed as JsonLike;
    }
    return null;
  } catch {
    return null;
  }
}

function messageFromPayload(payload: JsonLike | null): string | null {
  if (!payload) return null;
  const message = payload.message;
  if (typeof message === 'string' && message.trim() !== '') return message;
  const error = payload.error;
  if (typeof error === 'string' && error.trim() !== '') return error;
  return null;
}

export function toErrorMessage(err: unknown, fallback: string): string {
  if (err instanceof Error && err.message.trim() !== '') {
    return err.message;
  }
  return fallback;
}

export async function apiRequest<T>(
  input: RequestInfo | URL,
  init?: RequestInit,
  fallbackErrorMessage = 'Request failed'
): Promise<T> {
  const res = await fetch(input, init);
  if (!res.ok) {
    const payload = await readJsonSafe(res);
    const message = messageFromPayload(payload) || fallbackErrorMessage;
    const code = payload && typeof payload.error === 'string' ? payload.error : undefined;
    throw new ApiError(message, res.status, code, payload ?? undefined);
  }

  if (res.status === 204) {
    return undefined as T;
  }

  return (await res.json()) as T;
}

export async function apiRequestRaw(
  input: RequestInfo | URL,
  init?: RequestInit
): Promise<Response> {
  return fetch(input, init);
}
