import type { ApiKeyStatus, ModelStatus, ProgressCurrent, ResourceStatus, Role } from '../api/types'

const roleLabels: Record<Role, string> = { student: '学生', mentor: '导师', teacher: '老师' }
const statusLabels: Record<ApiKeyStatus | ModelStatus | ResourceStatus | string, string> = {
  pending_mentor: '等待导师审核', pending_teacher: '等待老师审核', approved: '等待领取', active: '已启用',
  rejected: '已驳回', revoked: '已撤销', inactive: '已停用', pending_configuration: '待配置', ready: '已就绪', pending: '待审核',
  cancelled: '已取消', mentor_review: '导师审核', teacher_review: '老师审核', claim: '等待领取',
  rejected_teacher: '老师已驳回',
}
export const roleLabel = (role?: Role) => role ? roleLabels[role] : ''
export const statusLabel = (status?: string) => status ? statusLabels[status] ?? status : ''
export const progressLabel = (current: ProgressCurrent) => statusLabel(current)
export function fullTime(value?: string | null): string { return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '—' }
export function relativeTime(value?: string | null): string {
  if (!value) return '—'
  const seconds = Math.round((new Date(value).getTime() - Date.now()) / 1000)
  const formatter = new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' })
  if (Math.abs(seconds) < 60) return formatter.format(seconds, 'second')
  const minutes = Math.round(seconds / 60); if (Math.abs(minutes) < 60) return formatter.format(minutes, 'minute')
  const hours = Math.round(minutes / 60); if (Math.abs(hours) < 24) return formatter.format(hours, 'hour')
  return formatter.format(Math.round(hours / 24), 'day')
}
