import type { ReactNode } from 'react'

/** v0.4.0 空状态：inline SVG 插画 + 说明 + 可选操作，深浅色共用品牌色。 */
export function EmptyState({
  title,
  description,
  action,
  className = '',
}: {
  title: string
  description?: string
  action?: ReactNode
  className?: string
}) {
  return (
    <div className={`empty-state-card ${className}`.trim()}>
      <EmptyIllustration />
      <div className="empty-state-copy">
        <strong>{title}</strong>
        {description && <p>{description}</p>}
        {action && <div className="empty-state-action">{action}</div>}
      </div>
    </div>
  )
}

function EmptyIllustration() {
  return (
    <svg className="empty-state-illustration" viewBox="0 0 96 72" aria-hidden="true" focusable="false">
      <g fill="none" stroke="currentColor" strokeWidth="1.4" strokeLinecap="round" strokeLinejoin="round">
        <path d="M18 45.5 48 29.5l30 16-30 16z" opacity=".42" />
        <path d="M18 45.5v9L48 70.5l30-16v-9" opacity=".3" />
        <path d="M48 61.5v-16" opacity=".3" />
        <path d="M33 22.5 48 14l15 8.5" opacity=".62" />
        <circle cx="48" cy="22.5" r="2.6" opacity=".72" />
        <path d="M26 34.5 48 23l22 11.5" strokeDasharray="2 4" opacity=".45" />
        <path d="M21 65.5h54" opacity=".18" />
      </g>
      <g fill="currentColor" opacity=".6">
        <circle cx="22" cy="25" r="1.6" />
        <circle cx="75" cy="19" r="1.3" />
        <circle cx="81" cy="36" r="1.8" />
      </g>
    </svg>
  )
}
