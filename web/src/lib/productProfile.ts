import type { UpdatePolicy } from '../types'

export const DEFAULT_UPDATE_POLICY: UpdatePolicy = 'LIFETIME'

export const UPDATE_POLICY_LABELS: Record<UpdatePolicy, string> = {
  LIFETIME: '有效期内免费更新',
  TILL_DATE: '更新至指定日期',
  CURRENT_ONLY: '仅当前版本',
}

/** 旧版 API 可能不返回更新策略；未知值统一回退到 LIFETIME。 */
export function normalizeUpdatePolicy(policy?: string | null): UpdatePolicy {
  if (policy === 'TILL_DATE' || policy === 'CURRENT_ONLY' || policy === 'LIFETIME') return policy
  return DEFAULT_UPDATE_POLICY
}

function localDateParts(value: string): { year: number; month: number; day: number } | null {
  const trimmed = value.trim()
  // 纯日期（无时区）按字面日期展示，避免被 parse 成 UTC 午夜后本地日期偏移一天。
  const dateOnly = /^(\d{4})-(\d{2})-(\d{2})$/.exec(trimmed)
  if (dateOnly) {
    return { year: Number(dateOnly[1]), month: Number(dateOnly[2]), day: Number(dateOnly[3]) }
  }
  const date = new Date(trimmed)
  if (!Number.isNaN(date.getTime())) {
    return { year: date.getFullYear(), month: date.getMonth() + 1, day: date.getDate() }
  }
  // 后端按 RFC3339 返回；无法解析时仍尝试取日期前缀，避免展示异常值。
  const match = /^(\d{4})-(\d{2})-(\d{2})/.exec(trimmed)
  if (!match) return null
  return { year: Number(match[1]), month: Number(match[2]), day: Number(match[3]) }
}

/** 把 RFC3339 时间格式化为本地日期 YYYY-MM-DD。 */
export function formatUpdateUntil(value?: string | null): string | null {
  if (!value) return null
  const parts = localDateParts(value)
  if (!parts) return null
  return `${parts.year}-${String(parts.month).padStart(2, '0')}-${String(parts.day).padStart(2, '0')}`
}

/**
 * 统一更新策略文案：
 * LIFETIME=“有效期内免费更新”；TILL_DATE=“更新至 YYYY-MM-DD”；CURRENT_ONLY=“仅当前版本”。
 */
export function updatePolicyLabel(policy?: string | null, updateUntil?: string | null): string {
  switch (normalizeUpdatePolicy(policy)) {
    case 'TILL_DATE': {
      const date = formatUpdateUntil(updateUntil)
      return date ? `更新至 ${date}` : UPDATE_POLICY_LABELS.TILL_DATE
    }
    case 'CURRENT_ONLY':
      return UPDATE_POLICY_LABELS.CURRENT_ONLY
    case 'LIFETIME':
    default:
      return UPDATE_POLICY_LABELS.LIFETIME
  }
}
