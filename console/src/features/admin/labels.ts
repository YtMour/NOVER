import type { OperatorSession, Product } from '../../types'

export function productStatusLabel(status: Product['status']) {
  return { DRAFT: '草稿', ACTIVE: '已发布', ARCHIVED: '已归档' }[status]
}

export function roleLabel(role?: OperatorSession['role']) {
  if (!role) return '未分配角色'
  return { OWNER: '空间所有者', ADMIN: '管理员', EDITOR: '内容编辑', FULFILLMENT: '交付运营', ANALYST: '数据分析' }[role]
}

export function applicationStatusLabel(status?: OperatorSession['application_status']) {
  return { PENDING: '审核中', REVIEWING: '复核中', APPROVED: '已通过', REJECTED: '未通过' }[status ?? 'PENDING']
}

export function platformRoleLabel(role: OperatorSession['platform_role']) {
  if (!role) return '租户成员'
  return { PLATFORM_OWNER: '平台所有者', PLATFORM_SUPPORT: '平台支持', PLATFORM_AUDITOR: '平台审计' }[role]
}
