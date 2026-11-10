export function BrandMark({ compact = false }: { compact?: boolean }) {
  return (
    <div className={`brand-mark ${compact ? 'brand-mark--compact' : ''}`} aria-label="知屿 NOVER">
      <span className="island-glyph" aria-hidden="true">
        <i />
        <i />
        <i />
      </span>
      <span className="brand-wordmark">
        <strong>知屿</strong>
        <span>NOVER</span>
      </span>
    </div>
  )
}
