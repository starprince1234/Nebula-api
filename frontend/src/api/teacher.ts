import { get, page, patch, post } from './client'
import type { ApiKey, Application, Binding, BindingCreate, BindingUpdate, MentorCandidate, Model, ModelCreate, ModelUpdate, Organization, OrganizationCreate, OrganizationUpdate, Project, ProjectCreate, ProjectUpdate, Provider, ProviderCreate, ProviderUpdate, ProjectUsage } from './types'
export const teacherAPI = {
  invite: (email: string) => post<{ invited: boolean }>('/api/v1/teacher/invitations', { email }),
  organizations: () => get<Organization[]>('/api/v1/teacher/organizations'),
  createOrganization: (body: OrganizationCreate) => post<Organization>('/api/v1/teacher/organizations', body),
  updateOrganization: (id: string, body: OrganizationUpdate) => patch<Organization>(`/api/v1/teacher/organizations/${id}`, body),
  mentorCandidates: (org: string, query = '', cursor = '', limit = 20) => page<MentorCandidate[]>(`/api/v1/teacher/organizations/${org}/mentor-candidates?${new URLSearchParams({ q: query, ...(cursor ? { cursor } : {}), limit: String(limit) })}`),
  assignMentor: (org: string, mentor: string) => post<void>(`/api/v1/teacher/organizations/${org}/mentors/${mentor}`),
  projects: (filters: { organization_id?: string; status?: string } = {}) => { const query = new URLSearchParams(filters); return get<Project[]>(`/api/v1/teacher/projects${query.size ? `?${query}` : ''}`) },
  projectSpend: (query = '') => get<ProjectUsage[]>(`/api/v1/teacher/project-spend${query ? `?${query}` : ''}`),
  createProject: (body: ProjectCreate) => post<Project>('/api/v1/teacher/projects', body),
  updateProject: (id: string, body: ProjectUpdate) => patch<Project>(`/api/v1/teacher/projects/${id}`, body),
  mentorApplications: (status = 'pending') => get<Application[]>(`/api/v1/teacher/mentor-project-applications?status=${status}`),
  reviewMentor: (id: string, approve: boolean, comment?: string) => post<void>(`/api/v1/teacher/mentor-project-applications/${id}/${approve ? 'approve' : 'reject'}`, comment ? { comment } : undefined),
  providers: () => get<Provider[]>('/api/v1/teacher/providers'),
  createProvider: (body: ProviderCreate) => post<Provider>('/api/v1/teacher/providers', body),
  updateProvider: (id: string, body: ProviderUpdate) => patch<Provider>(`/api/v1/teacher/providers/${id}`, body),
  models: () => get<Model[]>('/api/v1/teacher/models'),
  createModel: (body: ModelCreate) => post<Model>('/api/v1/teacher/models', body),
  updateModel: (id: string, body: ModelUpdate) => patch<Model>(`/api/v1/teacher/models/${id}`, body),
  model: (id: string) => get<{ model: Model; bindings: Binding[] }>(`/api/v1/teacher/models/${id}`),
  createBinding: (id: string, body: BindingCreate) => post<Binding>(`/api/v1/teacher/models/${id}/bindings`, body),
  updateBinding: (id: string, body: BindingUpdate) => patch<Binding>(`/api/v1/teacher/model-bindings/${id}`, body),
  keyReviews: () => get<ApiKey[]>('/api/v1/teacher/api-key-reviews'),
  keyReview: (id: string) => get<ApiKey>(`/api/v1/teacher/api-key-reviews/${id}`),
  reviewKey: (id: string, approve: boolean, comment?: string) => post<void>(`/api/v1/teacher/api-key-reviews/${id}/${approve ? 'approve' : 'reject'}`, comment ? { comment } : undefined)
}
