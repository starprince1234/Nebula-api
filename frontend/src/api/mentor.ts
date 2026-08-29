import { get, patch, post } from './client'
import type { ApiKey, Application, Organization, Project, ProjectUsage, CallLog, InputMonitorItem } from './types'
export const mentorAPI = {
  organizations: () => get<Organization[]>('/api/v1/mentor/organizations'),
  projects: (org: string) => get<Project[]>(`/api/v1/mentor/organizations/${org}/projects`),
  applications: () => get<Application[]>('/api/v1/mentor/project-applications'),
  apply: (project_id: string) => post<Application>('/api/v1/mentor/project-applications', { project_id }),
  reviews: () => get<ApiKey[]>('/api/v1/mentor/api-key-reviews'),
  review: (id: string) => get<ApiKey>(`/api/v1/mentor/api-key-reviews/${id}`),
  approve: (id: string, comment?: string) => post<void>(`/api/v1/mentor/api-key-reviews/${id}/approve`, comment ? { comment } : undefined),
  reject: (id: string, comment: string) => post<void>(`/api/v1/mentor/api-key-reviews/${id}/reject`, { comment }),
  activeKeys: (project: string) => get<ApiKey[]>(`/api/v1/mentor/projects/${project}/api-keys`),
  revoke: (id: string, comment: string) => post<void>(`/api/v1/mentor/api-keys/${id}/revoke`, { comment })
  ,updateQuota: (id: string, monthly_credits: string, reason: string) => patch<void>(`/api/v1/mentor/api-keys/${id}/monthly-credit-quota`, { monthly_credits, reason })
  ,usage: (id: string, month?: string) => get<ProjectUsage>(`/api/v1/mentor/projects/${id}/usage${month ? `?month=${month}` : ''}`)
  ,callLogs: (query = '') => get<{items:CallLog[];next_cursor?:string}>(`/api/v1/mentor/call-logs${query ? `?${query}` : ''}`)
  ,inputs: (query = '') => get<{items:InputMonitorItem[];next_cursor?:string}>(`/api/v1/mentor/input-monitor${query ? `?${query}` : ''}`)
  ,input: (id: string) => get<InputMonitorItem>(`/api/v1/mentor/input-monitor/${id}`)
}
