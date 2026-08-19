import { get, post } from './client'
import type { ApiKey, Application, Organization, Project } from './types'
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
}
