// Lightweight loading placeholders. The shimmer is pure CSS and the global
// prefers-reduced-motion rule collapses the animation automatically.
export function SkeletonRows({ rows = 4, columns = 5, label = '正在加载内容' }: { rows?: number; columns?: number; label?: string }) {
  return <div className="skeleton-block" role="status" aria-live="polite" aria-busy="true">
    <span className="sr-only">{label}</span>
    {Array.from({ length: rows }, (_, row) => <div className="skeleton-row" key={row}>
      {Array.from({ length: columns }, (_, column) => <span className={`skeleton-bar skeleton-bar--${(row + column) % 4}`} key={column} />)}
    </div>)}
  </div>
}

export function SkeletonLines({ lines = 3, label = '正在加载内容' }: { lines?: number; label?: string }) {
  return <div className="skeleton-block skeleton-block--lines" role="status" aria-live="polite" aria-busy="true">
    <span className="sr-only">{label}</span>
    {Array.from({ length: lines }, (_, line) => <span className={`skeleton-bar skeleton-bar--${line % 4}`} key={line} />)}
  </div>
}
