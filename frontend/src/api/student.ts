import { get, post } from './client'
import type { ApiKey, Model, Organization, Project, SubmitApiKey } from './types'
export const studentAPI = {
  organizations: () => get<Organization[]>('/api/v1/student/organizations'),
  projects: (org: string) => get<Project[]>(`/api/v1/student/organizations/${org}/projects`),
  models: () => get<Model[]>('/api/v1/student/models'),
  keys: () => get<ApiKey[]>('/api/v1/student/api-keys'),
  key: (id: string) => get<ApiKey>(`/api/v1/student/api-keys/${id}`),
  submit: (body: SubmitApiKey) => post<ApiKey>('/api/v1/student/api-keys', body),
  claim: (id: string) => post<{ api_key: string; key_prefix: string; claimed_at: string }>(`/api/v1/student/api-keys/${id}/claim`)
}
