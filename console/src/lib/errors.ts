import { ApiError } from './api'

// Surface the API error code next to the human message so operators can quote
// the exact contract value when they report an issue back to the backend.
export function describeError(error: unknown): string {
  if (error instanceof ApiError) {
    if (!error.code || error.code === 'request_failed' || error.code === 'network_error') return error.message
    return `${error.message}（错误码：${error.code}）`
  }
  return error instanceof Error ? error.message : '操作没有完成，请稍后重试。'
}
