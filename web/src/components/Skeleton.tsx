/**
 * v0.4.0 加载骨架：占位结构尽量对齐真实布局，减少内容加载完成后的布局跳动。
 * 容器统一带 aria-busy，读屏通过隐藏文本获知加载状态。
 */
export function CatalogSkeleton() {
  return (
    <div className="catalog-skeleton" aria-busy="true">
      <span className="sr-only" role="status">正在整理产品目录…</span>
      <div className="product-grid" aria-hidden="true">
        {[0, 1, 2].map((index) => (
          <div className="product-card skeleton-card" key={index}>
            <div className="skeleton-block skeleton-card-cover" />
            <div className="product-card-copy">
              <div className="skeleton-line skeleton-line--index" />
              <div className="skeleton-line skeleton-line--lg" />
              <div className="skeleton-line" />
              <div className="skeleton-line skeleton-line--short" />
              <div className="product-card-foot">
                <span className="skeleton-line skeleton-line--foot" />
                <span className="skeleton-line skeleton-line--foot-short" />
              </div>
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

export function ProductDetailSkeleton() {
  return (
    <section className="product-detail-page product-detail-skeleton" aria-busy="true">
      <span className="sr-only" role="status">正在读取商品信息…</span>
      <div className="skeleton-line skeleton-line--back" aria-hidden="true" />
      <div className="product-detail-layout" aria-hidden="true">
        <div className="product-detail-copy">
          <div className="skeleton-line skeleton-line--kicker" />
          <div className="skeleton-line skeleton-line--index" />
          <div className="skeleton-block skeleton-title-block" />
          <div className="skeleton-line" />
          <div className="skeleton-line" />
          <div className="skeleton-line skeleton-line--short" />
          <div className="skeleton-line skeleton-line--meta" />
        </div>
        <aside className="product-detail-action surface-panel skeleton-panel">
          <div className="skeleton-block skeleton-panel-icon" />
          <div className="skeleton-line skeleton-line--kicker" />
          <div className="skeleton-line skeleton-line--lg" />
          <div className="skeleton-line" />
          <div className="skeleton-line skeleton-line--short" />
          <div className="skeleton-block skeleton-panel-button" />
        </aside>
      </div>
      <div className="product-gallery" aria-hidden="true">
        <div className="skeleton-block skeleton-gallery-block skeleton-gallery-block--featured" />
        <div className="skeleton-block skeleton-gallery-block" />
        <div className="skeleton-block skeleton-gallery-block" />
      </div>
    </section>
  )
}

export function DeliverySkeleton() {
  return (
    <section className="delivery-page delivery-skeleton" aria-busy="true">
      <span className="sr-only" role="status">正在打开交付空间…</span>
      <div className="delivery-title-row" aria-hidden="true">
        <div>
          <div className="skeleton-line skeleton-line--kicker" />
          <div className="skeleton-line skeleton-line--page-title" />
        </div>
        <div className="skeleton-block skeleton-icon-block" />
      </div>
      <div className="delivery-skeleton-overview" aria-hidden="true">
        <div className="skeleton-block skeleton-symbol" />
        <div className="delivery-skeleton-overview-copy">
          <div className="skeleton-line skeleton-line--kicker" />
          <div className="skeleton-line skeleton-line--lg" />
          <div className="skeleton-line skeleton-line--short" />
        </div>
        <div className="skeleton-line skeleton-line--quota" />
      </div>
      <div className="delivery-skeleton-version" aria-hidden="true">
        <div className="skeleton-block skeleton-icon-block" />
        <div className="delivery-skeleton-overview-copy">
          <div className="skeleton-line skeleton-line--kicker" />
          <div className="skeleton-line skeleton-line--lg" />
          <div className="skeleton-line" />
        </div>
      </div>
      <div className="content-section-heading" aria-hidden="true">
        <div>
          <div className="skeleton-line skeleton-line--kicker" />
          <div className="skeleton-line skeleton-line--section-title" />
        </div>
      </div>
      <div className="delivery-skeleton-assets" aria-hidden="true">
        {[0, 1, 2].map((index) => (
          <div className="delivery-skeleton-asset" key={index}>
            <div className="skeleton-block skeleton-file-icon" />
            <div className="delivery-skeleton-overview-copy">
              <div className="skeleton-line skeleton-line--lg" />
              <div className="skeleton-line skeleton-line--short" />
            </div>
            <div className="skeleton-line skeleton-line--button" />
          </div>
        ))}
      </div>
    </section>
  )
}

/** 懒加载路由的 Suspense 占位：保持与页面一致的留白，避免首屏跳动。 */
export function RouteLoading() {
  return (
    <div className="route-loading" aria-busy="true">
      <span className="sr-only" role="status">正在打开页面…</span>
      <div className="skeleton-line skeleton-line--kicker" aria-hidden="true" />
      <div className="skeleton-block route-loading-title" aria-hidden="true" />
      <div className="skeleton-line route-loading-copy" aria-hidden="true" />
      <div className="skeleton-line skeleton-line--short" aria-hidden="true" />
    </div>
  )
}
