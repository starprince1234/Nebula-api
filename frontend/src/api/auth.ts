import { get, post } from './client'
import type { Session, User } from './types'
export const authAPI = {
  sendCode: (email: string, purpose: string) => post<{ sent: boolean }>('/api/v1/auth/verification-codes', { email, purpose }),
  register: (role: 'student' | 'mentor', body: { name: string; email: string; password: string; verification_code: string }) => post<User>(`/api/v1/auth/register/${role}`, body),
  login: (body: { email: string; password: string }) => post<Session>('/api/v1/auth/login', body),
  refresh: () => post<Session>('/api/v1/auth/refresh'),
  logout: () => post<void>('/api/v1/auth/logout'),
  forgot: (email: string) => post<{ accepted: boolean }>('/api/v1/auth/password/forgot', { email }),
  reset: (body: { email: string; verification_code: string; new_password: string }) => post<void>('/api/v1/auth/password/reset', body),
  activateTeacher: (body: { token: string; name: string; password: string }) => post<User>('/api/v1/auth/teacher-invitations/activate', body),
  me: () => get<User>('/api/v1/me')
}
