import { useState, type FormEvent } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { ChevronDown, ChevronUp, ListFilter, RefreshCw, ScrollText } from 'lucide-react'
import { StatusMessage } from '../../components/StatusMessage'
import { loadAuditEvents } from '../../lib/api'
import { describeError } from '../../lib/errors'
import type { AuditEvent } from '../../types'

const PAGE_SIZE = 20
// Values written by the backend admin_events table. The action filter is
// uppercased server-side; resource types match these snake_case values.
const RESOURCE_TYPES = ['product', 'product_release', 'product_image', 'asset', 'redeem_code', 'entitlement', 'member', 'operator_user', 'join_application', 'platform_settings']
const KNOWN_ACTIONS = ['PRODUCT_DRAFT_CREATED', 'PRODUCT_CONTENT_UPDATED', 'PRODUCT_STATUS_UPDATED', 'PRODUCT_RELEASE_CREATED', 'PRODUCT_RELEASE_STATUS_UPDATED', 'PRODUCT_IMAGE_UPLOADED', 'PRODUCT_IMAGE_DELETED', 'ASSET_UPLOADED', 'ASSET_DELETED', 'CODES_ISSUED', 'SUSPEND', 'RESUME', 'REVOKE', 'REFUND', 'MEMBER_CREATED', 'MEMBER_UPDATED', 'PASSWORD_CHANGED', 'JOIN_APPLICATION_REVIEWED', 'PLATFORM_SETTINGS_UPDATED']

export function AuditView({ token }: { token: string }) {
  const [actionInput, setActionInput] = useState('')
  const [resourceTypeInput, setResourceTypeInput] = useState('')
  const [filters, setFilters] = useState({ action: '', resource_type: '' })
  const audit = useInfiniteQuery({
    queryKey: ['audit-events', token, filters],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => loadAuditEvents(token, { limit: PAGE_SIZE, offset: pageParam, action: filters.action, resource_type: filters.resource_type }),
    getNextPageParam: (lastPage) => lastPage.has_more ? lastPage.offset + lastPage.limit : undefined,
    retry: false,
  })
  const items = audit.data?.pages.flatMap((page) => page.items) ?? []
  const total = audit.data?.pages[0]?.total

  function applyFilters(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const next = { action: actionInput.trim(), resource_type: resourceTypeInput.trim() }
    if (next.action === filters.action && next.resource_type === filters.resource_type) {
      void audit.refetch()
      return
    }
    setFilters(next)
  }

  function resetFilters() {
    setActionInput('')
    setResourceTypeInput('')
    setFilters({ action: '', resource_type: '' })
  }

  return <section className="audit-view">
    <div className="overview-heading"><div><span className="section-kicker">AUDIT TRAIL</span><h2>审计日志</h2></div><span>{audit.isLoading ? '正在读取记录' : total === undefined ? '只读审计视图' : `共 ${total} 条记录`}</span></div>
    <div className="application-policy-banner"><ScrollText size={19} /><div><strong>关键操作只读留痕</strong><span>审计日志按时间倒序记录成员、商品、兑换与账户安全操作。当前视图只读，不能修改或删除历史记录。</span></div></div>
    <form className="audit-filter-bar" onSubmit={applyFilters}>
      <label>操作动作 action<input value={actionInput} onChange={(event) => setActionInput(event.target.value)} maxLength={64} list="audit-actions" placeholder="例如：REFUND" /><datalist id="audit-actions">{KNOWN_ACTIONS.map((action) => <option value={action} key={action} />)}</datalist></label>
      <label>资源类型 resource_type<input value={resourceTypeInput} onChange={(event) => setResourceTypeInput(event.target.value)} maxLength={64} list="audit-resource-types" placeholder="例如：redeem_code" /><datalist id="audit-resource-types">{RESOURCE_TYPES.map((type) => <option value={type} key={type} />)}</datalist></label>
      <div className="audit-filter-actions">
        <button className="button button-secondary" type="submit" disabled={audit.isFetching}><ListFilter size={15} />查询</button>
        <button className="button button-secondary" type="button" onClick={resetFilters} disabled={audit.isFetching && !items.length}><RefreshCw size={15} />重置</button>
      </div>
    </form>
    {audit.isError && <StatusMessage>{describeError(audit.error)}</StatusMessage>}
    <div className="audit-table">
      <div className="audit-table-head"><span>时间</span><span>操作者</span><span>动作</span><span>资源</span><span>元数据</span></div>
      {audit.isLoading ? <div className="empty-state">正在读取审计事件…</div> : items.length ? items.map((item) => <AuditRow item={item} key={item.id} />) : <div className="audit-empty"><ScrollText size={24} /><strong>没有匹配的审计事件</strong><span>调整操作或资源类型筛选后再试。</span></div>}
    </div>
    {audit.hasNextPage && <button className="button button-secondary audit-load-more" type="button" disabled={audit.isFetchingNextPage} onClick={() => void audit.fetchNextPage()}><ChevronDown size={15} />{audit.isFetchingNextPage ? '加载中…' : '加载更多'}</button>}
    {!audit.hasNextPage && items.length > 0 && <p className="audit-end-note">已显示全部 {items.length} 条匹配记录。</p>}
  </section>
}

function AuditRow({ item }: { item: AuditEvent }) {
  const [expanded, setExpanded] = useState(false)
  const metadataText = formatMetadata(item.metadata)
  return <article className="audit-row">
    <span className="audit-time">{formatTime(item.created_at)}</span>
    <span className="audit-actor" title={item.actor}>{item.actor || '系统'}</span>
    <code className="audit-action">{item.action || '—'}</code>
    <span className="audit-resource"><strong>{item.resource_type || '—'}</strong><small title={item.resource_id}>{item.resource_id || '无资源 ID'}</small></span>
    <div className="audit-metadata">
      {metadataText ? <>
        <button className="text-icon-button" type="button" aria-expanded={expanded} onClick={() => setExpanded((value) => !value)}>{expanded ? <ChevronUp size={14} /> : <ChevronDown size={14} />}{expanded ? '收起元数据' : '展开元数据'}</button>
        {expanded && <pre>{metadataText}</pre>}
      </> : <span className="audit-muted">—</span>}
    </div>
  </article>
}

function formatTime(value: string) {
  if (!value) return '未知时间'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString('zh-CN')
}

function formatMetadata(metadata: unknown) {
  if (metadata === null || metadata === undefined) return ''
  if (typeof metadata === 'string') return metadata.trim()
  if (typeof metadata === 'object' && Object.keys(metadata as Record<string, unknown>).length === 0) return ''
  try {
    return JSON.stringify(metadata, null, 2)
  } catch {
    return String(metadata)
  }
}
